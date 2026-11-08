package archive

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ActionRecord is one operation that changed, or was done to, live
// infrastructure. Finished records are never modified.
type ActionRecord struct {
	APIVersion      string       `json:"apiVersion"`
	Kind            string       `json:"kind"`
	ID              string       `json:"id"`
	Action          string       `json:"action"`
	Targets         []string     `json:"targets"`
	Actor           Actor        `json:"actor"`
	Approver        *Actor       `json:"approver,omitempty"`
	ArchiveRevision string       `json:"archiveRevision,omitempty"`
	Plan            *PlanSummary `json:"plan,omitempty"`
	Status          string       `json:"status"`
	Message         string       `json:"message,omitempty"`
	Steps           []Step       `json:"steps"`
	StartedAt       string       `json:"startedAt"`
	FinishedAt      string       `json:"finishedAt,omitempty"`
	Links           Links        `json:"links,omitempty"`
	file            string
	raw             []byte
}

type Actor struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Session string `json:"session,omitempty"`
	Machine string `json:"machine,omitempty"`
	At      string `json:"at,omitempty"`
}
type PlanSummary struct {
	Digest  string `json:"digest,omitempty"`
	Summary string `json:"summary,omitempty"`
}
type Step struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	At      string `json:"at,omitempty"`
	Message string `json:"message,omitempty"`
}
type Links struct {
	Run      string `json:"run,omitempty"`
	Artifact string `json:"artifact,omitempty"`
	Commit   string `json:"commit,omitempty"`
}

// File returns the record path relative to the archive.
func (r *ActionRecord) File() string { return r.file }

var (
	ulidRE   = regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{26}$`)
	actionRE = regexp.MustCompile(`^[a-z][a-z0-9-]*(\.[a-z][a-z0-9-]*)*$`)
	actorRE  = regexp.MustCompile(`^(human|ai)/[A-Za-z0-9._@-]{1,64}$`)
)

var recordStatuses = []string{"running", "succeeded", "failed", "cancelled", "unknown"}

func terminal(status string) bool { return status != "" && status != "running" }

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// NewULID returns a time-ordered 26 character identifier.
func NewULID(t time.Time) string {
	var b [16]byte
	binary.BigEndian.PutUint64(b[:8], uint64(t.UnixMilli())<<16)
	_, _ = rand.Read(b[6:])
	var out [26]byte
	// 128 bits, most significant first, 5 bits per character (the first carries 3).
	hi, lo := binary.BigEndian.Uint64(b[:8]), binary.BigEndian.Uint64(b[8:])
	for i := 25; i >= 0; i-- {
		out[i] = crockford[lo&31]
		lo = lo>>5 | hi<<59
		hi >>= 5
	}
	return string(out[:])
}

func recordPath(id, startedAt string) (string, error) {
	t, err := time.Parse(time.RFC3339, startedAt)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("records/actions/%04d/%02d/%s.json", t.Year(), int(t.Month()), id), nil
}

func (a *Archive) loadRecords() {
	for _, f := range a.listFiles("records") {
		if !strings.HasPrefix(f, "records/actions/") {
			continue // records/runs and records/artifacts are reserved for imported desktop records
		}
		b, err := readRegular(filepath.Join(a.Dir, f), maxRecordSize)
		if err != nil {
			a.errorf(f, 0, "", "file.unreadable", "%v", err)
			continue
		}
		rec, problems := parseRecord(b)
		for _, p := range problems {
			a.errorf(f, 0, p[0], "record.invalid", "%s", p[1])
		}
		if rec == nil {
			continue
		}
		if want, err := recordPath(rec.ID, rec.StartedAt); err == nil && want != f {
			a.errorf(f, 0, "", "record.path", "record %s belongs at %s", rec.ID, want)
		}
		rec.file, rec.raw = f, b
		a.Records = append(a.Records, rec)
	}
}

// parseRecord decodes strictly and returns [path, message] problems.
func parseRecord(b []byte) (*ActionRecord, [][2]string) {
	var rec ActionRecord
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&rec); err != nil {
		return nil, [][2]string{{"", "invalid JSON: " + err.Error()}}
	}
	var p [][2]string
	add := func(field, format string, args ...any) { p = append(p, [2]string{field, fmt.Sprintf(format, args...)}) }
	if rec.APIVersion != APIVersion || rec.Kind != "ActionRecord" {
		add("kind", "apiVersion must be %s and kind ActionRecord", APIVersion)
	}
	if !ulidRE.MatchString(rec.ID) {
		add("id", "id must be a ULID")
	}
	if !actionRE.MatchString(rec.Action) {
		add("action", "action must be dotted lowercase words such as config.set or manual")
	}
	if len(rec.Targets) == 0 {
		add("targets", "at least one target is required")
	}
	for _, t := range rec.Targets {
		if _, _, _, _, err := ParseRef(t); err != nil {
			add("targets", "%v", err)
		}
	}
	if (rec.Actor.Type != "human" && rec.Actor.Type != "ai") || rec.Actor.Name == "" {
		add("actor", "actor needs type human or ai and a name")
	}
	if !contains(recordStatuses, rec.Status) {
		add("status", "status must be one of %s", strings.Join(recordStatuses, ", "))
	}
	if _, err := time.Parse(time.RFC3339, rec.StartedAt); err != nil {
		add("startedAt", "startedAt must be an RFC 3339 timestamp")
	}
	if terminal(rec.Status) {
		if _, err := time.Parse(time.RFC3339, rec.FinishedAt); err != nil {
			add("finishedAt", "finished records need an RFC 3339 finishedAt")
		}
	}
	return &rec, p
}

// NoteInput describes a manual operation to record.
type NoteInput struct {
	Targets  []string
	Message  string
	Actor    string // human/<name> or ai/<name>
	Session  string
	Status   string
	Revision string
	Now      time.Time
}

// AddNote writes an action: manual record for work done outside DevOpsPad.
func (a *Archive) AddNote(in NoteInput) (*ActionRecord, error) {
	if strings.TrimSpace(in.Message) == "" {
		return nil, errors.New("-m is required: describe what was done")
	}
	if !actorRE.MatchString(in.Actor) {
		return nil, errors.New("--actor must be human/<name> or ai/<name>")
	}
	if in.Status == "" {
		in.Status = "succeeded"
	}
	if !terminal(in.Status) || !contains(recordStatuses, in.Status) {
		return nil, errors.New("--status must be succeeded, failed, cancelled or unknown")
	}
	for _, t := range in.Targets {
		kind, name, _, env, err := ParseRef(t)
		if err != nil {
			return nil, err
		}
		if a.byRef[kind+":"+name] == nil {
			return nil, fmt.Errorf("%s does not exist%s", kind+":"+name, a.suggest(kind))
		}
		if env != "" {
			if kind != "service" && kind != "product" {
				return nil, fmt.Errorf("%s: @environment is only used with product and service targets", t)
			}
			prod := name
			if kind == "service" {
				prod, _, _ = strings.Cut(name, "/")
			}
			if e, names := a.environment(prod, env); e == nil {
				return nil, fmt.Errorf("environment %q is not defined in product:%s; defined: %s", env, prod, listOrNone(names))
			}
		}
	}
	if len(in.Targets) == 0 {
		return nil, errors.New("--refs is required: which resources were affected")
	}
	typ, name, _ := strings.Cut(in.Actor, "/")
	machine, _ := os.Hostname()
	now := in.Now.Format(time.RFC3339)
	rec := &ActionRecord{APIVersion: APIVersion, Kind: "ActionRecord", ID: NewULID(in.Now), Action: "manual", Targets: in.Targets,
		Actor: Actor{Type: typ, Name: name, Session: in.Session, Machine: machine}, ArchiveRevision: in.Revision,
		Status: in.Status, Message: in.Message, Steps: []Step{}, StartedAt: now, FinishedAt: now}
	b, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return nil, err
	}
	b = append(b, '\n')
	for _, p := range secretPatterns {
		if p.re.Match(b) {
			return nil, fmt.Errorf("the note looks like it contains a %s; records never hold secrets", p.name)
		}
	}
	rel, _ := recordPath(rec.ID, rec.StartedAt)
	full := filepath.Join(a.Dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return nil, err
	}
	if err := writeAtomic(full, b, 0o644); err != nil {
		return nil, err
	}
	rec.file = rel
	return rec, nil
}

// checkRecordHistory rejects edits to records that were already finished at
// HEAD. staged compares the index instead of the working tree.
func (a *Archive) checkRecordHistory(repo string, staged bool) {
	if _, err := git(repo, "rev-parse", "--verify", "-q", "HEAD"); err != nil {
		return
	}
	args := []string{"diff", "--name-status", "--no-renames"}
	if staged {
		args = append(args, "--cached")
	}
	out, err := git(repo, append(args, "HEAD", "--", "records/actions")...)
	if err != nil {
		return
	}
	for _, line := range strings.Split(out, "\n") {
		status, file, ok := strings.Cut(line, "\t")
		if !ok || (status != "M" && status != "D") || path.Ext(file) != ".json" {
			continue
		}
		old, err := git(repo, "show", "HEAD:"+file)
		if err != nil {
			continue
		}
		var prev ActionRecord
		if json.Unmarshal([]byte(old), &prev) == nil && terminal(prev.Status) {
			verb := map[string]string{"M": "modified", "D": "deleted"}[status]
			a.errorf(file, 0, "", "record.immutable", "finished record was %s; records are append-only (restore it with git checkout HEAD -- %s)", verb, file)
		}
	}
}

func writeAtomic(p string, b []byte, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(p), ".devopspad-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err = f.Chmod(mode); err != nil {
		return err
	}
	if _, err = f.Write(b); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), p)
}
