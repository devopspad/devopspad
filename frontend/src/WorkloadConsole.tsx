import { useEffect, useMemo, useState } from "react";
import { LoaderCircle, RefreshCw, Search } from "lucide-react";
import { api, errorText } from "./api";
import type { Connection, Workload } from "./types";
function tone(item: Workload) {
  if (item.desired === item.ready) return "ready";
  if (item.ready === 0 && item.desired > 0) return "down";
  return "partial";
}
function when(value: string) {
  if (!value) return "—";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}
export function WorkloadConsole({
  connection,
  namespace,
  namespaces,
  onNamespace,
  onOpen,
}: {
  connection: Connection;
  namespace: string;
  namespaces: string[];
  onNamespace: (namespace: string) => void;
  onOpen: (item: Workload) => void;
}) {
  const [items, setItems] = useState<Workload[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [query, setQuery] = useState("");
  const [version, setVersion] = useState(0);
  useEffect(() => {
    let stale = false;
    setLoading(true);
    setError("");
    api
      .ListWorkloads(connection.id, namespace || "default")
      .then((rows) => {
        if (!stale) setItems(rows || []);
      })
      .catch((e) => {
        if (!stale) {
          setItems([]);
          setError(errorText(e));
        }
      })
      .finally(() => {
        if (!stale) setLoading(false);
      });
    return () => {
      stale = true;
    };
  }, [connection.id, namespace, version]);
  const visible = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return items;
    return items.filter((item) =>
      `${item.kind} ${item.name} ${item.image} ${item.namespace}`
        .toLowerCase()
        .includes(needle),
    );
  }, [items, query]);
  return (
    <section className="workload-console" aria-label="Workloads">
      <div className="workload-toolbar">
        <div>
          <h2>Workloads</h2>
          <p>Deployments, StatefulSets, and DaemonSets in this namespace.</p>
        </div>
        <div className="workload-controls">
          <label>
            Namespace
            <select
              aria-label="Workload namespace"
              value={namespace || "default"}
              onChange={(e) => onNamespace(e.target.value)}
            >
              {(namespaces.length ? namespaces : [namespace || "default"]).map(
                (name) => (
                  <option key={name} value={name}>
                    {name === "*" ? "All namespaces" : name}
                  </option>
                ),
              )}
            </select>
          </label>
          <label className="workload-search">
            <Search size={14} />
            <input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Filter name or image"
              aria-label="Filter workloads"
            />
          </label>
          <button
            className="button"
            onClick={() => setVersion((v) => v + 1)}
            disabled={loading}
          >
            {loading ? (
              <LoaderCircle className="spin" size={14} />
            ) : (
              <RefreshCw size={14} />
            )}
            Refresh
          </button>
        </div>
      </div>
      {error && (
        <p className="tree-error" role="alert">
          {error}
        </p>
      )}
      <div className="workload-table-wrap">
        <table>
          <thead>
            <tr>
              <th>Kind</th>
              <th>Name</th>
              <th>Ready</th>
              <th>Image</th>
              <th>Updated</th>
            </tr>
          </thead>
          <tbody>
            {visible.map((item) => (
              <tr key={`${item.resource}/${item.namespace}/${item.name}`}>
                <td>
                  <span className={`workload-kind ${item.kind.toLowerCase()}`}>
                    {item.kind}
                  </span>
                </td>
                <td>
                  <button
                    className="workload-name"
                    onClick={() => onOpen(item)}
                  >
                    {item.name}
                  </button>
                  {namespace === "*" && <small>{item.namespace}</small>}
                </td>
                <td>
                  <span className={`workload-ready ${tone(item)}`}>
                    {item.ready} / {item.desired}
                  </span>
                </td>
                <td className="workload-image" title={item.image}>
                  {item.image || "—"}
                  {item.extraImages > 0 && <small> +{item.extraImages}</small>}
                </td>
                <td>{when(item.updatedAt)}</td>
              </tr>
            ))}
            {!loading && visible.length === 0 && (
              <tr>
                <td colSpan={5} className="workload-empty">
                  No workloads in this namespace.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
      <p className="workload-count">
        {visible.length} workload{visible.length === 1 ? "" : "s"}
        {""}
      </p>
    </section>
  );
}
