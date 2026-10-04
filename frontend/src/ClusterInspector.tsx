import { useEffect, useState } from "react";
import { Copy, LoaderCircle, RefreshCw, Terminal } from "lucide-react";
import { Modal } from "./Modal";
import { api, display, errorText } from "./api";
import {
  demoClusterDetails,
  demoClusterEvents,
  demoPodLogs,
} from "./cluster-demo";
import type {
  ClusterDetails,
  ClusterRef,
  Connection,
  PodLogs,
  QueryResult,
} from "./types";

export function ClusterInspector({
  connection,
  resource,
  onClose,
}: {
  connection: Connection;
  resource: ClusterRef;
  onClose: () => void;
}) {
  const demo = connection.id.startsWith("demo-");
  const [tab, setTab] = useState("overview");
  const [details, setDetails] = useState<ClusterDetails>();
  const [detailError, setDetailError] = useState("");
  const [events, setEvents] = useState<QueryResult>();
  const [eventError, setEventError] = useState("");
  const [logs, setLogs] = useState<PodLogs>();
  const [logError, setLogError] = useState("");
  const [container, setContainer] = useState("");
  const [tailLines, setTailLines] = useState(200);
  const [previous, setPrevious] = useState(false);
  const [version, setVersion] = useState(0);
  const [sectionVersion, setSectionVersion] = useState(0);
  const [copyStatus, setCopyStatus] = useState("");
  useEffect(() => {
    let stale = false;
    setDetails(undefined);
    setDetailError("");
    setEvents(undefined);
    setLogs(undefined);
    (demo
      ? demoClusterDetails(resource)
      : api.InspectClusterResource(connection.id, resource)
    )
      .then((d) => {
        if (stale) return;
        setDetails(d);
        setContainer((old) =>
          d.containers.some((c) => c.name === old)
            ? old
            : d.containers[0]?.name || "",
        );
      })
      .catch((e) => {
        if (!stale) setDetailError(errorText(e));
      });
    return () => {
      stale = true;
    };
  }, [connection.id, resource, demo, version]);
  useEffect(() => {
    if (tab !== "events" || !details) return;
    let stale = false;
    setEvents(undefined);
    setEventError("");
    (demo ? demoClusterEvents() : api.ClusterEvents(connection.id, details.ref))
      .then((r) => {
        if (!stale) setEvents(r);
      })
      .catch((e) => {
        if (!stale) setEventError(errorText(e));
      });
    return () => {
      stale = true;
    };
  }, [tab, details, connection.id, demo, sectionVersion]);
  useEffect(() => {
    if (tab !== "logs" || !details || !container) return;
    let stale = false;
    setLogs(undefined);
    setLogError("");
    const request = { ref: details.ref, container, tailLines, previous };
    (demo ? demoPodLogs(request) : api.ReadPodLogs(connection.id, request))
      .then((r) => {
        if (!stale) setLogs(r);
      })
      .catch((e) => {
        if (!stale) setLogError(errorText(e));
      });
    return () => {
      stale = true;
    };
  }, [
    tab,
    details,
    container,
    tailLines,
    previous,
    connection.id,
    demo,
    sectionVersion,
  ]);
  async function copy(text: string) {
    try {
      await navigator.clipboard.writeText(text);
      setCopyStatus("Copied to clipboard");
    } catch (e) {
      setCopyStatus(errorText(e));
    }
  }
  const loading = (
    <div className="detail-loading" role="status">
      <LoaderCircle size={18} className="spin" />
      Loading…
    </div>
  );
  return (
    <Modal
      title={resource.name}
      subtitle={`${resource.resource} · ${resource.context || "Current context"} / ${resource.namespace || "Cluster-wide"}${demo ? " · Sample data" : ""}`}
      wide
      onClose={onClose}
    >
      <div className="object-inspector cluster-inspector">
        {detailError ? (
          <div className="error-box" role="alert">
            {detailError}
          </div>
        ) : !details ? (
          loading
        ) : (
          <>
            <div
              className="detail-tabs"
              role="tablist"
              aria-label="Cluster resource sections"
            >
              {[
                "overview",
                "events",
                ...(resource.resource === "pods" ? ["logs"] : []),
                "json",
              ].map((id) => (
                <button
                  key={id}
                  role="tab"
                  aria-selected={tab === id}
                  onClick={() => {
                    setTab(id);
                    setCopyStatus("");
                  }}
                >
                  {id === "json" ? "JSON" : id[0].toUpperCase() + id.slice(1)}
                </button>
              ))}
            </div>
            {tab === "overview" && (
              <section aria-label="Resource overview">
                <div className="detail-properties">
                  {details.properties.map((p) => (
                    <div key={p.name}>
                      <span>{p.name}</span>
                      <strong>{p.value || "—"}</strong>
                    </div>
                  ))}
                </div>
                <h3>Labels</h3>
                <div className="cluster-labels">
                  {Object.entries(details.labels).length ? (
                    Object.entries(details.labels).map(([key, value]) => (
                      <code key={key}>
                        {key}={value}
                      </code>
                    ))
                  ) : (
                    <span className="field-help">No labels.</span>
                  )}
                </div>
                {!!details.containers.length && (
                  <>
                    <h3>Containers</h3>
                    <div className="detail-table">
                      <table>
                        <thead>
                          <tr>
                            <th>Name</th>
                            <th>Type</th>
                            <th>State</th>
                            <th>Ready</th>
                            <th>Restarts</th>
                          </tr>
                        </thead>
                        <tbody>
                          {details.containers.map((c) => (
                            <tr key={c.name}>
                              <td>
                                <button
                                  className="text-button"
                                  title={c.image}
                                  onClick={() => {
                                    setContainer(c.name);
                                    setTab("logs");
                                  }}
                                >
                                  <Terminal size={13} />
                                  {c.name}
                                </button>
                              </td>
                              <td>{c.kind}</td>
                              <td>{c.state}</td>
                              <td>{c.ready ? "Yes" : "No"}</td>
                              <td>{c.restarts}</td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  </>
                )}
              </section>
            )}
            {tab === "json" && (
              <div className="definition-panel">
                <p className="field-help">
                  Read-only resource definition. Managed fields are omitted.
                </p>
                <pre aria-label="Resource JSON">{details.definition}</pre>
                <button
                  className="button"
                  onClick={() => void copy(details.definition)}
                >
                  <Copy size={14} />
                  Copy JSON
                </button>
              </div>
            )}
            {tab === "events" && (
              <section aria-label="Resource events">
                {eventError ? (
                  <div className="error-box" role="alert">
                    {eventError}
                  </div>
                ) : !events ? (
                  loading
                ) : (
                  <>
                    {events.rows.length ? (
                      <div className="detail-table cluster-events">
                        <table>
                          <thead>
                            <tr>
                              {events.columns.map((c) => (
                                <th key={c}>{c.replaceAll("_", " ")}</th>
                              ))}
                            </tr>
                          </thead>
                          <tbody>
                            {events.rows.map((row, i) => (
                              <tr key={i}>
                                {row.map((v, j) => (
                                  <td key={j}>{display(v)}</td>
                                ))}
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      </div>
                    ) : (
                      <div className="detail-empty">
                        No retained events for this resource.
                      </div>
                    )}
                    <p className="preview-note">
                      {events.message}
                      {events.truncated &&
                        " Event limit reached; this list is incomplete."}
                    </p>
                  </>
                )}
              </section>
            )}
            {tab === "logs" && (
              <section aria-label="Pod logs">
                <div className="log-controls">
                  <label>
                    Container
                    <select
                      aria-label="Log container"
                      value={container}
                      onChange={(e) => setContainer(e.target.value)}
                    >
                      {details.containers.map((c) => (
                        <option key={c.name} value={c.name}>
                          {c.name} · {c.kind}
                        </option>
                      ))}
                    </select>
                  </label>
                  <label>
                    Tail lines
                    <select
                      value={tailLines}
                      onChange={(e) => setTailLines(Number(e.target.value))}
                    >
                      {[100, 200, 500, 1000, 2000].map((n) => (
                        <option key={n}>{n}</option>
                      ))}
                    </select>
                  </label>
                  <label className="log-previous">
                    <input
                      type="checkbox"
                      checked={previous}
                      onChange={(e) => setPrevious(e.target.checked)}
                    />
                    Previous instance
                  </label>
                </div>
                <p className="field-help">
                  {demo
                    ? "Sample logs · no cluster request sent"
                    : "Log snapshot · timestamps included · up to 256 KiB"}
                </p>
                {!container ? (
                  <div className="detail-empty">No containers available.</div>
                ) : logError ? (
                  <div className="error-box" role="alert">
                    {logError}
                  </div>
                ) : !logs ? (
                  loading
                ) : (
                  <>
                    <pre className="pod-logs" aria-label="Log output">
                      {logs.text ||
                        "No log output returned for this container."}
                    </pre>
                    {logs.truncated && (
                      <p className="preview-note">
                        Log size limit reached. Reduce the number of tail lines
                        to view a smaller snapshot.
                      </p>
                    )}
                    <button
                      className="button"
                      disabled={!logs.text}
                      onClick={() => void copy(logs.text)}
                    >
                      <Copy size={14} />
                      Copy logs
                    </button>
                  </>
                )}
              </section>
            )}
          </>
        )}
        {copyStatus && (
          <p className="field-help" role="status">
            {copyStatus}
          </p>
        )}
      </div>
      <div className="modal-footer">
        <span className="field-help">
          Read-only ·{" "}
          {demo ? "Sample resource" : "Permissions enforced by your cluster"}
        </span>
        <button
          className="button"
          onClick={() => {
            setCopyStatus("");
            if (details && (tab === "logs" || tab === "events"))
              setSectionVersion((v) => v + 1);
            else setVersion((v) => v + 1);
          }}
        >
          <RefreshCw size={14} />
          {tab === "logs"
            ? "Refresh logs"
            : tab === "events"
              ? "Refresh events"
              : "Refresh details"}
        </button>
        <button className="button primary" onClick={onClose}>
          Done
        </button>
      </div>
    </Modal>
  );
}
