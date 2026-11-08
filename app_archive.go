package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"devopspad/internal/archive"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// ArchiveResource is one row of the archive browser.
type ArchiveResource struct {
	Ref     string            `json:"ref"`
	Kind    string            `json:"kind"`
	Name    string            `json:"name"`
	Title   string            `json:"title"`
	File    string            `json:"file"`
	Labels  map[string]string `json:"labels"`
	Header  []string          `json:"header"`
	Row     []string          `json:"row"`
	UsedBy  []string          `json:"usedBy"`
	Product string            `json:"product,omitempty"`
}

type ArchiveSummary struct {
	Dir       string               `json:"dir"`
	Name      string               `json:"name"`
	Errors    []archive.Issue      `json:"errors"`
	Warnings  []archive.Issue      `json:"warnings"`
	Resources []ArchiveResource    `json:"resources"`
	Matrix    []archive.MatrixRow  `json:"matrix"`
	Renewals  []archive.RenewalRow `json:"renewals"`
	Drift     []archive.Drift      `json:"drift"`
	Git       *archive.Status      `json:"git,omitempty"`
	GitError  string               `json:"gitError,omitempty"`
}

type ArchiveDetail struct {
	Description *archive.Description `json:"description"`
	Text        string               `json:"text"`
	Revision    string               `json:"revision"`
	Observation *archive.Observation `json:"observation,omitempty"`
}

type ArchiveSaveResult struct {
	Revision string          `json:"revision"`
	Issues   []archive.Issue `json:"issues"`
}

// desktopConfigDir matches the directory the app's connection store uses.
func desktopConfigDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "DevOpsPad"), nil
}

func (a *App) archive() (*archive.Archive, error) {
	cfg, err := desktopConfigDir()
	if err != nil {
		return nil, err
	}
	dir, err := archive.Find("", cfg)
	if err != nil {
		return nil, errors.New("No archive selected. Create one, or open an existing archive folder.")
	}
	return archive.Load(dir)
}

func fileRevision(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

// GetArchive returns everything the archive page shows. Drift and observed
// renewals come from the last local scans; nothing here touches the network.
func (a *App) GetArchive() (ArchiveSummary, error) {
	ar, err := a.archive()
	if err != nil {
		return ArchiveSummary{}, err
	}
	s := ArchiveSummary{Dir: ar.Dir, Name: ar.Root.Metadata.Name, Errors: ar.Errors, Warnings: ar.Warnings, Resources: []ArchiveResource{}}
	all, _ := ar.List("", "")
	for _, r := range all {
		h := r.Header()
		header, row := archive.Summary(r)
		s.Resources = append(s.Resources, ArchiveResource{Ref: r.Ref, Kind: r.Kind, Name: r.Name, Title: h.Metadata.Title, File: r.File,
			Labels: h.Metadata.Labels, Header: header, Row: row, UsedBy: ar.UsedBy(r.Ref), Product: h.Metadata.Product})
	}
	s.Matrix, _ = ar.Matrix("", "")
	days := ar.Root.Spec.Defaults.RenewalWarningDays
	if days == 0 {
		days = 30
	}
	s.Renewals = ar.RenewalsWithObserved(time.Now(), days)
	s.Drift, _ = ar.Diff(nil, "", time.Now())
	if st, err := archive.GitStatus(ar.Dir, false); err != nil {
		s.GitError = err.Error()
	} else {
		s.Git = &st
	}
	return s, nil
}

// SelectArchive opens an existing archive folder, or creates one when create is set.
func (a *App) SelectArchive(create bool) (ArchiveSummary, error) {
	title := "Choose an archive folder (contains devopspad.yaml)"
	if create {
		title = "Choose an empty folder for the new archive"
	}
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: title, CanCreateDirectories: true})
	if err != nil || dir == "" {
		if err == nil {
			return a.GetArchive()
		}
		return ArchiveSummary{}, err
	}
	if create {
		if _, err := archive.Init(dir, ""); err != nil {
			return ArchiveSummary{}, err
		}
	} else if _, err := archive.Find(dir, ""); err != nil {
		return ArchiveSummary{}, err
	}
	cfg, err := desktopConfigDir()
	if err != nil {
		return ArchiveSummary{}, err
	}
	if err := os.MkdirAll(cfg, 0o700); err != nil {
		return ArchiveSummary{}, err
	}
	abs, _ := filepath.Abs(dir)
	if err := os.WriteFile(filepath.Join(cfg, "archive-path"), []byte(abs+"\n"), 0o600); err != nil {
		return ArchiveSummary{}, err
	}
	return a.GetArchive()
}

func (a *App) DescribeArchiveResource(ref string) (ArchiveDetail, error) {
	ar, err := a.archive()
	if err != nil {
		return ArchiveDetail{}, err
	}
	d, err := ar.Describe(ref)
	if err != nil {
		return ArchiveDetail{}, err
	}
	raw := ar.Get(d.Ref).Raw()
	out := ArchiveDetail{Description: d, Text: string(raw), Revision: fileRevision(raw)}
	if d.Kind == "Domain" || d.Kind == "Cluster" || d.Kind == "Host" {
		out.Observation, _ = ar.ReadObservation(d.Ref)
	}
	return out, nil
}

// editableArchiveFile allows resource YAML, notes and configs only.
func editableArchiveFile(rel string) bool {
	clean := path.Clean(rel)
	if clean != rel || strings.HasPrefix(clean, "../") || path.IsAbs(clean) {
		return false
	}
	return (strings.HasPrefix(clean, "inventory/") || strings.HasPrefix(clean, "products/") || strings.HasPrefix(clean, "notes/")) &&
		(strings.HasSuffix(clean, ".yaml") || strings.HasSuffix(clean, ".md") || strings.Contains(clean, "/configs/"))
}

// SaveArchiveFile writes one file if it still has the expected revision, then
// returns the validation issues of that file. Git keeps the previous version.
func (a *App) SaveArchiveFile(file, text, revision string) (ArchiveSaveResult, error) {
	ar, err := a.archive()
	if err != nil {
		return ArchiveSaveResult{}, err
	}
	if !editableArchiveFile(file) {
		return ArchiveSaveResult{}, errors.New("Only resource, note and config files can be edited here")
	}
	p := filepath.Join(ar.Dir, filepath.FromSlash(file))
	if info, err := os.Lstat(p); err == nil && !info.Mode().IsRegular() {
		return ArchiveSaveResult{}, errors.New("Not a regular file")
	}
	if old, err := os.ReadFile(p); err == nil && fileRevision(old) != revision {
		return ArchiveSaveResult{}, errors.New("The file changed on disk since it was opened; reload it, your edits are kept in the editor")
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".devopspad-*")
	if err != nil {
		return ArchiveSaveResult{}, err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(text); err != nil {
		tmp.Close()
		return ArchiveSaveResult{}, err
	}
	tmp.Chmod(0o644)
	if err := tmp.Close(); err != nil {
		return ArchiveSaveResult{}, err
	}
	if err := os.Rename(tmp.Name(), p); err != nil {
		return ArchiveSaveResult{}, err
	}
	res := ArchiveSaveResult{Revision: fileRevision([]byte(text)), Issues: []archive.Issue{}}
	if after, err := archive.Load(ar.Dir); err == nil {
		for _, i := range after.Errors {
			if i.File == file {
				res.Issues = append(res.Issues, i)
			}
		}
	}
	return res, nil
}

func (a *App) NewArchiveResource(kind, name, product string) (string, error) {
	ar, err := a.archive()
	if err != nil {
		return "", err
	}
	return ar.NewResource(kind, name, product)
}

// ArchiveGit runs status, commit (with message), pull or push.
func (a *App) ArchiveGit(action, message string) (string, error) {
	ar, err := a.archive()
	if err != nil {
		return "", err
	}
	switch action {
	case "commit":
		actor := "human/" + strings.ReplaceAll(archive.GitUser(ar.Dir), " ", "-")
		if actor == "human/" {
			actor = "human/desktop"
		}
		rev, err := archive.Commit(ar.Dir, archive.CommitInput{Message: message, Actor: actor})
		if err != nil {
			return "", err
		}
		return "Committed " + rev, nil
	case "pull":
		if _, err := archive.Pull(ar.Dir); err != nil {
			return "", err
		}
		return "Pulled", nil
	case "push":
		if err := archive.Push(ar.Dir); err != nil {
			return "", err
		}
		return "Pushed", nil
	case "fetch":
		st, err := archive.GitStatus(ar.Dir, true)
		if err != nil {
			return "", err
		}
		if st.Behind > 0 {
			return "Behind the remote by " + strconv.Itoa(st.Behind) + " commits; pull to update", nil
		}
		return "Up to date with the remote", nil
	}
	return "", errors.New("Unsupported Git operation")
}

// ScanArchive reads live facts (read-only network access) into the local
// observation cache. kind is domain, cluster, host or empty for all.
func (a *App) ScanArchive(kind string) (*archive.ScanResult, error) {
	ar, err := a.archive()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Minute)
	defer cancel()
	var kinds []string
	if kind != "" {
		kinds = []string{kind}
	}
	return ar.Scan(ctx, archive.ScanInput{Kinds: kinds, Collector: "devopspad-desktop"})
}

// SyncArchiveConnections creates or updates desktop connections from the
// archive's clusters and datastores, through the running app's store.
func (a *App) SyncArchiveConnections(dryRun bool) ([]archive.SyncItem, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	ar, err := a.archive()
	if err != nil {
		return nil, err
	}
	cfg, err := desktopConfigDir()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 5*time.Minute)
	defer cancel()
	return ar.ConnectSync(ctx, a.store, filepath.Join(cfg, "kube"), dryRun)
}
