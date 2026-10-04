import type {
  ClusterDetails,
  ClusterRef,
  PodLogRequest,
  PodLogs,
  QueryResult,
} from "./types";

export async function demoClusterDetails(
  ref: ClusterRef,
): Promise<ClusterDetails> {
  const pod = ref.resource === "pods";
  const containers = pod
    ? [
        {
          name: "app",
          kind: "Application",
          image: "example/api:1.8.0",
          state: "running",
          ready: true,
          restarts: 1,
        },
        {
          name: "setup",
          kind: "Init",
          image: "example/setup:1.0",
          state: "terminated · Completed",
          ready: false,
          restarts: 0,
        },
      ]
    : [];
  const resolved = { ...ref, uid: `sample-${ref.name}` };
  return {
    ref: resolved,
    properties: [
      { name: "Name", value: ref.name },
      { name: "Namespace", value: ref.namespace || "Cluster-wide" },
      { name: "Status", value: pod ? "Running (sample)" : "Sample resource" },
    ],
    labels: {
      "app.kubernetes.io/name": "infradex-demo",
      environment: "sample",
    },
    containers,
    definition: JSON.stringify(
      {
        apiVersion: "v1",
        kind: (
          {
            pods: "Pod",
            deployments: "Deployment",
            services: "Service",
            nodes: "Node",
            namespaces: "Namespace",
            events: "Event",
            configmaps: "ConfigMap",
          } as Record<string, string>
        )[ref.resource],
        metadata: {
          name: ref.name,
          namespace: ref.namespace,
          uid: resolved.uid,
        },
        ...(pod
          ? {
              spec: {
                containers: containers
                  .filter((c) => c.kind === "Application")
                  .map(({ name, image }) => ({ name, image })),
              },
            }
          : {}),
      },
      null,
      2,
    ),
  };
}
export async function demoClusterEvents(): Promise<QueryResult> {
  return {
    columns: ["last_seen", "type", "reason", "message", "count"],
    rows: [
      [
        "2026-10-01T08:30:00Z",
        "Normal",
        "Started",
        "Started container app (sample)",
        1,
      ],
    ],
    duration: 0,
    truncated: false,
    message: "Sample event · no cluster request sent",
  };
}
export async function demoPodLogs(request: PodLogRequest): Promise<PodLogs> {
  return {
    text: request.previous
      ? "2026-10-01T08:20:00Z [sample] Previous container received shutdown signal\n"
      : `2026-10-01T08:30:00Z [sample] ${request.container}: starting service\n2026-10-01T08:30:01Z [sample] Health check passed\n2026-10-01T08:30:02Z [sample] Ready to accept requests\n`,
    truncated: false,
  };
}
