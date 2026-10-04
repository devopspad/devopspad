import { useEffect, useState } from "react";
import {
  Check,
  Copy,
  Database,
  FileCode2,
  ListTree,
  LoaderCircle,
  RefreshCw,
  Table2,
} from "lucide-react";
import { api, display, errorText } from "./api";
import { demoInspect } from "./demo";
import { Modal } from "./Modal";
import type {
  Connection,
  QueryResult,
  Resource,
  ResourceDetails,
} from "./types";

function DetailGrid({ result }: { result: QueryResult }) {
  if (!result.columns.length || !result.rows.length)
    return <div className="detail-empty">No entries returned.</div>;
  return (
    <div className="detail-table">
      <table>
        <thead>
          <tr>
            {result.columns.map((c, i) => (
              <th key={i}>{c}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {result.rows.map((row, i) => (
            <tr key={i}>
              {row.map((cell, j) => (
                <td key={j} title={display(cell)}>
                  {cell == null ? (
                    <span className="null-value">NULL</span>
                  ) : (
                    display(cell)
                  )}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
export function ResourceInspector({
  connection,
  database,
  resource,
  onClose,
  onQuery,
}: {
  connection: Connection;
  database: string;
  resource: Resource;
  onClose: () => void;
  onQuery: () => void;
}) {
  const demo = connection.id.startsWith("demo-");
  const redis = connection.kind === "redis";
  const [details, setDetails] = useState<ResourceDetails | null>(null);
  const [error, setError] = useState("");
  const [tab, setTab] = useState(redis ? "value" : "columns");
  const [attempt, setAttempt] = useState(0);
  const [copied, setCopied] = useState(false);
  useEffect(() => {
    let stale = false;
    setDetails(null);
    setError("");
    (demo
      ? demoInspect(connection.kind, database, resource.name)
      : api.InspectResource(connection.id, database, resource.name)
    )
      .then((value) => {
        if (!stale) setDetails(value);
      })
      .catch((e) => {
        if (!stale) setError(errorText(e));
      });
    return () => {
      stale = true;
    };
  }, [connection.id, connection.kind, database, resource.name, attempt, demo]);
  return (
    <Modal
      title={resource.name}
      subtitle={`${redis ? "Key details" : "Table structure"} · ${database || "default"}${demo ? " · Sample data" : ""}`}
      wide
      onClose={onClose}
    >
      <div className="object-inspector">
        {error ? (
          <div className="error-box" role="alert">
            {error}
            <button
              className="text-button"
              onClick={() => setAttempt((x) => x + 1)}
            >
              Try again
            </button>
          </div>
        ) : !details ? (
          <div className="detail-loading">
            <LoaderCircle size={21} className="spin" />
            Loading resource details…
          </div>
        ) : (
          <>
            <div className="detail-properties">
              {details.properties.map((p) => (
                <div key={p.name}>
                  <span>{p.name}</span>
                  <strong>{p.value}</strong>
                </div>
              ))}
            </div>
            <div
              className="detail-tabs"
              role="tablist"
              aria-label="Resource detail sections"
            >
              {(redis
                ? [["value", "Value preview", Database]]
                : [
                    ["columns", "Columns", Table2],
                    ["indexes", "Indexes", ListTree],
                    ["ddl", "DDL", FileCode2],
                  ]
              ).map(([id, title, Icon]) => {
                const Glyph = Icon as typeof Database;
                return (
                  <button
                    key={String(id)}
                    role="tab"
                    aria-selected={tab === id}
                    onClick={() => setTab(String(id))}
                  >
                    <Glyph size={14} />
                    {String(title)}
                  </button>
                );
              })}
            </div>
            {tab === "ddl" ? (
              <div className="definition-panel">
                {demo && (
                  <p className="field-help">
                    This definition comes from the local SQLite demo. Live
                    connections show the server’s native DDL.
                  </p>
                )}
                <pre>{details.definition || "No definition returned."}</pre>
                <button
                  className="button"
                  onClick={async () => {
                    try {
                      await navigator.clipboard.writeText(details.definition);
                      setCopied(true);
                    } catch (e) {
                      setError(errorText(e));
                    }
                  }}
                >
                  {copied ? <Check size={14} /> : <Copy size={14} />}{" "}
                  {copied ? "Copied" : "Copy DDL"}
                </button>
              </div>
            ) : (
              <DetailGrid
                result={
                  tab === "columns"
                    ? details.columns
                    : tab === "indexes"
                      ? details.indexes
                      : details.preview
                }
              />
            )}
            {redis &&
              (details.preview.message || details.preview.truncated) && (
                <p className="preview-note">
                  {details.preview.message}{" "}
                  {details.preview.truncated && "Preview limit reached."}
                </p>
              )}
          </>
        )}
      </div>
      <div className="modal-footer">
        <button className="button" onClick={() => setAttempt((x) => x + 1)}>
          <RefreshCw size={14} />
          Refresh
        </button>
        <button className="button primary" onClick={onQuery}>
          <FileCode2 size={14} />
          Open query
        </button>
      </div>
    </Modal>
  );
}
