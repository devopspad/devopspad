package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"devopspad/internal/archive"
)

// cmdConfigArchive syncs archive configs/ with live Nginx files and ConfigMaps.
func cmdConfigArchive(sub string, args []string, stdout, stderr io.Writer) int {
	fs, c := newFlags("config "+sub, stdout, stderr)
	name := fs.String("name", "", "config name when the service has several in that environment")
	yes := fs.Bool("yes", false, "push: apply; without it only the diff is printed")
	actor := fs.String("actor", "", "push: human/<name> or ai/<name> (default: human/<git user.name>)")
	session := fs.String("session", "", "push: AI session or task ID")
	pos, err := parseInterleaved(fs, args)
	if err != nil || len(pos) != 1 {
		fmt.Fprintf(stderr, "usage: devopspadcli config %s <product>/<service>@<environment> [--name <config>]%s\n", sub, map[bool]string{true: " [--yes]", false: ""}[sub == "push"])
		return 2
	}
	ref, err := archive.ParseConfigRef(pos[0], *name)
	if err != nil {
		return c.fail(err)
	}
	a, err := c.load()
	if err != nil {
		return c.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var res archive.ConfigResult
	switch sub {
	case "diff":
		res, err = a.ConfigDiff(ctx, ref)
	case "pull":
		res, err = a.ConfigPull(ctx, ref)
	case "push":
		if *actor == "" {
			*actor = "human/" + slugName(archive.GitUser(a.Dir))
		}
		res, _, err = a.ConfigPush(ctx, ref, archive.OpInput{Actor: *actor, Session: *session, Revision: archive.Revision(a.Dir)}, *yes)
	}
	if err != nil {
		if res.Diff != "" && !c.json() {
			fmt.Fprint(stdout, res.Diff)
		}
		return c.fail(err)
	}
	if c.print(res) >= 0 {
		if sub == "diff" && !res.InSync {
			return 1
		}
		return 0
	}
	switch {
	case res.InSync:
		fmt.Fprintf(stdout, "%s matches %s\n", res.File, res.Target)
	case sub == "diff":
		fmt.Fprint(stdout, res.Diff)
		fmt.Fprintf(stderr, "%s differs from %s: %d line(s) only live, %d only in the archive\n", res.File, res.Target, res.Added, res.Removed)
		return 1
	case sub == "pull":
		fmt.Fprint(stdout, res.Diff)
		fmt.Fprintf(stderr, "Saved the live config to %s; review and commit it with devopspadcli archive commit -m \"config(%s): pull %s\"\n", res.File, ref.Service, res.Name)
	case sub == "push" && res.Record == "":
		fmt.Fprint(stdout, res.Diff)
		fmt.Fprintf(stderr, "Plan: write %s to %s. Lines marked - are the archive copy that will be written; lines marked + exist only live and will be replaced. Re-run with --yes to apply\n", res.File, res.Target)
	default:
		fmt.Fprintf(stderr, "Pushed %s to %s; recorded in %s (commit it with devopspadcli archive commit)\n", res.File, res.Target, res.Record)
	}
	if len(res.MissingSecrets) > 0 {
		fmt.Fprintf(stderr, "note: secret references not available here: %v\n", res.MissingSecrets)
	}
	return 0
}
