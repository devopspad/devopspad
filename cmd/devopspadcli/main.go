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
	case "version", "--version":
		return cmdVersion(args[1:], stdout, stderr)
	case "archive":
		return cmdArchive(args[1:], stdout, stderr)
	case "get":
		return cmdGet(args[1:], stdout, stderr)
	case "describe":
		return cmdDescribe(args[1:], stdout, stderr)
	case "refs":
		return cmdRefs(args[1:], stdout, stderr)
	case "schema":
		return cmdSchema(args[1:], stdout, stderr)
	case "new":
		return cmdNew(args[1:], stdout, stderr)
	case "report":
		return cmdReport(args[1:], stdout, stderr)
	case "secrets":
		return cmdSecrets(args[1:], stdout, stderr)
	case "secret":
		if len(args) < 2 || (args[1] != "set" && args[1] != "keys") {
			fmt.Fprintln(stderr, "usage: devopspadcli secret set|keys --environment <env> --application <app> --name <secret> ...")
			return 2
		}
		return cmdSecretK8s(args[1:], stdout, stderr)
	case "note":
		return cmdNote(args[1:], stdout, stderr)
	case "skill":
		return cmdSkill(args[1:], stdout, stderr)
	case "scan":
		return cmdScan(args[1:], stdout, stderr)
	case "diff":
		return cmdDiff(args[1:], stdout, stderr)
	case "connect":
		return cmdConnect(args[1:], stdout, stderr)
	case "job":
		return cmdJob(args[1:], stdout, stderr)
	case "nginx":
		return cmdNginx(args[1:], stdout, stderr)
	case "edge":
		return cmdEdge(args[1:], stdout, stderr)
	case "app":
		return cmdApp(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n\n%s", args[0], usage)
		return 2
	}
}

func cmdRelease(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && (args[0] == "plan" || args[0] == "apply" || args[0] == "image") {
		return cmdReleaseOps(args[0], args[1:], stdout, stderr)
	}
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
		fmt.Fprintln(stderr, "usage: devopspadcli release validate -f <spec.yaml>")
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

const usage = `devopspadcli — OPC archive, local cluster access and AI execution records

Archive (a Git repository describing every product, service, host, domain and account):
  devopspadcli archive init [dir] [--name <name>]      create an archive, Git repo and pre-commit hook
  devopspadcli archive use <dir>                       make <dir> the default archive
  devopspadcli archive validate [--staged]             check schema, references and literal secrets
  devopspadcli archive status [--fetch]                Git and validation state
  devopspadcli archive commit -m "<type>(<scope>): <summary>" [--actor ai/<name>] [--session <id>]
  devopspadcli archive pull | push | index
  devopspadcli archive import [--workspace] [--fleet] [--project <dir> [--product <name>]] [--cloudflare] [--dry-run]
  devopspadcli get <kind> [name] [-l <selector>]       kinds: provider host cluster datastore registry domain product service
  devopspadcli describe <ref>                          e.g. service:heartie/admin, host:hk-1
  devopspadcli refs <ref>                              what references <ref>
  devopspadcli new <kind> <name> [--product <name>]    write a commented skeleton
  devopspadcli schema [kind]                           JSON Schema of a kind
  devopspadcli report matrix [-l <selector>] [--stage <stage>]
  devopspadcli report renewals [--within <days>|--all] [--observed] | report cost | report orphans
  devopspadcli scan [<ref> ...] [--kind domain|cluster|host]
                    domains: Cloudflare zone/DNS/expiry, RDAP, TLS certificates; clusters: nodes and
                    workloads; hosts: OS, size, services, Nginx config hashes (all read-only)
  devopspadcli diff [<ref> ...]                        archive vs last scan
  devopspadcli edge routes list|add|remove --zone <zone> [--pattern '<zone>/path*'] [--script <worker>|none] [--yes]
  devopspadcli edge dns list|set --zone <zone> [--name <n> --type A --content <v>|--target host:<h>] [--proxied] [--yes]
                                                       Cloudflare Workers routes and DNS, recorded in the archive
  devopspadcli config diff|pull|push <product>/<service>@<env> [--name <config>] [--yes]
                                                       archive configs/ vs live Nginx file or ConfigMap key;
                                                       ${secret:...} placeholders are rendered locally, never shown
  devopspadcli connect sync [--dry-run]                create/update desktop connections from clusters and datastores
                                                       (close the desktop app first, or use its Archive page)
  devopspadcli secrets check                           which secret references resolve on this machine
  devopspadcli secrets set keychain:<account>          store a secret (read from stdin) in the OS keychain
  devopspadcli note add --refs <ref>[,<ref>] -m "<what was done>" [--actor ai/<name>]
  devopspadcli skill install [--scope user|archive] | skill print [file]
  devopspadcli version
  Archive commands accept --archive <dir> (default: DEVOPSPAD_ARCHIVE, the enclosing
  archive, or the last one used) and -o text|json|yaml. They never print secret
  values. Only scan, connect sync and import --cloudflare make (read-only) network
  calls; scan results stay in .devopspad/observed and are never committed.

Desktop project, releases and records:
  devopspadcli release plan --environment <env> (--application <app> --container <c> [--workload <w>] | --target <app>:<workload>:<container> ...)
                            (--artifact <id|run id> | --image <ref@sha256:...>) [--migrate job.yaml [--migrate-timeout 180s]]
  devopspadcli release apply --plan <plan id> [--timeout 180s]
  devopspadcli release image <same flags as plan> --yes [--timeout 180s]
  devopspadcli job run --environment <env> --application <app> -f job.yaml --image <ref@sha256:...> [--wait 180s] [--yes]
  devopspadcli release validate -f <spec.yaml>
  devopspadcli runs record --project <dir> -f <event.json|->
  devopspadcli runs list --project <dir>
  devopspadcli runs get --project <dir> --id <run-id>
  devopspadcli fleet list
  devopspadcli cluster inspect --connection <name> --namespace <namespace>
  devopspadcli cluster attach --connection <name> --namespace <namespace> --environment <name> [--project <dir>]
  devopspadcli cluster attach --connection <name> --namespace <namespace> --environment <name> --application <name> --selector <labels> [--project <dir>]
  devopspadcli config get --environment <name> --application <name> --configmap <name> --key <key> -o <file|->
  devopspadcli config set --environment <name> --application <name> --configmap <name> --key <key> -f <file|->
                          --expect-revision <resourceVersion> [--dry-run] [--restart --yes [--workload <name>] [--timeout 180s]]
  devopspadcli nginx get --environment <name> --application <name> -o <file|->
  devopspadcli nginx set --environment <name> --application <name> -f <file|-> --expect-revision <sha256> [--dry-run]
  devopspadcli app set --application <name> --nginx-path </etc/nginx/conf.d/site.conf | "">
  devopspadcli secret set --environment <name> --application <name> --name <secret> [--type generic] --from-file KEY=path ... [--yes]
  devopspadcli secret set ... --type docker-registry --docker-server <host> --docker-username <user> --password-stdin [--yes]
  devopspadcli secret keys --environment <name> --application <name> --name <secret>       (key names only, never values)
  devopspadcli workload patch --environment <name> --application <name> [--workload <name>] [--add-image-pull-secret <s>]
                          [--container <c> --add-env-from-secret <s> | --set-env-secret NAME=secret:key | --remove-env NAME ...] [--yes]
  devopspadcli workload restart --environment <name> --application <name> [--workload <name>] [--yes] [--timeout 180s]

Config:
  DEVOPSPAD_CONFIG_DIR  directory containing workspace.json, fleet.json and archive-path
                       (default: the DevOpsPad OS config directory)
  DEVOPSPAD_ARCHIVE     archive directory

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
