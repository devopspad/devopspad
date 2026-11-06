package ops

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

const RunEventVersion = "infradex.run-event/v1"

type RunActor struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	SessionID string `json:"sessionId"`
}
type RunSource struct {
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
	Branch     string `json:"branch"`
	Dirty      bool   `json:"dirty"`
}
type RunTarget struct {
	Environment string `json:"environment"`
	Workload    string `json:"workload"`
	Container   string `json:"container"`
	BuildID     string `json:"buildId"`
}
type RunStart struct {
	Kind        string     `json:"kind"`
	Application string     `json:"application"`
	Version     string     `json:"version"`
	Actor       RunActor   `json:"actor"`
	Source      RunSource  `json:"source"`
	Recipe      string     `json:"recipe"`
	Target      *RunTarget `json:"target,omitempty"`
}
type RunStep struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
	LogPath string `json:"logPath,omitempty"`
}
type RunFinish struct {
	Status  string `json:"status"`
	Image   string `json:"image,omitempty"`
	Message string `json:"message"`
}
type RunEvent struct {
	APIVersion string     `json:"apiVersion"`
	ID         string     `json:"id"`
	RunID      string     `json:"runId"`
	Type       string     `json:"type"`
	Start      *RunStart  `json:"start,omitempty"`
	Step       *RunStep   `json:"step,omitempty"`
	Finish     *RunFinish `json:"finish,omitempty"`
}
type SavedRunEvent struct {
	Event RunEvent `json:"event"`
	At    string   `json:"at"`
}
type Run struct {
	APIVersion string `json:"apiVersion"`
	ID         string `json:"id"`
	RunStart
	Connection string          `json:"connection,omitempty"`
	Namespace  string          `json:"namespace,omitempty"`
	Status     string          `json:"status"`
	Image      string          `json:"image,omitempty"`
	StartedAt  string          `json:"startedAt"`
	FinishedAt string          `json:"finishedAt,omitempty"`
	Message    string          `json:"message"`
	Events     []SavedRunEvent `json:"events"`
}

func DecodeRunEvent(b []byte) (RunEvent, error) {
	var e RunEvent
	if len(b) > 64<<10 {
		return e, errors.New("run event exceeds 64 KiB")
	}
	if err := strictJSON(b, &e); err != nil {
		return e, err
	}
	return e, validateRunEvent(e)
}
func strictJSON(b []byte, out any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return errors.New("expected one JSON object")
	}
	return nil
}
func validUUID(s string) bool { id, err := uuid.Parse(s); return err == nil && id.String() == s }

var commitRE = regexp.MustCompile(`^([a-f0-9]{40}|[a-f0-9]{64})$`)

func plain(s string, max int) bool { return len(s) <= max && !strings.ContainsRune(s, 0) }
func safeRepository(s string) bool {
	if s == "" || !plain(s, 500) {
		return false
	}
	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		return err == nil && u.User == nil && u.RawQuery == "" && u.Fragment == "" && (u.Scheme == "https" || u.Scheme == "ssh" || u.Scheme == "file")
	}
	return !strings.ContainsAny(s, "\n\r?#")
}
func validateRunEvent(e RunEvent) error {
	if e.APIVersion != RunEventVersion || !validUUID(e.ID) || !validUUID(e.RunID) {
		return errors.New("apiVersion must be infradex.run-event/v1; id and runId must be canonical UUIDs")
	}
	switch e.Type {
	case "start":
		if e.Start == nil || e.Step != nil || e.Finish != nil {
			return errors.New("start event requires only start payload")
		}
		s := e.Start
		if (s.Kind != "build" && s.Kind != "release") || !ValidName(s.Application) || strings.TrimSpace(s.Version) == "" || !plain(s.Version, 120) {
			return errors.New("invalid run kind, application or version")
		}
		if (s.Actor.Kind != "ai" && s.Actor.Kind != "human") || strings.TrimSpace(s.Actor.Name) == "" || !plain(s.Actor.Name, 120) || strings.TrimSpace(s.Actor.SessionID) == "" || !plain(s.Actor.SessionID, 200) {
			return errors.New("actor kind, name and sessionId are required")
		}
		if !safeRepository(s.Source.Repository) || !commitRE.MatchString(s.Source.Commit) || !plain(s.Source.Branch, 200) || strings.TrimSpace(s.Recipe) == "" || !plain(s.Recipe, 500) {
			return errors.New("source repository, full commit SHA and recipe label are required; repository URLs must not contain credentials or query parameters")
		}
		if s.Kind == "build" && s.Target != nil {
			return errors.New("build must not have a release target")
		}
		if s.Kind == "release" && (s.Target == nil || !ValidName(s.Target.Environment) || !ValidName(s.Target.Workload) || !ValidName(s.Target.Container) || !validUUID(s.Target.BuildID)) {
			return errors.New("release requires environment, workload, container and successful buildId")
		}
	case "step":
		if e.Step == nil || e.Start != nil || e.Finish != nil {
			return errors.New("step event requires only step payload")
		}
		s := e.Step
		if strings.TrimSpace(s.Name) == "" || !plain(s.Name, 120) || (s.Status != "succeeded" && s.Status != "failed") || !plain(s.Message, 4000) {
			return errors.New("step requires name, succeeded/failed status and bounded message")
		}
		if s.LogPath != "" && (filepath.IsAbs(s.LogPath) || filepath.Clean(s.LogPath) != s.LogPath || strings.Contains(s.LogPath, "\\") || strings.HasPrefix(s.LogPath, "../") || s.LogPath == ".." || !strings.HasPrefix(s.LogPath, "logs/") || !plain(s.LogPath, 500)) {
			return errors.New("logPath must be a relative path under logs/")
		}
	case "finish":
		if e.Finish == nil || e.Start != nil || e.Step != nil {
			return errors.New("finish event requires only finish payload")
		}
		f := e.Finish
		if (f.Status != "succeeded" && f.Status != "failed" && f.Status != "cancelled") || strings.TrimSpace(f.Message) == "" || !plain(f.Message, 4000) {
			return errors.New("finish requires succeeded/failed/cancelled status and message")
		}
		if f.Image != "" && !ValidImage(f.Image) {
			return errors.New("image must have a full sha256 digest")
		}
	default:
		return errors.New("unknown event type; use start, step or finish")
	}
	return nil
}

func (s *Store) projectLock() (func(), error) {
	path := filepath.Join(s.Root, ".infradex-write.lock")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, errors.New("project is being written; retry the same event ID after the other writer finishes")
	}
	_ = f.Close()
	return func() { _ = os.Remove(path) }, nil
}

// RecordRunEvent accepts facts from an external executor. It never invokes a
// build tool, registry push, shell command, or Kubernetes write.
func (s *Store) RecordRunEvent(e RunEvent) (Run, error) {
	var r Run
	if err := validateRunEvent(e); err != nil {
		return r, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.projectLock()
	if err != nil {
		return r, err
	}
	defer unlock()
	p, err := s.project()
	if err != nil {
		return r, err
	}
	path := filepath.Join(s.Root, "runs", e.RunID+".json")
	b, err := readBounded(path)
	missing := errors.Is(err, os.ErrNotExist)
	if err != nil && !missing {
		return r, err
	}
	if !missing {
		r, err = s.readRun(e.RunID)
		if err != nil {
			return r, err
		}
		for _, saved := range r.Events {
			if saved.Event.ID == e.ID {
				if !reflect.DeepEqual(saved.Event, e) {
					return r, errors.New("event ID already exists with different content")
				}
				return r, nil
			}
		}
		if r.Status != "running" {
			return r, errors.New("run is terminal; create a new run for a retry")
		}
	}
	at := time.Now().UTC().Format(time.RFC3339Nano)
	switch e.Type {
	case "start":
		if !missing {
			return r, errors.New("runId already exists; retry with the original event ID")
		}
		found := false
		for _, a := range p.Project.Applications {
			found = found || a.Name == e.Start.Application
		}
		if !found {
			return r, errors.New("application is not registered in project.yaml")
		}
		r = Run{APIVersion: "infradex.run/v1", ID: e.RunID, RunStart: *e.Start, Status: "running", StartedAt: at, Events: []SavedRunEvent{}}
		if r.Kind == "release" {
			target := r.Target
			for _, env := range p.Project.Environments {
				if env.Name == target.Environment {
					r.Connection = env.Connection
					r.Namespace = env.Namespace
				}
			}
			if r.Connection == "" {
				return r, errors.New("target environment is not registered")
			}
			build, err := s.readRun(target.BuildID)
			if err != nil || build.Kind != "build" || build.Status != "succeeded" || build.Application != r.Application || build.Version != r.Version || !reflect.DeepEqual(build.Source, r.Source) {
				return r, errors.New("release must reference a successful build with matching application, version and source")
			}
			r.Image = build.Image
		}
	case "step":
		if missing {
			return r, errors.New("run does not exist; record start before execution")
		}
	case "finish":
		if missing {
			return r, errors.New("run does not exist; record start before execution")
		}
		f := e.Finish
		if f.Status == "succeeded" {
			checks := map[string]string{}
			for _, event := range r.Events {
				if event.Event.Step != nil {
					checks[event.Event.Step.Name] = event.Event.Step.Status
				}
			}
			if r.Kind == "build" {
				if !ValidImage(f.Image) || checks["build"] != "succeeded" || checks["push"] != "succeeded" {
					return r, errors.New("successful build requires successful build and push steps plus immutable image digest")
				}
				r.Image = f.Image
			} else if f.Image != r.Image || checks["rollout"] != "succeeded" || checks["image-check"] != "succeeded" {
				return r, errors.New("successful release requires rollout and image-check steps plus the exact build image")
			}
			for _, status := range checks {
				if status == "failed" {
					return r, errors.New("a failed step must be resolved before success can be reported")
				}
			}
		} else if f.Image != "" {
			return r, errors.New("failed/cancelled finish must omit image")
		}
		r.Status = f.Status
		r.FinishedAt = at
		r.Message = f.Message
	}
	if len(r.Events) >= 256 || (e.Type != "finish" && len(r.Events) >= 255) {
		return r, errors.New("run event limit reached (256)")
	}
	r.Events = append(r.Events, SavedRunEvent{Event: e, At: at})
	b, err = json.MarshalIndent(r, "", "  ")
	if err != nil {
		return r, err
	}
	if len(b) > 1<<20 {
		return r, errors.New("run exceeds 1 MiB")
	}
	if err = writeAtomic(path, b); err != nil {
		return r, err
	}
	return r, nil
}
func (s *Store) readRun(id string) (Run, error) {
	var r Run
	if !validUUID(id) {
		return r, errors.New("invalid run ID")
	}
	b, err := readBounded(filepath.Join(s.Root, "runs", id+".json"))
	if err != nil {
		return r, err
	}
	if err = strictJSON(b, &r); err != nil {
		return r, err
	}
	if r.APIVersion != "infradex.run/v1" || r.ID != id || (r.Kind != "build" && r.Kind != "release") || !ValidName(r.Application) || (r.Status != "running" && r.Status != "succeeded" && r.Status != "failed" && r.Status != "cancelled") || (r.Status == "succeeded" && !ValidImage(r.Image)) {
		return r, errors.New("invalid run record")
	}
	if len(r.Events) == 0 || len(r.Events) > 256 || r.Events[0].Event.Type != "start" || r.Events[0].Event.Start == nil {
		return r, errors.New("run is missing its start event")
	}
	if !reflect.DeepEqual(*r.Events[0].Event.Start, r.RunStart) {
		return r, errors.New("run source differs from its start event")
	}
	seen := map[string]bool{}
	for _, saved := range r.Events {
		if err := validateRunEvent(saved.Event); err != nil {
			return r, err
		}
		if saved.Event.RunID != r.ID || seen[saved.Event.ID] {
			return r, errors.New("invalid run event identity")
		}
		seen[saved.Event.ID] = true
		if _, err := time.Parse(time.RFC3339Nano, saved.At); err != nil {
			return r, errors.New("invalid run event time")
		}
	}
	last := r.Events[len(r.Events)-1].Event
	if r.Status != "running" && (last.Type != "finish" || last.Finish == nil || last.Finish.Status != r.Status) {
		return r, errors.New("terminal run is missing a matching finish event")
	}
	return r, nil
}
func (s *Store) GetRun(id string) (Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readRun(id)
}
func (s *Store) readRuns() ([]Run, error) {
	out := []Run{}
	entries, err := os.ReadDir(filepath.Join(s.Root, "runs"))
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	if len(entries) > 10000 {
		return nil, errors.New("run count exceeds 10000; archive old runs")
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		r, err := s.readRun(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", entry.Name(), err)
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt > out[j].StartedAt })
	return out, nil
}
