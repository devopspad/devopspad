package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"time"

	"devopspad/internal/opsrun"
	"devopspad/internal/workspace"
)

type nginxMeta struct {
	Path     string `json:"path"`
	Revision string `json:"revision"`
	Bytes    int    `json:"bytes"`
	File     string `json:"file,omitempty"`
}

func cmdNginx(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || (args[0] != "get" && args[0] != "set") {
		fmt.Fprintln(stderr, "usage: devopspadcli nginx get|set --environment <env> --application <app> ...")
		return 2
	}
	fs := flag.NewFlagSet("nginx "+args[0], flag.ContinueOnError)
	fs.SetOutput(stderr)
	environment := fs.String("environment", "", "project environment")
	application := fs.String("application", "", "project application")
	root := fs.String("project", "", "local project directory")
	output := fs.String("o", "", "get: write to this file (0600), or - for stdout")
	file := fs.String("f", "", "set: new file contents, or - for stdin")
	expect := fs.String("expect-revision", "", "set: revision printed by nginx get")
	dry := fs.Bool("dry-run", false, "set: print the diff only")
	if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 0 {
		return 2
	}
	if *environment == "" || *application == "" || (args[0] == "get" && *output == "") || (args[0] == "set" && (*file == "" || *expect == "")) {
		fmt.Fprintln(stderr, "usage: devopspadcli nginx get --environment <env> --application <app> -o <file|->\n       devopspadcli nginx set --environment <env> --application <app> -f <file|-> --expect-revision <sha256> [--dry-run]")
		return 2
	}
	t, err := cliTarget(*root, *environment, *application)
	if err != nil {
		return jsonFail(stderr, err)
	}
	if t.Application.NginxPath == "" {
		return jsonFail(stderr, fmt.Errorf("application %s has no nginxPath; set it with devopspadcli app set --application %s --nginx-path /etc/nginx/conf.d/<site>.conf", *application, *application))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	current, err := workspace.ReadNginx(ctx, t.Connection, t.Application.NginxPath)
	if err != nil {
		return jsonFail(stderr, err)
	}
	if args[0] == "get" {
		meta := nginxMeta{Path: current.Path, Revision: current.Revision, Bytes: len(current.Text)}
		if *output == "-" {
			fmt.Fprint(stdout, current.Text)
			_ = json.NewEncoder(stderr).Encode(meta)
			return 0
		}
		if err := writePrivate(*output, current.Text); err != nil {
			return jsonFail(stderr, err)
		}
		meta.File = *output
		_ = json.NewEncoder(stdout).Encode(meta)
		return 0
	}
	next, err := readValue(*file)
	if err != nil {
		return jsonFail(stderr, err)
	}
	if current.Revision != *expect {
		return jsonFail(stderr, fmt.Errorf("%s changed since revision %s (now %s); run nginx get again", current.Path, *expect, current.Revision))
	}
	if next == current.Text {
		fmt.Fprintln(stderr, "No change; nothing written")
		return 0
	}
	diff, counts := lineDiff(current.Path, current.Text, next)
	fmt.Fprint(stdout, diff)
	if *dry {
		fmt.Fprintf(stderr, "Dry run: %s; nothing written\n", counts)
		return 0
	}
	summary := fmt.Sprintf("%s, sha256 %s -> %s", counts, digest(current.Text)[:12], digest(next)[:12])
	r, result, err := opsrun.SaveNginx(ctx, t, workspace.NginxUpdate{Revision: *expect, Text: next}, summary)
	if err == nil {
		fmt.Fprintf(stderr, "%s; new revision %s\n", result.Message, result.Revision)
	}
	return printRecord(r, err, stdout, stderr)
}

// cmdApp edits project.yaml application settings.
func cmdApp(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "set" {
		fmt.Fprintln(stderr, "usage: devopspadcli app set --application <app> --nginx-path </etc/nginx/conf.d/site.conf | \"\">")
		return 2
	}
	fs := flag.NewFlagSet("app set", flag.ContinueOnError)
	fs.SetOutput(stderr)
	application := fs.String("application", "", "project application")
	nginxPath := fs.String("nginx-path", "\x00", "host Nginx file under /etc/nginx/conf.d/ (empty removes it)")
	root := fs.String("project", "", "local project directory")
	if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 0 || *application == "" || *nginxPath == "\x00" {
		fmt.Fprintln(stderr, "usage: devopspadcli app set --application <app> --nginx-path </etc/nginx/conf.d/site.conf | \"\">")
		return 2
	}
	store, err := projectStore(*root)
	if err != nil {
		return jsonFail(stderr, err)
	}
	snap, err := store.SetNginxPath(*application, *nginxPath)
	if err != nil {
		return jsonFail(stderr, err)
	}
	for _, a := range snap.Project.Applications {
		if a.Name == *application {
			_ = json.NewEncoder(stdout).Encode(a)
		}
	}
	return 0
}
