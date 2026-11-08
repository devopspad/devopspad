package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"devopspad/internal/fleet"
	"devopspad/internal/release"
	"devopspad/internal/workspace"
)

func main() {
	code, out, errOut := run(os.Args[1:])
	fmt.Fprint(os.Stdout, out)
	fmt.Fprint(os.Stderr, errOut)
	os.Exit(code)
}

func run(args []string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := dispatch(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func dispatch(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	case "release":
		return cmdRelease(args[1:], stdout, stderr)
	case "runs":
		return cmdRuns(args[1:], stdout, stderr)
	case "cluster":
		return cmdCluster(args[1:], stdout, stderr)
	case "fleet":
		return cmdFleet(args[1:], stdout, stderr)
	case "config":
		return cmdConfig(args[1:], stdout, stderr)
	case "workload":
		return cmdWorkload(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n\n%s", args[0], usage)
		return 2
	}
}

func cmdRelease(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "validate" {
		fmt.Fprint(stderr, usage)
		return 2
	}
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	file := fs.String("f", "", "release spec file")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if *file == "" || fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: devopspadctl release validate -f <spec.yaml>")
		return 2
	}
	spec, err := release.ParseSpec(*file)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	fmt.Fprintf(stdout, "service=%s\nsha256=%s\n", spec.Service, spec.SHA256())
	return 0
}

func cmdFleet(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 || args[0] != "list" {
		fmt.Fprint(stderr, usage)
		return 2
	}
	dir, err := configDir()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	f, err := fleet.Load(dir, connectionExists(dir))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "fleets: %d\n", len(f.Fleets))
	for _, item := range f.Fleets {
		fmt.Fprintln(stdout, item.Name)
	}
	return 0
}

func configDir() (string, error) {
	if dir := os.Getenv("DEVOPSPAD_CONFIG_DIR"); dir != "" {
		return dir, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "DevOpsPad"), nil
}

func connectionExists(dir string) func(string) bool {
	var ids map[string]struct{}
	return func(id string) bool {
		if ids == nil {
			ids = map[string]struct{}{}
			store, err := workspace.NewStore(dir, stubVault{})
			if err == nil && store.Ready() == nil {
				for _, c := range store.List() {
					ids[c.ID] = struct{}{}
				}
			}
		}
		_, ok := ids[id]
		return ok
	}
}

type stubVault struct{}

func (stubVault) Set(string, string) error   { return nil }
func (stubVault) Get(string) (string, error) { return "", errors.New("not used") }
func (stubVault) Delete(string) error        { return nil }

const usage = `devopspadctl — local cluster access and AI execution records

Usage:
  devopspadctl release validate -f <spec.yaml>
  devopspadctl runs record --project <dir> -f <event.json|->
  devopspadctl runs list --project <dir>
  devopspadctl runs get --project <dir> --id <run-id>
  devopspadctl fleet list
  devopspadctl cluster inspect --connection <name> --namespace <namespace>
  devopspadctl cluster attach --connection <name> --namespace <namespace> --environment <name> [--project <dir>]
  devopspadctl cluster attach --connection <name> --namespace <namespace> --environment <name> --application <name> --selector <labels> [--project <dir>]
  devopspadctl config get --environment <name> --application <name> --configmap <name> --key <key> -o <file|->
  devopspadctl config set --environment <name> --application <name> --configmap <name> --key <key> -f <file|->
                          --expect-revision <resourceVersion> [--dry-run] [--restart --yes [--workload <name>] [--timeout 180s]]
  devopspadctl workload restart --environment <name> --application <name> [--workload <name>] [--yes] [--timeout 180s]

Config:
  DEVOPSPAD_CONFIG_DIR  directory containing fleet.json
                       (default: the DevOpsPad OS config directory)

validate reads only the spec file. It does not open SSH or write a release record.

config and workload resolve the environment and application from the local
project (--project, DEVOPSPAD_PROJECT_DIR, or the desktop's project directory)
and the cluster connection of the same name saved in DevOpsPad. Only ConfigMaps
referenced by the application's workloads can be read or written.
  config get   writes the value to a 0600 file, or to stdout with -o -, and
               prints its resourceVersion and sha256.
  config set   prints a diff, refuses to write if the ConfigMap changed since
               --expect-revision, and records the change (key, line counts and
               hashes only, never the contents). --dry-run prints the diff only.
               --restart rolls the Deployment in the same record.
  workload restart  plans a rolling restart (server dry-run); --yes applies it,
               waits for the rollout and records the result.
`
