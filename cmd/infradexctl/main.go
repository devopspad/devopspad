package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"infradex/internal/fleet"
	"infradex/internal/release"
	"infradex/internal/workspace"
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
		fmt.Fprintln(stderr, "usage: infradexctl release validate -f <spec.yaml>")
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
	if dir := os.Getenv("INFRADEX_CONFIG_DIR"); dir != "" {
		return dir, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "Infradex"), nil
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

const usage = `infradexctl — local cluster access and AI execution records

Usage:
  infradexctl release validate -f <spec.yaml>
  infradexctl runs record --project <dir> -f <event.json|->
  infradexctl runs list --project <dir>
  infradexctl runs get --project <dir> --id <run-id>
  infradexctl fleet list
  infradexctl cluster inspect --connection <name> --namespace <namespace>
  infradexctl cluster attach --connection <name> --namespace <namespace> --environment <name> [--project <dir>]
  infradexctl cluster attach --connection <name> --namespace <namespace> --environment <name> --application <name> --selector <labels> [--project <dir>]

Config:
  INFRADEX_CONFIG_DIR  directory containing fleet.json
                       (default: the Infradex OS config directory)

validate reads only the spec file. It does not open SSH or write a release record.
`
