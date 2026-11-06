import { useEffect, useState } from "react";
import { LoaderCircle, RefreshCw, Search, Server } from "lucide-react";
import { api, errorText } from "./api";
import { WorkloadConsole } from "./WorkloadConsole";
import type { ClusterRef, Connection, QueryResult, Workload } from "./types";
type NodeObject = {
  metadata?: {
    name?: string;
    uid?: string;
    labels?: Record<string, string>;
  };
  spec?: {
    unschedulable?: boolean;
  };
  status?: {
    conditions?: {
      type: string;
      status: string;
    }[];
    addresses?: {
      type: string;
      address: string;
    }[];
    capacity?: {
      cpu?: string;
      memory?: string;
    };
    nodeInfo?: {
      kubeletVersion?: string;
      osImage?: string;
      architecture?: string;
    };
  };
};
function memory(value?: string) {
  if (!value) return "—";
  const match = /^(\d+(?:\.\d+)?)(Ki|Mi|Gi|Ti)?$/.exec(value);
  if (!match) return value;
  const powers: Record<string, number> = { Ki: 1, Mi: 2, Gi: 3, Ti: 4 };
  return `${((Number(match[1]) * 1024 ** (powers[match[2]] || 0)) / 1024 ** 3).toLocaleString(undefined, { maximumFractionDigits: 1 })} GiB`;
}
function nodesFrom(result: QueryResult): NodeObject[] {
  const index = result.columns.indexOf("resource");
  if (index < 0) throw new Error("节点响应缺少资源定义，请刷新重试。");
  return result.rows.map((row) => {
    const node = row[index] as NodeObject | null;
    if (!node || typeof node !== "object" || !node.metadata?.name)
      throw new Error("节点响应格式不完整，请刷新重试。");
    return node;
  });
}
export function InfraCluster({
  connection,
  namespace,
  namespaces,
  onNamespace,
  onOpen,
  onNode,
  nodesOnly = false,
}: {
  nodesOnly?: boolean;
  connection: Connection;
  namespace: string;
  namespaces: string[];
  onNamespace: (value: string) => void;
  onOpen: (item: Workload) => void;
  onNode: (ref: ClusterRef) => void;
}) {
  const [tab, setTab] = useState<"nodes" | "workloads">("nodes");
  const [nodes, setNodes] = useState<NodeObject[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [truncated, setTruncated] = useState(false);
  const [version, setVersion] = useState(0);
  const [search, setSearch] = useState("");
  useEffect(() => {
    if (tab !== "nodes") return;
    let stale = false;
    let pending = true;
    const requestID = crypto.randomUUID();
    setLoading(true);
    setError("");
    setNodes([]);
    setTruncated(false);
    api
      .ExecuteInContext(
        connection.id,
        "",
        connection.context,
        "get nodes",
        requestID,
      )
      .then((result) => ({
        nodes: nodesFrom(result),
        truncated: result.truncated,
      }))
      .then((result) => {
        if (!stale) {
          setNodes(result.nodes);
          setTruncated(result.truncated);
        }
      })
      .catch((e) => {
        if (!stale) setError(errorText(e));
      })
      .finally(() => {
        pending = false;
        if (!stale) setLoading(false);
      });
    return () => {
      stale = true;
      if (pending) void api.CancelQuery(requestID).catch(() => {});
    };
  }, [connection, tab, version]);
  const filtered = nodes.filter((node) =>
    `${node.metadata?.name} ${(node.status?.addresses || []).map((a) => a.address).join(" ")}`
      .toLowerCase()
      .includes(search.toLowerCase()),
  );
  return (
    <section className="infra-cluster">
      <header className="infra-cluster-heading">
        <div>
          <h1>{connection.name}</h1>
          <p>
            {connection.environment || "未分组"} ·{" "}
            {connection.kind === "k3s" ? "K3s" : "Kubernetes"} ·{" "}
            {connection.context || "默认 context"}
            {""}
          </p>
        </div>
      </header>
      {!nodesOnly && (
        <>
          <div className="infra-tabs" role="tablist" aria-label="集群视图">
            <button
              role="tab"
              aria-selected={tab === "nodes"}
              onClick={() => setTab("nodes")}
            >
              节点 / 机器
            </button>
            <button
              role="tab"
              aria-selected={tab === "workloads"}
              onClick={() => setTab("workloads")}
            >
              工作负载
            </button>
          </div>
        </>
      )}
      {tab === "workloads" ? (
        <WorkloadConsole
          connection={connection}
          namespace={namespace}
          namespaces={namespaces}
          onNamespace={onNamespace}
          onOpen={onOpen}
        />
      ) : (
        <>
          <div className="infra-node-toolbar">
            <span>
              <Server size={16} />
              {loading
                ? "读取节点中…"
                : `${nodes.length} 台机器${truncated ? "（列表不完整）" : ""}`}
            </span>
            <label>
              <Search size={14} />
              <input
                aria-label="搜索节点"
                placeholder="搜索节点名称或 IP"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
              />
            </label>
            <button
              className="button"
              aria-label="刷新节点"
              disabled={loading}
              onClick={() => setVersion((v) => v + 1)}
            >
              <RefreshCw size={15} />
              刷新
            </button>
          </div>
          {loading ? (
            <div className="infra-message" role="status">
              <LoaderCircle size={18} className="spin" />
              正在读取集群的所有节点…
            </div>
          ) : error ? (
            <div className="error-box" role="alert">
              {error}
            </div>
          ) : (
            <>
              {truncated && (
                <p className="infra-message" role="status">
                  返回结果达到读取上限，以下不是集群的全部节点。
                </p>
              )}
              <div className="infra-node-table">
                <table>
                  <thead>
                    <tr>
                      <th>节点</th>
                      <th>状态</th>
                      <th>角色</th>
                      <th>IP 地址</th>
                      <th>CPU / 内存</th>
                      <th>系统 / Kubernetes</th>
                    </tr>
                  </thead>
                  <tbody>
                    {filtered.map((node) => {
                      const { metadata = {}, status = {}, spec = {} } = node;
                      const ready = status.conditions?.find(
                        (c) => c.type === "Ready",
                      )?.status;
                      const roles = Object.keys(metadata.labels || {})
                        .filter((key) =>
                          key.startsWith("node-role.kubernetes.io/"),
                        )
                        .map((key) =>
                          key.slice("node-role.kubernetes.io/".length),
                        );
                      return (
                        <tr key={metadata.uid || metadata.name}>
                          <td>
                            <button
                              className="text-button"
                              onClick={() =>
                                onNode({
                                  name: metadata.name!,
                                  uid: metadata.uid || "",
                                  namespace: "",
                                  context: connection.context,
                                  resource: "nodes",
                                })
                              }
                            >
                              {metadata.name}
                            </button>
                          </td>
                          <td>
                            <span
                              className={`infra-node-status ${ready === "True" ? "ready" : "unknown"}`}
                            >
                              {ready === "True"
                                ? "Ready"
                                : ready === "False"
                                  ? "NotReady"
                                  : "Unknown"}
                            </span>
                            {spec.unschedulable && <small>已禁止调度</small>}
                          </td>
                          <td>
                            {roles.join(", ") ||
                              metadata.labels?.["kubernetes.io/role"] ||
                              "worker"}
                          </td>
                          <td>
                            {(status.addresses || [])
                              .filter((address) =>
                                ["InternalIP", "ExternalIP"].includes(
                                  address.type,
                                ),
                              )
                              .map((address) => (
                                <div key={`${address.type}:${address.address}`}>
                                  {address.address}
                                  <small>{address.type}</small>
                                </div>
                              ))}
                          </td>
                          <td>
                            {status.capacity?.cpu || "—"} CPU
                            <small>{memory(status.capacity?.memory)}</small>
                          </td>
                          <td>
                            {status.nodeInfo?.osImage || "—"}
                            <small>
                              {status.nodeInfo?.kubeletVersion || "—"} ·{" "}
                              {status.nodeInfo?.architecture || "—"}
                            </small>
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
              {!filtered.length && (
                <p className="infra-message">
                  {nodes.length ? "没有匹配的节点。" : "集群未返回节点。"}
                </p>
              )}
            </>
          )}
        </>
      )}
    </section>
  );
}
