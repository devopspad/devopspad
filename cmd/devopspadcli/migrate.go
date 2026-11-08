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

	"devopspad/internal/ops"
	"devopspad/internal/opsrun"
	"devopspad/internal/workspace"

	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

const maxJobWait = 600 * time.Second

func readJobManifest(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > 1<<20 {
		return nil, errors.New("the Job manifest exceeds 1 MiB")
	}
	return os.ReadFile(path)
}

// planMigration renders the Job with the release image and dry-runs it.
func planMigration(ctx context.Context, cl dynamic.Interface, file, namespace string, saved opsrun.SavedPlan, wait time.Duration) (*opsrun.SavedMigrate, error) {
	b, err := readJobManifest(file)
	if err != nil {
		return nil, err
	}
	job, err := opsrun.RenderJob(b, saved.Image, namespace, saved.ID)
	if err != nil {
		return nil, err
	}
	if err := opsrun.DryRunJob(ctx, cl, job); err != nil {
		return nil, err
	}
	js, err := json.Marshal(job.Object)
	if err != nil {
		return nil, err
	}
	return &opsrun.SavedMigrate{JobFile: file, Name: job.GetName(), Manifest: string(js), WaitTimeoutSeconds: int(wait / time.Second)}, nil
}

func migrationStep(t opsrun.Target, saved opsrun.SavedPlan) func(context.Context, dynamic.Interface, func(string) error) error {
	return func(ctx context.Context, cl dynamic.Interface, step func(string) error) error {
		job := &unstructured.Unstructured{}
		if err := json.Unmarshal([]byte(saved.Migrate.Manifest), &job.Object); err != nil {
			return err
		}
		err := opsrun.RunJob(ctx, cl, job, time.Duration(saved.Migrate.WaitTimeoutSeconds)*time.Second, step)
		if err != nil {
			printJobLogs(ctx, cl, t.Connection, job, os.Stderr)
		}
		return err
	}
}

// printJobLogs shows the last lines of the Job's pods on the executor only.
func printJobLogs(ctx context.Context, cl dynamic.Interface, c workspace.Connection, job *unstructured.Unstructured, w io.Writer) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pods, err := cl.Resource(schema.GroupVersionResource{Version: "v1", Resource: "pods"}).Namespace(job.GetNamespace()).List(ctx, metav1.ListOptions{LabelSelector: "job-name=" + job.GetName(), Limit: 5})
	if err != nil || len(pods.Items) == 0 {
		return
	}
	for _, p := range pods.Items {
		containers, _, _ := unstructured.NestedSlice(p.Object, "spec", "containers")
		if len(containers) == 0 {
			continue
		}
		name, _ := containers[0].(map[string]any)["name"].(string)
		logs, err := workspace.ReadPodLogs(ctx, c, workspace.PodLogRequest{Ref: workspace.ClusterRef{Namespace: p.GetNamespace(), Resource: "pods", Name: p.GetName(), UID: string(p.GetUID())}, Container: name, TailLines: 20})
		if err == nil && strings.TrimSpace(logs.Text) != "" {
			fmt.Fprintf(w, "--- last log lines of %s (not recorded) ---\n%s\n", p.GetName(), strings.TrimRight(logs.Text, "\n"))
		}
	}
}

// cmdJob runs a one-off Job with an image, recorded like a release step.
func cmdJob(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "run" {
		fmt.Fprintln(stderr, "usage: devopspadcli job run --environment <env> --application <app> -f job.yaml --image <ref@sha256:...> [--wait 180s] [--yes]")
		return 2
	}
	fs := flag.NewFlagSet("job run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	environment := fs.String("environment", "", "project environment")
	application := fs.String("application", "", "project application the Job belongs to")
	file := fs.String("f", "", "Job manifest")
	image := fs.String("image", "", "image pinned by digest; replaces "+opsrun.ImagePlaceholder+" or the same repository")
	wait := fs.Duration("wait", 180*time.Second, "how long to wait for completion (max 600s)")
	yes := fs.Bool("yes", false, "create the Job; without it only the dry-run runs")
	root := fs.String("project", "", "local project directory")
	if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 0 {
		return 2
	}
	if *environment == "" || *application == "" || *file == "" || !ops.ValidImage(*image) {
		fmt.Fprintln(stderr, "job run requires --environment, --application, -f and a digest-pinned --image")
		return 2
	}
	if *wait <= 0 || *wait > maxJobWait {
		fmt.Fprintln(stderr, "--wait must be between 1s and 600s")
		return 2
	}
	t, err := cliTarget(*root, *environment, *application)
	if err != nil {
		return jsonFail(stderr, err)
	}
	b, err := readJobManifest(*file)
	if err != nil {
		return jsonFail(stderr, err)
	}
	id := uuid.NewString()
	job, err := opsrun.RenderJob(b, *image, t.Environment.Namespace, id)
	if err != nil {
		return jsonFail(stderr, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), *wait+60*time.Second)
	defer cancel()
	cl, closeFn, err := workspace.OpenCluster(ctx, t.Connection)
	if err != nil {
		return jsonFail(stderr, err)
	}
	defer closeFn()
	if err := opsrun.DryRunJob(ctx, cl, job); err != nil {
		return jsonFail(stderr, err)
	}
	fmt.Fprintf(stderr, "Plan: create Job %s in %s/%s with %s (server dry-run passed)\n", job.GetName(), t.Environment.Name, t.Environment.Namespace, *image)
	if !*yes {
		fmt.Fprintln(stderr, "Plan only; re-run with --yes to create the Job")
		return 0
	}
	r := ops.Record{ID: id, Action: "job", Application: t.Application.Name, Environment: t.Environment.Name, Connection: t.Environment.Connection,
		Namespace: t.Environment.Namespace, Workload: job.GetName(), Image: *image, Status: "running", StartedAt: time.Now().UTC().Format(time.RFC3339Nano), Steps: []string{}}
	if err := t.Store.WriteRecord(r); err != nil {
		return jsonFail(stderr, err)
	}
	runErr := opsrun.RunJob(ctx, cl, job, *wait, func(s string) error {
		r.Steps = append(r.Steps, s)
		fmt.Fprintln(stderr, s)
		return t.Store.WriteRecord(r)
	})
	if runErr != nil {
		printJobLogs(ctx, cl, t.Connection, job, stderr)
	}
	r, err = opsrun.Finish(t.Store, r, runErr, "Job completed")
	return printRecord(r, err, stdout, stderr)
}
