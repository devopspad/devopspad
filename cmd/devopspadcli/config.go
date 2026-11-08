package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"devopspad/internal/ops"
	"devopspad/internal/opsrun"
	"devopspad/internal/release"
	"devopspad/internal/workspace"
	"github.com/google/uuid"
	"github.com/pmezard/go-difflib/difflib"
)

const maxRolloutTimeout = 180 * time.Second

// cliTarget resolves an application the same way the desktop does.
func cliTarget(root, environment, application string) (opsrun.Target, error) {
	store, err := projectStore(root)
	if err != nil {
		return opsrun.Target{}, err
	}
	dir, err := configDir()
	if err != nil {
		return opsrun.Target{}, err
	}
	conns, err := workspace.NewStore(dir, workspace.Keychain{})
	if err != nil {
		return opsrun.Target{}, err
	}
	if err = conns.Ready(); err != nil {
		return opsrun.Target{}, err
	}
	return opsrun.Resolve(store, conns, environment, application)
}

type configMeta struct {
	ConfigMap       string `json:"configMap"`
	Namespace       string `json:"namespace"`
	Key             string `json:"key"`
	ResourceVersion string `json:"resourceVersion"`
	SHA256          string `json:"sha256"`
	Bytes           int    `json:"bytes"`
	File            string `json:"file,omitempty"`
}

func cmdConfig(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && (args[0] == "pull" || args[0] == "diff" || args[0] == "push") {
		return cmdConfigArchive(args[0], args[1:], stdout, stderr)
	}
	if len(args) == 0 || (args[0] != "get" && args[0] != "set") {
		fmt.Fprint(stderr, usage)
		return 2
	}
	fs := flag.NewFlagSet("config "+args[0], flag.ContinueOnError)
	fs.SetOutput(stderr)
	environment := fs.String("environment", "", "project environment")
	application := fs.String("application", "", "project application")
	configMap := fs.String("configmap", "", "ConfigMap referenced by the application's workloads")
	key := fs.String("key", "", "ConfigMap data key")
	root := fs.String("project", "", "local project directory")
	output := fs.String("o", "", "get: file to write (0600), or - for stdout")
	input := fs.String("f", "", "set: file with the new value, or - for stdin")
	expect := fs.String("expect-revision", "", "set: resourceVersion printed by config get")
	dryRun := fs.Bool("dry-run", false, "set: print the diff only")
	restart := fs.Bool("restart", false, "set: roll the workload after the update, in the same record")
	workload := fs.String("workload", "", "set --restart: Deployment to roll (default: the application name)")
	yes := fs.Bool("yes", false, "set --restart: confirm the restart")
	timeout := fs.Duration("timeout", maxRolloutTimeout, "set --restart: rollout timeout (max 180s)")
	if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 0 {
		return 2
	}
	if *environment == "" || *application == "" || *configMap == "" || *key == "" {
		fmt.Fprintf(stderr, "config %s requires --environment, --application, --configmap and --key\n", args[0])
		return 2
	}
	if args[0] == "get" {
		if *output == "" {
			fmt.Fprintln(stderr, "config get requires -o <file> (written 0600) or -o - for stdout; contents can hold secrets")
			return 2
		}
		return configGet(*root, *environment, *application, *configMap, *key, *output, stdout, stderr)
	}
	if *input == "" || *expect == "" {
		fmt.Fprintln(stderr, "config set requires -f <file|-> and --expect-revision <resourceVersion from config get>")
		return 2
	}
	if *restart && !*dryRun && !*yes {
		fmt.Fprintln(stderr, "config set --restart requires --yes; nothing was written")
		return 2
	}
	if *timeout <= 0 || *timeout > maxRolloutTimeout {
		fmt.Fprintln(stderr, "--timeout must be between 1s and 180s; the release lock lasts 300s")
		return 2
	}
	if *workload == "" {
		*workload = *application
	}
	value, err := readValue(*input)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return configSet(configChange{root: *root, environment: *environment, application: *application, configMap: *configMap, key: *key, value: value, expect: *expect, dryRun: *dryRun, restart: *restart, workload: *workload, timeout: *timeout}, stdout, stderr)
}

func configGet(root, environment, application, name, key, output string, stdout, stderr io.Writer) int {
	t, err := cliTarget(root, environment, application)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cm, err := workspace.ReadApplicationConfigMap(ctx, t.Connection, t.Environment.Namespace, t.Application.Selector, name)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	value, ok := cm.Data[key]
	if !ok {
		fmt.Fprintf(stderr, "ConfigMap %s has no text key %s; keys: %s\n", name, key, strings.Join(sortedKeys(cm.Data), ", "))
		return 1
	}
	meta := configMeta{ConfigMap: name, Namespace: cm.Namespace, Key: key, ResourceVersion: cm.ResourceVersion, SHA256: digest(value), Bytes: len(value)}
	metaOut := stdout
	if output == "-" {
		fmt.Fprint(stdout, value)
		metaOut = stderr
	} else {
		if err = writePrivate(output, value); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		meta.File = output
	}
	if err = json.NewEncoder(metaOut).Encode(meta); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

type configChange struct {
	root, environment, application, configMap, key, value, expect, workload string
	dryRun, restart                                                         bool
	timeout                                                                 time.Duration
}

func configSet(ch configChange, stdout, stderr io.Writer) int {
	t, err := cliTarget(ch.root, ch.environment, ch.application)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cm, err := workspace.ReadApplicationConfigMap(ctx, t.Connection, t.Environment.Namespace, t.Application.Selector, ch.configMap)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if cm.ResourceVersion != ch.expect {
		fmt.Fprintf(stderr, "ConfigMap %s changed since revision %s (now %s); run config get again and reapply your edit. Nothing was written.\n", ch.configMap, ch.expect, cm.ResourceVersion)
		return 1
	}
	if cm.Immutable {
		fmt.Fprintf(stderr, "ConfigMap %s is immutable\n", ch.configMap)
		return 1
	}
	before, ok := cm.Data[ch.key]
	if !ok {
		fmt.Fprintf(stderr, "ConfigMap %s has no text key %s; keys: %s\n", ch.configMap, ch.key, strings.Join(sortedKeys(cm.Data), ", "))
		return 1
	}
	if before == ch.value {
		fmt.Fprintln(stderr, "No changes; nothing was written")
		return 0
	}
	diff, summary := lineDiff(ch.key, before, ch.value)
	fmt.Fprint(stdout, diff)
	var plan release.DesktopPlan
	if ch.restart {
		// Plan before writing so a paused or foreign Deployment stops the change early.
		cl, close, err := workspace.OpenCluster(ctx, t.Connection)
		if err == nil {
			plan, err = release.PrepareDesktopAction(ctx, cl, t.Environment.Namespace, ch.workload, t.Application.Selector, "restart", 0)
			close()
		}
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	if ch.dryRun {
		if ch.restart {
			fmt.Fprintf(stderr, "Restart plan for deployment %s passed the server dry-run (%d replicas)\n", ch.workload, plan.Replicas)
		}
		fmt.Fprintln(stderr, "Dry run: nothing was written")
		return 0
	}
	r := ops.Record{ID: uuid.NewString(), Application: t.Application.Name, Environment: t.Environment.Name, Connection: t.Environment.Connection, Namespace: t.Environment.Namespace, Workload: ch.configMap, Steps: []string{}}
	if ch.restart {
		r = opsrun.PlanRecord(t, plan, "", "")
	}
	r.Action = "config"
	r.Version = ch.configMap + "/" + ch.key
	r.Status = "running"
	r.StartedAt = time.Now().UTC().Format(time.RFC3339Nano)
	r.Steps = append(r.Steps, "Checked ConfigMap revision "+cm.ResourceVersion)
	if err = t.Store.WriteRecord(r); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	data := make(map[string]string, len(cm.Data))
	for k, v := range cm.Data {
		data[k] = v
	}
	data[ch.key] = ch.value
	updated, err := workspace.SaveConfigMap(ctx, t.Connection, workspace.ConfigMapUpdate{Ref: workspace.ClusterRef{Namespace: cm.Namespace, Resource: "configmaps", Name: cm.Name, UID: cm.UID}, ResourceVersion: cm.ResourceVersion, Data: data})
	if err == nil {
		// Record only a hash and line counts: ConfigMap contents can hold secrets.
		r.Steps = append(r.Steps, fmt.Sprintf("Updated %s key %s: %s (sha256 %s → %s, revision %s → %s)", ch.configMap, ch.key, summary, digest(before)[:12], digest(ch.value)[:12], cm.ResourceVersion, revisionOf(updated)))
		fmt.Fprintln(stderr, r.Steps[len(r.Steps)-1])
	}
	if err != nil || !ch.restart {
		r, err = opsrun.Finish(t.Store, r, err, "ConfigMap updated")
		return printRecord(r, err, stdout, stderr)
	}
	applyCtx, cancelApply := context.WithTimeout(context.Background(), ch.timeout)
	defer cancelApply()
	r, err = opsrun.Apply(applyCtx, t.Store, t.Connection, plan, r, func(step string) { fmt.Fprintln(stderr, step) })
	return printRecord(r, err, stdout, stderr)
}

func cmdWorkload(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "patch" {
		return cmdWorkloadPatch(args[1:], stdout, stderr)
	}
	if len(args) == 0 || args[0] != "restart" {
		fmt.Fprint(stderr, usage)
		return 2
	}
	fs := flag.NewFlagSet("workload restart", flag.ContinueOnError)
	fs.SetOutput(stderr)
	environment := fs.String("environment", "", "project environment")
	application := fs.String("application", "", "project application")
	workload := fs.String("workload", "", "Deployment to roll (default: the application name)")
	root := fs.String("project", "", "local project directory")
	yes := fs.Bool("yes", false, "apply the plan; without it only the plan is printed")
	timeout := fs.Duration("timeout", maxRolloutTimeout, "rollout timeout (max 180s)")
	if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 0 {
		return 2
	}
	if *environment == "" || *application == "" {
		fmt.Fprintln(stderr, "workload restart requires --environment and --application")
		return 2
	}
	if *timeout <= 0 || *timeout > maxRolloutTimeout {
		fmt.Fprintln(stderr, "--timeout must be between 1s and 180s; the release lock lasts 300s")
		return 2
	}
	if *workload == "" {
		*workload = *application
	}
	t, err := cliTarget(*root, *environment, *application)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cl, close, err := workspace.OpenCluster(ctx, t.Connection)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	plan, err := release.PrepareDesktopAction(ctx, cl, t.Environment.Namespace, *workload, t.Application.Selector, "restart", 0)
	close()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stderr, "Plan: rolling restart of deployment %s in %s/%s (%d replicas); server dry-run passed\n", plan.Workload, t.Environment.Name, plan.Namespace, plan.Replicas)
	if !*yes {
		fmt.Fprintln(stderr, "Plan only; re-run with --yes to restart")
		return 0
	}
	applyCtx, cancelApply := context.WithTimeout(context.Background(), *timeout)
	defer cancelApply()
	r, err := opsrun.Apply(applyCtx, t.Store, t.Connection, plan, opsrun.PlanRecord(t, plan, "", ""), func(step string) { fmt.Fprintln(stderr, step) })
	return printRecord(r, err, stdout, stderr)
}

// printRecord writes the final record; a failed operation exits non-zero.
func printRecord(r ops.Record, err error, stdout, stderr io.Writer) int {
	if encodeErr := json.NewEncoder(stdout).Encode(r); encodeErr != nil {
		fmt.Fprintln(stderr, encodeErr)
		return 1
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if r.Status != "succeeded" {
		fmt.Fprintln(stderr, r.Message)
		return 1
	}
	return 0
}

func readValue(path string) (string, error) {
	var r io.Reader = os.Stdin
	if path != "-" {
		f, err := os.Open(path)
		if err != nil {
			return "", err
		}
		defer f.Close()
		r = f
	}
	b, err := io.ReadAll(io.LimitReader(r, (1<<20)+1))
	if err != nil {
		return "", err
	}
	if len(b) > 1<<20 {
		return "", errors.New("The new value exceeds 1 MiB")
	}
	if !utf8.Valid(b) {
		return "", errors.New("The new value is not UTF-8 text")
	}
	return string(b), nil
}

// writePrivate creates or replaces a file readable only by its owner.
func writePrivate(path, value string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	if err = f.Chmod(0600); err == nil {
		_, err = f.WriteString(value)
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
}

// lineDiff returns a unified diff and a summary that names no content.
func lineDiff(key, before, after string) (string, string) {
	a, b := difflib.SplitLines(before), difflib.SplitLines(after)
	text, _ := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{A: a, B: b, FromFile: "a/" + key, ToFile: "b/" + key, Context: 3})
	added, removed := 0, 0
	for _, op := range difflib.NewMatcher(a, b).GetOpCodes() {
		if op.Tag == 'r' || op.Tag == 'd' {
			removed += op.I2 - op.I1
		}
		if op.Tag == 'r' || op.Tag == 'i' {
			added += op.J2 - op.J1
		}
	}
	return text, fmt.Sprintf("%d line(s) added, %d removed", added, removed)
}

func digest(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func revisionOf(definition string) string {
	var obj struct {
		Metadata struct {
			ResourceVersion string `json:"resourceVersion"`
		} `json:"metadata"`
	}
	if json.Unmarshal([]byte(definition), &obj) != nil || obj.Metadata.ResourceVersion == "" {
		return "unknown"
	}
	return obj.Metadata.ResourceVersion
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
