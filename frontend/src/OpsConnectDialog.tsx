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
      title="接入 K3s 应用"
      subtitle="读取集群负载，建立本地环境与应用映射"
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
            集群连接
            <select
              aria-label="接入集群"
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
              {!clusters.length && <option value="">还没有集群连接</option>}
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
            新建 K3s 连接
          </button>
          {connection && (
            <p className="ops-muted">
              Context：{connection.context || "当前上下文"} ·{" "}
              {connection.ssh?.enabled
                ? "通过已保存的 SSH 隧道"
                : "直接连接 Kubernetes API"}
            </p>
          )}
          <label>
            命名空间
            <input
              aria-label="接入命名空间"
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
            读取负载
          </button>
          {cluster && (
            <>
              {cluster.warnings.map((w) => (
                <div className="ops-error" key={w}>
                  {w}
                </div>
              ))}
              {cluster.truncated && (
                <p className="ops-notice">列表已截断，仅展示前 500 条资源。</p>
              )}
              <p role="status">
                已连接：{cluster.workloads.length} 个负载 ·{" "}
                {cluster.pods.length} 个 Pod
              </p>
              <label>
                工作负载
                <select
                  aria-label="接入负载"
                  value={workload?.uid || ""}
                  disabled={!!busy}
                  onChange={(e) =>
                    select(
                      cluster.workloads.find((w) => w.uid === e.target.value),
                    )
                  }
                >
                  {!cluster.workloads.length && (
                    <option value="">此命名空间暂无可接入负载</option>
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
                    环境名称
                    <input
                      required
                      maxLength={63}
                      pattern="[a-z0-9][a-z0-9-]*"
                      aria-label="接入环境名称"
                      value={environment}
                      disabled={!!busy}
                      onChange={(e) => setEnvironment(e.target.value)}
                    />
                  </label>
                  <label>
                    应用名称
                    <input
                      required
                      maxLength={63}
                      pattern="[a-z0-9][a-z0-9-]*"
                      aria-label="接入应用名称"
                      value={application}
                      disabled={!!busy}
                      onChange={(e) => setApplication(e.target.value)}
                    />
                  </label>
                  <div className="ops-release-images">
                    <label>
                      应用选择器
                      <code>{workload.selector || "没有可用选择器"}</code>
                    </label>
                  </div>
                </>
              )}
            </>
          )}
          <p className="ops-muted">
            接入只保存本地
            project.yaml，不会更改集群负载。凭据继续保存在本机，AI
            可使用应用名称上报构建和发布记录。
          </p>
        </div>
        <div className="ops-dialog-footer">
          <button
            type="button"
            className="ops-btn"
            disabled={!!busy}
            onClick={onClose}
          >
            取消
          </button>
          <button
            className="ops-btn primary"
            disabled={!!busy || !workload?.selector}
          >
            {busy === "save" && <LoaderCircle size={15} className="spin" />}
            接入应用
          </button>
        </div>
      </form>
    </Modal>
  );
}
