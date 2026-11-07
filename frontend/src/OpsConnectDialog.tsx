import { useState } from "react";
import { LoaderCircle, Plus, PlugZap } from "lucide-react";
import { api, errorText } from "./api";
import { Modal } from "./Modal";
import { ConnectionDialog } from "./ConnectionDialog";
import type { Connection, Workload } from "./types";
import type { OpsCluster, OpsSnapshot } from "./ops-api";

export function OpsConnectDialog({
  snapshot,
  connections,
  onConnections,
  onClose,
  onDone,
}: {
  snapshot: OpsSnapshot;
  connections: Connection[];
  onConnections: (cs: Connection[]) => void;
  onClose: () => void;
  onDone: (
    snapshot: OpsSnapshot,
    environment: string,
    application: string,
  ) => void;
}) {
  const clusters = connections.filter(
    (c) => c.kind === "k3s" || c.kind === "k8s",
  );
  const [connectionId, setConnectionId] = useState(clusters[0]?.id || "");
  const connection = clusters.find((c) => c.id === connectionId);
  const [namespace, setNamespace] = useState(
    connection?.database && connection.database !== "*"
      ? connection.database
      : "default",
  );
  const [environment, setEnvironment] = useState(
    connection?.name
      .toLowerCase()
      .replace(/[^a-z0-9-]/g, "-")
      .slice(0, 63) || "local",
  );
  const [application, setApplication] = useState("");
  const [cluster, setCluster] = useState<OpsCluster>();
  const [workload, setWorkload] = useState<Workload>();
  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");
  const [add, setAdd] = useState(false);
  const select = (w?: Workload) => {
    setWorkload(w);
    setApplication(w?.name || "");
  };
  async function discover() {
    if (!connection) return;
    setBusy("read");
    setError("");
    setCluster(undefined);
    select(undefined);
    try {
      const data = await api.ReadOpsCluster(connection.id, namespace, "");
      setCluster(data);
      select(data.workloads[0]);
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy("");
    }
  }
  async function attach() {
    if (!connection || !workload) return;
    setBusy("save");
    setError("");
    try {
      const next = await api.AttachOpsWorkload({
        connectionId: connection.id,
        namespace,
        workload: workload.name,
        uid: workload.uid,
        resource: workload.resource,
        environment,
        application,
        revision: snapshot.revision,
      });
      onDone(next, environment, application);
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy("");
    }
  }
  if (add)
    return (
      <ConnectionDialog
        defaultKind="k3s"
        onClose={() => setAdd(false)}
        onSaved={(c) => {
          onConnections([...connections, c]);
          setConnectionId(c.id);
          setNamespace(
            c.database && c.database !== "*" ? c.database : "default",
          );
          setEnvironment(
            c.name
              .toLowerCase()
              .replace(/[^a-z0-9-]/g, "-")
              .slice(0, 63),
          );
          setCluster(undefined);
          select(undefined);
          setAdd(false);
        }}
      />
    );
  return (
    <Modal
      title="Connect a K3s app"
      subtitle="Read cluster workloads and map them to a local environment and application"
      onClose={() => {
        if (!busy) onClose();
      }}
    >
      <form
        onSubmit={(e) => {
          e.preventDefault();
          void attach();
        }}
      >
        <div className="ops-dialog-body ops-form">
          {error && (
            <div role="alert" className="ops-error">
              {error}
            </div>
          )}
          <label>
            Cluster connection
            <select
              aria-label="Cluster to connect"
              disabled={!!busy}
              value={connectionId}
              onChange={(e) => {
                setConnectionId(e.target.value);
                setCluster(undefined);
                select(undefined);
                const c = clusters.find((c) => c.id === e.target.value);
                setNamespace(
                  c?.database && c.database !== "*" ? c.database : "default",
                );
                setEnvironment(
                  c?.name
                    .toLowerCase()
                    .replace(/[^a-z0-9-]/g, "-")
                    .slice(0, 63) || "local",
                );
              }}
            >
              {!clusters.length && <option value="">No cluster connections yet</option>}
              {clusters.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name} · {c.kind.toUpperCase()}
                </option>
              ))}
            </select>
          </label>
          <button
            type="button"
            className="ops-btn"
            disabled={!!busy}
            onClick={() => setAdd(true)}
          >
            <Plus size={15} />
            New K3s connection
          </button>
          {connection && (
            <p className="ops-muted">
              Context: {connection.context || "current context"} ·{" "}
              {connection.ssh?.enabled
                ? "through the saved SSH tunnel"
                : "direct to the Kubernetes API"}
            </p>
          )}
          <label>
            Namespace
            <input
              aria-label="Namespace to connect"
              required
              value={namespace}
              disabled={!!busy}
              onChange={(e) => {
                setNamespace(e.target.value);
                setCluster(undefined);
                select(undefined);
              }}
            />
          </label>
          <button
            type="button"
            className="ops-btn"
            disabled={!!busy || !connection || !namespace || namespace === "*"}
            onClick={() => void discover()}
          >
            {busy === "read" ? (
              <LoaderCircle size={15} className="spin" />
            ) : (
              <PlugZap size={15} />
            )}
            Read workloads
          </button>
          {cluster && (
            <>
              {cluster.warnings.map((w) => (
                <div className="ops-error" key={w}>
                  {w}
                </div>
              ))}
              {cluster.truncated && (
                <p className="ops-notice">List truncated: showing the first 500 resources only.</p>
              )}
              <p role="status">
                Connected: {cluster.workloads.length} {cluster.workloads.length === 1 ? "workload" : "workloads"} ·{" "}
                {cluster.pods.length} {cluster.pods.length === 1 ? "Pod" : "Pods"}
              </p>
              <label>
                Workload
                <select
                  aria-label="Workload to connect"
                  value={workload?.uid || ""}
                  disabled={!!busy}
                  onChange={(e) =>
                    select(
                      cluster.workloads.find((w) => w.uid === e.target.value),
                    )
                  }
                >
                  {!cluster.workloads.length && (
                    <option value="">No workloads to connect in this namespace</option>
                  )}
                  {cluster.workloads.map((w) => (
                    <option key={w.uid} value={w.uid}>
                      {w.kind} · {w.name}
                    </option>
                  ))}
                </select>
              </label>
              {workload && (
                <>
                  <label>
                    Environment name
                    <input
                      required
                      maxLength={63}
                      pattern="[a-z0-9][a-z0-9-]*"
                      aria-label="Environment name"
                      value={environment}
                      disabled={!!busy}
                      onChange={(e) => setEnvironment(e.target.value)}
                    />
                  </label>
                  <label>
                    Application name
                    <input
                      required
                      maxLength={63}
                      pattern="[a-z0-9][a-z0-9-]*"
                      aria-label="Application name"
                      value={application}
                      disabled={!!busy}
                      onChange={(e) => setApplication(e.target.value)}
                    />
                  </label>
                  <div className="ops-release-images">
                    <label>
                      Application selector
                      <code>{workload.selector || "No selector available"}</code>
                    </label>
                  </div>
                </>
              )}
            </>
          )}
          <p className="ops-muted">
            Connecting only writes the local
            project.yaml and never changes cluster workloads. Credentials stay on this device; AI
            agents can report builds and releases by application name.
          </p>
        </div>
        <div className="ops-dialog-footer">
          <button
            type="button"
            className="ops-btn"
            disabled={!!busy}
            onClick={onClose}
          >
            Cancel
          </button>
          <button
            className="ops-btn primary"
            disabled={!!busy || !workload?.selector}
          >
            {busy === "save" && <LoaderCircle size={15} className="spin" />}
            Connect app
          </button>
        </div>
      </form>
    </Modal>
  );
}
