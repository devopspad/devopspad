import { useState } from "react";
import { Check, CircleAlert, Clock3, Terminal, Rocket } from "lucide-react";
import { Modal } from "./Modal";
import type { OpsArtifact, OpsRun } from "./ops-api";

const at = (s?: string) =>
  s ? new Date(s).toLocaleString("en-US", { hour12: false }) : "—";
const status = (s: string) =>
  s === "succeeded"
    ? "Reported"
    : s === "failed"
      ? "Failed"
      : s === "cancelled"
        ? "Cancelled"
        : "Running / unverified";
export function OpsRuns({
  runs,
  artifacts,
  onRelease,
  selectedId,
  onSelected,
}: {
  runs: OpsRun[];
  artifacts: OpsArtifact[];
  onRelease: (a: OpsArtifact) => void;
  selectedId?: string;
  onSelected: (id?: string) => void;
}) {
  const [kind, setKind] = useState("");
  const [query, setQuery] = useState("");
  const selected = runs.find((r) => r.id === selectedId);
  const visible = runs.filter(
    (r) =>
      (!kind || r.kind === kind) &&
      `${r.version} ${r.actor.name} ${r.source.commit} ${r.id}`
        .toLowerCase()
        .includes(query.toLowerCase()),
  );
  return (
    <div className="ops-table-pad">
      <div className="ops-run-filter">
        <select
          aria-label="Run type"
          value={kind}
          onChange={(e) => setKind(e.target.value)}
        >
          <option value="">All run types</option>
          <option value="build">AI build</option>
          <option value="release">External release</option>
        </select>
        <input
          aria-label="Filter runs"
          placeholder="Version, commit or actor"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        <span className="ops-muted">Reads local records every 10s</span>
      </div>
      <div className="ops-table-wrap bordered">
        <table className="ops-table">
          <thead>
            <tr>
              <th>Run / version</th>
              <th>Commit</th>
              <th>Actor</th>
              <th>Target</th>
              <th>Status</th>
              <th>Started</th>
              <th className="ops-right">Actions</th>
            </tr>
          </thead>
          <tbody>
            {visible.map((r) => (
              <tr key={r.id}>
                <td>
                  <button
                    className="ops-link neutral"
                    onClick={() => onSelected(r.id)}
                  >
                    <Terminal size={15} />
                    {r.kind === "build" ? "Build" : "Release"} · {r.version}
                  </button>
                  <small>{r.id.slice(0, 8)}</small>
                </td>
                <td>
                  <code>{r.source.commit.slice(0, 12)}</code>
                  <small>
                    {r.source.branch}
                    {r.source.dirty ? " · uncommitted changes" : ""}
                  </small>
                </td>
                <td>
                  {r.actor.name}
                  <small>
                    {r.actor.kind === "ai" ? "AI" : "Human"}
                  </small>
                </td>
                <td>
                  {r.target
                    ? `${r.target.environment} / ${r.target.workload}`
                    : "Artifacts"}
                </td>
                <td>
                  <span className={`ops-status ${r.status}`}>
                    {status(r.status)}
                  </span>
                </td>
                <td className="ops-muted">{at(r.startedAt)}</td>
                <td className="ops-right">
                  <button className="ops-link" onClick={() => onSelected(r.id)}>
                    Details
                  </button>
                  {r.kind === "build" &&
                    r.status === "succeeded" &&
                    artifacts.some((a) => a.runId === r.id) && (
                      <button
                        className="ops-link"
                        onClick={() =>
                          onRelease(artifacts.find((a) => a.runId === r.id)!)
                        }
                      >
                        <Rocket size={14} />
                        Release artifact
                      </button>
                    )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {!visible.length && (
          <div className="ops-empty">
            <Terminal size={28} />
            <strong>No runs yet</strong>
            <p>
              Have the AI call devopspadcli runs record
              to report the start, then each step and the result.
            </p>
          </div>
        )}
      </div>
      <p className="ops-footnote">
        Successful builds are added to Artifacts automatically. This page shows what the executor reported; an external release report does not touch the cluster and is not verified by DevOpsPad.
      </p>
      {selected && (
        <Modal
          wide
          title={`${selected.kind === "build" ? "Build" : "Release"} ${selected.version}`}
          subtitle={`${selected.actor.name} · ${selected.id}`}
          onClose={() => onSelected(undefined)}
        >
          <div className="ops-dialog-body ops-form">
            <span className={`ops-status ${selected.status}`}>
              {status(selected.status)}
            </span>
            <div className="ops-release-images">
              <label>
                Source
                <code>
                  {selected.source.repository}
                  <br />
                  {selected.source.commit} · {selected.source.branch}
                  {selected.source.dirty ? " · uncommitted changes in the working tree" : ""}
                </code>
              </label>
              <label>
                AI / session
                <code>
                  {selected.actor.name} · {selected.actor.sessionId}
                </code>
              </label>
              <label>
                Build / release method<code>{selected.recipe}</code>
              </label>
              {selected.image && (
                <label>
                  Image digest<code>{selected.image}</code>
                </label>
              )}
              {selected.target && (
                <label>
                  Release target
                  <code>
                    {selected.connection} / {selected.namespace} /{" "}
                    {selected.target.workload} / {selected.target.container}
                    <br />
                    Build {selected.target.buildId}
                  </code>
                </label>
              )}
            </div>
            <ol className="ops-steps ops-run-steps">
              {selected.events.map((saved) => (
                <li
                  key={saved.event.id}
                  className={
                    saved.event.step?.status === "failed" ||
                    (saved.event.finish &&
                      saved.event.finish.status !== "succeeded")
                      ? "failed"
                      : ""
                  }
                >
                  {saved.event.step?.status === "failed" ||
                  (saved.event.finish &&
                    saved.event.finish.status !== "succeeded") ? (
                    <CircleAlert size={15} />
                  ) : saved.event.type === "start" ? (
                    <Clock3 size={15} />
                  ) : (
                    <Check size={15} />
                  )}
                  <div>
                    <strong>
                      {saved.event.type === "start"
                        ? "Started"
                        : saved.event.type === "finish"
                          ? status(selected.status)
                          : saved.event.step?.name}
                    </strong>
                    <small>{at(saved.at)}</small>
                    <p>
                      {saved.event.step?.message || saved.event.finish?.message}
                    </p>
                    {saved.event.step?.logPath && (
                      <code>Log: {saved.event.step.logPath}</code>
                    )}
                  </div>
                </li>
              ))}
            </ol>
            <p>
              {selected.message ||
                "No finish reported yet. If the executor was interrupted, check and then report failed or cancelled."}
            </p>
            <p className="ops-muted">
              {at(selected.startedAt)} → {at(selected.finishedAt)}
            </p>
          </div>
        </Modal>
      )}
    </div>
  );
}
