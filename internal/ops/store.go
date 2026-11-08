// Package ops stores a portable, Git-friendly application project. Secrets and
// live cluster data never belong in this directory.
package ops

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/labels"
)

type Environment struct {
	Name       string `json:"name" yaml:"name"`
	Connection string `json:"connection" yaml:"connection"`
	Namespace  string `json:"namespace" yaml:"namespace"`
}
type Application struct {
	Name      string `json:"name" yaml:"name"`
	Selector  string `json:"selector" yaml:"selector"`
	NginxPath string `json:"nginxPath,omitempty" yaml:"nginxPath,omitempty"`
}
type Project struct {
	APIVersion   string        `json:"apiVersion" yaml:"apiVersion"`
	Name         string        `json:"name" yaml:"name"`
	Environments []Environment `json:"environments" yaml:"environments"`
	Applications []Application `json:"applications" yaml:"applications"`
}
type Artifact struct {
	RunID       string `json:"runId,omitempty" yaml:"runId,omitempty"`
	ID          string `json:"id" yaml:"id"`
	Application string `json:"application" yaml:"application"`
	Version     string `json:"version" yaml:"version"`
	Image       string `json:"image" yaml:"image"`
	Source      string `json:"source" yaml:"source"`
	CreatedAt   string `json:"createdAt" yaml:"createdAt"`
}
type Record struct {
	RunID          string   `json:"runId,omitempty"`
	Action         string   `json:"action,omitempty"`
	BeforeReplicas *int64   `json:"beforeReplicas,omitempty"`
	Replicas       *int64   `json:"replicas,omitempty"`
	ID             string   `json:"id"`
	Application    string   `json:"application"`
	Environment    string   `json:"environment"`
	Connection     string   `json:"connection"`
	Namespace      string   `json:"namespace"`
	Workload       string   `json:"workload"`
	Container      string   `json:"container"`
	Image          string   `json:"image"`
	BeforeImage    string   `json:"beforeImage"`
	Version        string   `json:"version"`
	Status         string   `json:"status"`
	StartedAt      string   `json:"startedAt"`
	FinishedAt     string   `json:"finishedAt"`
	Message        string   `json:"message"`
	Steps          []string `json:"steps"`
	// Targets lists every workload of a multi-workload release; Workload and
	// Container then join their names for display.
	Targets []RecordTarget `json:"targets,omitempty"`
	// ConfigMap and Key identify a config change (action config); Workload is
	// then set only when the change also restarted a Deployment.
	ConfigMap string `json:"configMap,omitempty"`
	Key       string `json:"key,omitempty"`
}

type RecordTarget struct {
	Application string `json:"application"`
	Workload    string `json:"workload"`
	Container   string `json:"container"`
	BeforeImage string `json:"beforeImage"`
	Status      string `json:"status"` // succeeded | failed | not-started
}
type Snapshot struct {
	Runs      []Run      `json:"runs"`
	Root      string     `json:"root"`
	YAML      string     `json:"yaml"`
	Revision  string     `json:"revision"`
	Project   Project    `json:"project"`
	Artifacts []Artifact `json:"artifacts"`
	Releases  []Record   `json:"releases"`
}
type Store struct {
	Root string
	mu   sync.Mutex
	// OnFinished, when set, observes every record written with a final status
	// (used to mirror operations into the archive). Its failure never fails
	// the operation.
	OnFinished func(Record, Application)
}

var nameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
var imageRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:/-]*@sha256:[a-f0-9]{64}$`)

func ValidImage(s string) bool {
	return len(s) < 1024 && imageRE.MatchString(s) && !strings.Contains(s, ":latest@")
}
func ValidName(s string) bool { return nameRE.MatchString(s) }
func New(root string) (*Store, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	// A project is a local checkout, not a shared network filesystem database.
	s := &Store{Root: root}
	if _, err = os.Stat(filepath.Join(root, "project.yaml")); errors.Is(err, os.ErrNotExist) {
		p := Project{APIVersion: "devopspad.project/v1", Name: filepath.Base(root), Environments: []Environment{}, Applications: []Application{}}
		b, _ := yaml.Marshal(p)
		if err = writeAtomic(filepath.Join(root, "project.yaml"), b); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	for _, dir := range []string{"artifacts", "releases", "runs"} {
		if err = os.MkdirAll(filepath.Join(root, dir), 0700); err != nil {
			return nil, err
		}
		info, e := os.Lstat(filepath.Join(root, dir))
		if e != nil {
			return nil, e
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("The project directory cannot be a symbolic link")
		}
	}
	if _, err = s.Read(); err != nil {
		return nil, err
	}
	return s, nil
}
func decode(b []byte, out any) error {
	if len(b) > 1<<20 {
		return errors.New("File exceeds 1 MiB")
	}
	d := yaml.NewDecoder(bytes.NewReader(b))
	d.KnownFields(true)
	if err := d.Decode(out); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return errors.New("Only one YAML document is allowed")
	}
	return nil
}
func Validate(text string) (Project, error) {
	var p Project
	if err := decode([]byte(text), &p); err != nil {
		return p, err
	}
	if p.APIVersion != "devopspad.project/v1" || strings.TrimSpace(p.Name) == "" || len(p.Name) > 120 {
		return p, errors.New("apiVersion: devopspad.project/v1 and a project name are required")
	}
	if len(p.Environments) > 32 || len(p.Applications) > 500 {
		return p, errors.New("A project can have at most 32 environments and 500 applications")
	}
	seen := map[string]bool{}
	for _, e := range p.Environments {
		if !ValidName(e.Name) || !ValidName(e.Namespace) || strings.TrimSpace(e.Connection) == "" || len(e.Connection) > 200 || seen[e.Name] {
			return p, errors.New("Environment names must be unique and set a connection name and a specific namespace")
		}
		seen[e.Name] = true
	}
	seen = map[string]bool{}
	for _, a := range p.Applications {
		if !ValidName(a.Name) || seen[a.Name] || strings.TrimSpace(a.Selector) == "" {
			return p, errors.New("Application names must be unique and set a label selector")
		}
		if _, err := labels.Parse(a.Selector); err != nil {
			return p, fmt.Errorf("Invalid label selector for application %s: %w", a.Name, err)
		}
		if a.NginxPath != "" && (len(a.NginxPath) > 240 || !regexp.MustCompile(`^/etc/nginx/conf\.d/[a-zA-Z0-9][a-zA-Z0-9._-]*\.conf$`).MatchString(a.NginxPath)) {
			return p, fmt.Errorf("nginxPath of application %s must be a .conf file under /etc/nginx/conf.d/", a.Name)
		}
		seen[a.Name] = true
	}
	if p.Environments == nil {
		p.Environments = []Environment{}
	}
	if p.Applications == nil {
		p.Applications = []Application{}
	}
	return p, nil
}
func revision(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func (s *Store) project() (Snapshot, error) {
	out := Snapshot{Root: s.Root, Artifacts: []Artifact{}, Releases: []Record{}}
	b, err := readBounded(filepath.Join(s.Root, "project.yaml"))
	if err != nil {
		return out, err
	}
	out.Project, err = Validate(string(b))
	out.YAML = string(b)
	out.Revision = revision(b)
	return out, err
}
func (s *Store) Read() (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read()
}
func (s *Store) read() (Snapshot, error) {
	out, err := s.project()
	if err != nil {
		return out, err
	}
	for _, kind := range []string{"artifacts", "releases"} {
		entries, err := os.ReadDir(filepath.Join(s.Root, kind))
		if err != nil {
			return out, err
		}
		if len(entries) > 10000 {
			return out, errors.New("The project has more than 10000 records; archive older records")
		}
		for _, entry := range entries {
			ext := ".yaml"
			if kind == "releases" {
				ext = ".json"
			}
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ext) {
				continue
			}
			if _, e := uuid.Parse(strings.TrimSuffix(entry.Name(), ext)); e != nil {
				return out, fmt.Errorf("Invalid record file name: %s", entry.Name())
			}
			b, err := readBounded(filepath.Join(s.Root, kind, entry.Name()))
			if err != nil {
				return out, err
			}
			if kind == "artifacts" {
				var a Artifact
				if err = decode(b, &a); err != nil {
					return out, fmt.Errorf("%s: %w", entry.Name(), err)
				}
				if !ValidImage(a.Image) || !ValidName(a.Application) || a.ID != strings.TrimSuffix(entry.Name(), ext) {
					return out, fmt.Errorf("Invalid artifact: %s", entry.Name())
				}
				out.Artifacts = append(out.Artifacts, a)
			} else {
				var r Record
				decoder := json.NewDecoder(bytes.NewReader(b))
				decoder.DisallowUnknownFields()
				if err = decoder.Decode(&r); err != nil {
					return out, fmt.Errorf("%s: %w", entry.Name(), err)
				}
				if r.ID != strings.TrimSuffix(entry.Name(), ext) || !ValidName(r.Application) || !ValidName(r.Environment) || (r.Status != "running" && r.Status != "succeeded" && r.Status != "failed") {
					return out, fmt.Errorf("Invalid release record: %s", entry.Name())
				}
				if r.Steps == nil {
					r.Steps = []string{}
				}
				out.Releases = append(out.Releases, r)
			}
		}
	}
	out.Runs, err = s.readRuns()
	if err != nil {
		return out, err
	}
	for _, run := range out.Runs {
		if run.Kind == "build" && run.Status == "succeeded" {
			out.Artifacts = append(out.Artifacts, Artifact{ID: run.ID, RunID: run.ID, Application: run.Application, Version: run.Version, Image: run.Image, Source: run.Source.Repository + "@" + run.Source.Commit, CreatedAt: run.FinishedAt})
		}
	}
	sort.Slice(out.Artifacts, func(i, j int) bool { return out.Artifacts[i].CreatedAt > out.Artifacts[j].CreatedAt })
	sort.Slice(out.Releases, func(i, j int) bool { return out.Releases[i].StartedAt > out.Releases[j].StartedAt })
	return out, nil
}
func (s *Store) Save(text, expected string) (Snapshot, error) {
	if _, err := Validate(text); err != nil {
		return Snapshot{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := os.OpenFile(filepath.Join(s.Root, ".devopspad-write.lock"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return Snapshot{}, errors.New("The project is being written; if a previous process crashed, check and remove .devopspad-write.lock")
	}
	lock.Close()
	defer os.Remove(lock.Name())
	old, err := s.project()
	if err != nil {
		return old, err
	}
	if old.Revision != expected {
		return old, errors.New("The project was modified; refresh and merge the changes before saving")
	}
	if err = writeAtomic(filepath.Join(s.Root, "project.yaml"), []byte(text)); err != nil {
		return old, err
	}
	return s.read()
}
func (s *Store) AddArtifact(a Artifact) (Snapshot, error) {
	if !ValidImage(a.Image) || !ValidName(a.Application) || strings.TrimSpace(a.Version) == "" || len(a.Version) > 120 || len(a.Source) > 500 {
		return Snapshot{}, errors.New("Enter the application, version and an image with a sha256 digest")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.project()
	if err != nil {
		return p, err
	}
	found := false
	for _, app := range p.Project.Applications {
		found = found || app.Name == a.Application
	}
	if !found {
		return p, errors.New("Application not found")
	}
	a.RunID = ""
	a.ID = uuid.NewString()
	a.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	b, err := yaml.Marshal(a)
	if err != nil {
		return p, err
	}
	if err = writeAtomic(filepath.Join(s.Root, "artifacts", a.ID+".yaml"), b); err != nil {
		return p, err
	}
	return s.read()
}
func (s *Store) WriteRecord(r Record) error {
	if _, err := uuid.Parse(r.ID); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := writeAtomic(filepath.Join(s.Root, "releases", r.ID+".json"), b); err != nil {
		return err
	}
	if s.OnFinished != nil && r.Status != "" && r.Status != "running" {
		app := Application{Name: r.Application}
		if snap, err := s.project(); err == nil {
			for _, a := range snap.Project.Applications {
				if a.Name == r.Application {
					app = a
				}
			}
		}
		func() {
			defer func() { _ = recover() }()
			s.OnFinished(r, app)
		}()
	}
	return nil
}
func readBounded(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("Project files must be regular files, not symbolic links")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if len(b) > 1<<20 {
		return nil, errors.New("File exceeds 1 MiB")
	}
	return b, err
}
func writeAtomic(path string, b []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".devopspad-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err = f.Chmod(0600); err != nil {
		return err
	}
	if _, err = f.Write(b); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
