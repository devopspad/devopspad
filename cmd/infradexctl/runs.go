package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"infradex/internal/ops"
)

func projectRoot(root string) (string, error) {
	if root == "" {
		root = os.Getenv("INFRADEX_PROJECT_DIR")
	}
	if root == "" {
		dir, err := configDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(dir, "applications")
		b, err := os.ReadFile(filepath.Join(dir, "project-path"))
		if err == nil {
			root = string(b)
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}
	return root, nil
}
func projectStore(root string) (*ops.Store, error) {
	root, err := projectRoot(root)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(root, "project.yaml")); err != nil {
		return nil, errors.New("project.yaml not found; attach an application in Infradex or specify --project")
	}
	return ops.New(root)
}
func cmdRuns(args []string, stdout, stderr io.Writer) int {
	fail := func(err error) int {
		_ = json.NewEncoder(stderr).Encode(map[string]string{"error": err.Error()})
		return 1
	}
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: infradexctl runs record|list|get --project <directory> [-f <event.json>|--id <run-id>]")
		return 2
	}
	fs := flag.NewFlagSet("runs "+args[0], flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("project", "", "local project directory")
	file := fs.String("f", "", "JSON event file; - reads stdin")
	id := fs.String("id", "", "run ID")
	if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 0 {
		return 2
	}
	if args[0] != "record" && args[0] != "get" && args[0] != "list" {
		return 2
	}
	store, err := projectStore(*root)
	if err != nil {
		return fail(err)
	}
	var result any
	switch args[0] {
	case "record":
		if *file == "" || *id != "" {
			return 2
		}
		var reader io.Reader = os.Stdin
		if *file != "-" {
			f, err := os.Open(*file)
			if err != nil {
				return fail(err)
			}
			defer f.Close()
			reader = f
		}
		b, err := io.ReadAll(io.LimitReader(reader, (64<<10)+1))
		if err != nil {
			return fail(err)
		}
		event, err := ops.DecodeRunEvent(b)
		if err != nil {
			return fail(err)
		}
		result, err = store.RecordRunEvent(event)
		if err != nil {
			return fail(err)
		}
	case "get":
		if *id == "" || *file != "" {
			return 2
		}
		result, err = store.GetRun(*id)
		if err != nil {
			return fail(err)
		}
	case "list":
		if *id != "" || *file != "" {
			return 2
		}
		snap, err := store.Read()
		if err != nil {
			return fail(err)
		}
		result = snap.Runs
	}
	if err = json.NewEncoder(stdout).Encode(result); err != nil {
		return fail(err)
	}
	return 0
}
