package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"devopspad/internal/opsrun"
	"devopspad/internal/release"
	"devopspad/internal/workspace"
)

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

// readSecretFiles reads KEY=path pairs; "-" reads stdin (once). Values never
// come from the command line.
func readSecretFiles(pairs []string) (map[string][]byte, error) {
	data := map[string][]byte{}
	usedStdin := false
	for _, pair := range pairs {
		key, path, ok := strings.Cut(pair, "=")
		if !ok || key == "" || path == "" {
			return nil, fmt.Errorf("--from-file %q: use KEY=path (or KEY=- for stdin)", pair)
		}
		if _, dup := data[key]; dup {
			return nil, fmt.Errorf("key %s given twice", key)
		}
		var b []byte
		var err error
		if path == "-" {
			if usedStdin {
				return nil, errors.New("only one key can be read from stdin")
			}
			usedStdin = true
			b, err = io.ReadAll(io.LimitReader(stdin, 1<<20))
		} else {
			b, err = os.ReadFile(path)
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		data[key] = b
	}
	return data, nil
}

func cmdSecretK8s(args []string, stdout, stderr io.Writer) int {
	sub := args[0]
	fs := flag.NewFlagSet("secret "+sub, flag.ContinueOnError)
	fs.SetOutput(stderr)
	environment := fs.String("environment", "", "project environment (the Secret lives in its namespace)")
	application := fs.String("application", "", "project application the Secret belongs to (for the record)")
	name := fs.String("name", "", "Secret name")
	root := fs.String("project", "", "local project directory")
	kind := fs.String("type", "generic", "generic or docker-registry")
	var files multiFlag
	fs.Var(&files, "from-file", "KEY=path, repeatable; KEY=- reads stdin")
	server := fs.String("docker-server", "", "docker-registry: registry host, e.g. ghcr.io")
	username := fs.String("docker-username", "", "docker-registry: user name")
	passwordStdin := fs.Bool("password-stdin", false, "docker-registry: read the password or token from stdin")
	yes := fs.Bool("yes", false, "apply; without it only the plan is printed")
	if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 0 {
		return 2
	}
	if *environment == "" || *application == "" || *name == "" {
		fmt.Fprintln(stderr, "usage: devopspadcli secret set|keys --environment <env> --application <app> --name <secret> ...")
		return 2
	}
	t, err := cliTarget(*root, *environment, *application)
	if err != nil {
		return jsonFail(stderr, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cl, closeFn, err := workspace.OpenCluster(ctx, t.Connection)
	if err != nil {
		return jsonFail(stderr, err)
	}
	defer closeFn()
	if sub == "keys" {
		info, err := release.DescribeSecret(ctx, cl, t.Environment.Namespace, *name)
		if err != nil {
			return jsonFail(stderr, err)
		}
		_ = json.NewEncoder(stdout).Encode(info)
		return 0
	}
	var data map[string][]byte
	switch {
	case *kind == "docker-registry" && *passwordStdin:
		if len(files) > 0 {
			fmt.Fprintln(stderr, "use either --password-stdin or --from-file .dockerconfigjson=path")
			return 2
		}
		pw, err := io.ReadAll(io.LimitReader(stdin, 64<<10))
		if err != nil {
			return jsonFail(stderr, err)
		}
		cfg, err := release.DockerConfigJSON(*server, *username, strings.TrimRight(string(pw), "\r\n"))
		if err != nil {
			return jsonFail(stderr, err)
		}
		data = map[string][]byte{".dockerconfigjson": cfg}
	case len(files) > 0:
		if data, err = readSecretFiles(files); err != nil {
			return jsonFail(stderr, err)
		}
	default:
		fmt.Fprintln(stderr, "give the values with --from-file KEY=path (repeatable), or for docker-registry --docker-server --docker-username --password-stdin")
		return 2
	}
	plan, err := release.PrepareSecret(ctx, cl, t.Environment.Namespace, *name, *kind, data)
	if err != nil {
		return jsonFail(stderr, err)
	}
	var keys []string
	for _, k := range plan.Keys {
		keys = append(keys, k.Key+" "+k.Change)
	}
	verb := "update"
	if plan.Create {
		verb = "create"
	}
	fmt.Fprintf(stderr, "Plan: %s Secret %s/%s (%s): %s; server dry-run passed\n", verb, plan.Namespace, plan.Name, plan.Type, strings.Join(keys, ", "))
	if !*yes {
		_ = json.NewEncoder(stdout).Encode(plan)
		fmt.Fprintln(stderr, "Plan only; re-run with --yes to apply")
		return 0
	}
	r, err := opsrun.ApplySecret(ctx, t, plan)
	return printRecord(r, err, stdout, stderr)
}

// cmdWorkloadPatch changes pull secrets and Secret-backed env of a Deployment.
func cmdWorkloadPatch(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("workload patch", flag.ContinueOnError)
	fs.SetOutput(stderr)
	environment := fs.String("environment", "", "project environment")
	application := fs.String("application", "", "project application")
	workload := fs.String("workload", "", "Deployment (default: the application name)")
	container := fs.String("container", "", "container for envFrom/env changes")
	root := fs.String("project", "", "local project directory")
	yes := fs.Bool("yes", false, "apply the plan; without it only the plan is printed")
	timeout := fs.Duration("timeout", maxRolloutTimeout, "rollout timeout (max 180s)")
	var addPull, removePull, addFrom, removeFrom, setEnv, removeEnv multiFlag
	fs.Var(&addPull, "add-image-pull-secret", "Secret name, repeatable")
	fs.Var(&removePull, "remove-image-pull-secret", "Secret name, repeatable")
	fs.Var(&addFrom, "add-env-from-secret", "Secret name, repeatable")
	fs.Var(&removeFrom, "remove-env-from-secret", "Secret name, repeatable")
	fs.Var(&setEnv, "set-env-secret", "NAME=secret:key, repeatable")
	fs.Var(&removeEnv, "remove-env", "env name backed by a secretKeyRef, repeatable")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return 2
	}
	if *environment == "" || *application == "" {
		fmt.Fprintln(stderr, "workload patch requires --environment and --application")
		return 2
	}
	if *timeout <= 0 || *timeout > maxRolloutTimeout {
		fmt.Fprintln(stderr, "--timeout must be between 1s and 180s; the release lock lasts 300s")
		return 2
	}
	if *workload == "" {
		*workload = *application
	}
	patch := release.WorkloadPatch{Container: *container, AddImagePullSecrets: addPull, RemoveImagePullSecrets: removePull, AddEnvFromSecrets: addFrom, RemoveEnvFromSecrets: removeFrom, RemoveEnv: removeEnv}
	for _, v := range setEnv {
		name, ref, ok1 := strings.Cut(v, "=")
		secret, key, ok2 := strings.Cut(ref, ":")
		if !ok1 || !ok2 {
			fmt.Fprintf(stderr, "--set-env-secret %q: use NAME=secret:key\n", v)
			return 2
		}
		patch.SetEnvSecretRefs = append(patch.SetEnvSecretRefs, release.EnvSecretRef{Name: name, Secret: secret, Key: key})
	}
	t, err := cliTarget(*root, *environment, *application)
	if err != nil {
		return jsonFail(stderr, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cl, closeFn, err := workspace.OpenCluster(ctx, t.Connection)
	if err != nil {
		return jsonFail(stderr, err)
	}
	plan, err := release.PrepareDesktopPatch(ctx, cl, t.Environment.Namespace, *workload, t.Application.Selector, patch)
	closeFn()
	if err != nil {
		return jsonFail(stderr, err)
	}
	fmt.Fprintf(stderr, "Plan: deployment %s in %s/%s: %s (rolls the pods; server dry-run passed)\n", plan.Workload, t.Environment.Name, plan.Namespace, release.ChangeSummary(plan))
	if !*yes {
		fmt.Fprintln(stderr, "Plan only; re-run with --yes to apply")
		return 0
	}
	record := opsrun.PlanRecord(t, plan, "", "")
	applyCtx, cancelApply := context.WithTimeout(context.Background(), *timeout)
	defer cancelApply()
	r, err := opsrun.Apply(applyCtx, t.Store, t.Connection, plan, record, func(step string) { fmt.Fprintln(stderr, step) })
	return printRecord(r, err, stdout, stderr)
}
