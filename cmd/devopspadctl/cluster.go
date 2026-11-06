package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"devopspad/internal/ops"
	"devopspad/internal/workspace"
)

func savedCluster(name string) (workspace.Connection, error) {
	dir, err := configDir()
	if err != nil {
		return workspace.Connection{}, err
	}
	store, err := workspace.NewStore(dir, workspace.Keychain{})
	if err != nil {
		return workspace.Connection{}, err
	}
	id := ""
	for _, c := range store.List() {
		if (c.ID == name || c.Name == name) && (c.Kind == "k3s" || c.Kind == "k8s") {
			if id != "" {
				return workspace.Connection{}, errors.New("cluster connection name is ambiguous")
			}
			id = c.ID
		}
	}
	if id == "" {
		return workspace.Connection{}, errors.New("saved cluster connection not found")
	}
	return store.Get(id)
}

func cmdCluster(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || (args[0] != "inspect" && args[0] != "attach") {
		fmt.Fprintln(stderr, "usage: devopspadctl cluster inspect --connection <name> --namespace <namespace>")
		return 2
	}
	fs := flag.NewFlagSet("cluster inspect", flag.ContinueOnError)
	fs.SetOutput(stderr)
	name := fs.String("connection", "", "saved cluster connection name or ID")
	namespace := fs.String("namespace", "default", "specific namespace")
	root := fs.String("project", "", "local project directory")
	environment := fs.String("environment", "", "local environment name for attach")
	application := fs.String("application", "", "explicit application mapping, including before its first deployment")
	selector := fs.String("selector", "", "Pod label selector for an explicit application mapping")
	if err := fs.Parse(args[1:]); err != nil || *name == "" || fs.NArg() != 0 {
		return 2
	}
	if (*application != "" || *selector != "") && (args[0] != "attach" || !ops.ValidName(*application) || *selector == "") {
		fmt.Fprintln(stderr, "explicit mapping requires cluster attach --application <name> --selector <labels>")
		return 2
	}
	if args[0] == "attach" && !ops.ValidName(*environment) {
		fmt.Fprintln(stderr, "attach requires a valid --environment name")
		return 2
	}
	c, err := savedCluster(*name)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if *application != "" {
		result, err := attachConfiguredApplication(*root, ops.Environment{Name: *environment, Connection: c.Name, Namespace: *namespace}, ops.Application{Name: *application, Selector: *selector})
		if err == nil {
			err = json.NewEncoder(stdout).Encode(result)
		}
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := workspace.ReadOpsCluster(ctx, c, *namespace, "")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if args[0] == "attach" {
		if !ops.ValidName(*environment) {
			fmt.Fprintln(stderr, "attach requires a valid --environment name")
			return 2
		}
		if result.Truncated || len(result.Warnings) > 0 {
			fmt.Fprintln(stderr, "cluster discovery incomplete; use the desktop to attach a specific workload")
			return 1
		}
		path, err := projectRoot(*root)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		store, err := ops.New(path)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		snapshot, err := store.Read()
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		for _, w := range result.Workloads {
			if w.Selector == "" || !ops.ValidName(w.Name) {
				fmt.Fprintln(stderr, "workload requires manual application mapping: "+w.Name)
				return 1
			}
			snapshot, err = store.Attach(ops.Environment{Name: *environment, Connection: c.Name, Namespace: *namespace}, ops.Application{Name: w.Name, Selector: w.Selector}, snapshot.Revision)
			if err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
		}
		if err = json.NewEncoder(stdout).Encode(snapshot); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	if err = json.NewEncoder(stdout).Encode(result); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

// Explicit mappings only update local project metadata. They allow a first
// build to be recorded before the workload exists; no cluster write occurs.
func attachConfiguredApplication(root string, env ops.Environment, app ops.Application) (ops.Snapshot, error) {
	store, err := projectStore(root)
	if err != nil {
		return ops.Snapshot{}, err
	}
	snapshot, err := store.Read()
	if err != nil {
		return snapshot, err
	}
	return store.Attach(env, app, snapshot.Revision)
}
