import { useEffect, useMemo, useState } from "react";
import {
  Archive,
  ArrowLeft,
  CircleCheck,
  Download,
  FolderOpen,
  GitBranch,
  Moon,
  PlugZap,
  Plus,
  Radar,
  RefreshCw,
  Save,
  Search,
  Sun,
  TriangleAlert,
  Upload,
} from "lucide-react";
import { errorText } from "./api";
import {
  archiveApi,
  type ArchiveDetail,
  type ArchiveIssue,
  type ArchiveResource,
  type ArchiveSummary,
  type ArchiveSyncItem,
} from "./archive-api";
import { ConfigCodeEditor } from "./ConfigCodeEditor";
import { WorkspaceNavigation } from "./WorkspaceNavigation";
import "./archive.css";

const KINDS = ["Product", "Service", "Domain", "Host", "Cluster", "Datastore", "Registry", "Provider"] as const;
const PLURAL: Record<string, string> = {
  Product: "Products",
  Service: "Services",
  Domain: "Domains",
  Host: "Hosts",
  Cluster: "Clusters",
  Datastore: "Datastores",
  Registry: "Registries",
  Provider: "Accounts",
};
const STAGES = ["idea", "building", "live", "paused", "sunset"];

type View = { page: "overview" } | { page: "kind"; kind: string } | { page: "issues" } | { page: "resource"; ref: string };

export function ArchiveConsole() {
  const [theme, setTheme] = useState<"dark" | "light">(() =>
    localStorage.getItem("devopspad-ops-theme") === "light" ? "light" : "dark",
  );
  const [summary, setSummary] = useState<ArchiveSummary>();
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState("");
  const [view, setView] = useState<View>({ page: "overview" });
  const [query, setQuery] = useState("");
  const [sync, setSync] = useState<ArchiveSyncItem[]>();
  const [gitOpen, setGitOpen] = useState(false);

  async function reload() {
    try {
      setSummary(await archiveApi.GetArchive());
      setError("");
    } catch (e) {
      setSummary(undefined);
      setError(errorText(e));
    }
  }
  useEffect(() => {
    reload();
  }, []);

  async function run(label: string, fn: () => Promise<string | void>) {
    setBusy(label);
    setNotice("");
    try {
      const message = await fn();
      if (message) setNotice(message);
      await reload();
    } catch (e) {
      setNotice(errorText(e));
    } finally {
      setBusy("");
    }
  }

  const counts = useMemo(() => {
    const c: Record<string, number> = {};
    for (const r of summary?.resources || []) c[r.kind] = (c[r.kind] || 0) + 1;
    return c;
  }, [summary]);

  const open = (ref: string) => setView({ page: "resource", ref });
  const git = summary?.git;

  return (
    <div className="ops-console archive-console" data-theme={theme}>
      <nav className="ops-workspace-rail" aria-label="Main navigation">
        <WorkspaceNavigation active="archive" disabled={Boolean(busy)} />
      </nav>
      <header className="ops-top">
        <a className="ops-brand" href="#archive">
          DevOps<span>Pad</span>
        </a>
        <nav aria-label="Archive">
          <button className="active" onClick={() => setView({ page: "overview" })}>
            Archive{summary ? ` · ${summary.name}` : ""}
          </button>
        </nav>
        <div className="ops-top-right">
          {summary && (
            <>
              <button
                className="archive-git-chip"
                aria-label="Git status"
                aria-expanded={gitOpen}
                onClick={() => setGitOpen(!gitOpen)}
                disabled={Boolean(busy)}
              >
                <GitBranch size={14} />
                {git ? (
                  <>
                    {git.branch || "no branch"}
                    {git.ahead > 0 && ` ↑${git.ahead}`}
                    {git.behind > 0 && ` ↓${git.behind}`}
                    {git.changes.length > 0 && ` · ${git.changes.length} changed`}
                  </>
                ) : (
                  "Not a Git repository"
                )}
              </button>
              <button
                disabled={Boolean(busy)}
                title="Read Cloudflare, domains, certificates, clusters and hosts (read-only). Results stay on this computer."
                onClick={() =>
                  run("Scanning", async () => {
                    const r = await archiveApi.ScanArchive("");
                    return [`Scanned ${r.observations.length} resources.`, ...r.notes].join(" ");
                  })
                }
              >
                <Radar size={15} />
                {busy === "Scanning" ? "Scanning…" : "Scan"}
              </button>
              <button
                disabled={Boolean(busy)}
                onClick={() =>
                  run("Planning", async () => {
                    setSync(await archiveApi.SyncArchiveConnections(true));
                  })
                }
              >
                <PlugZap size={15} />
                Sync connections
              </button>
            </>
          )}
          <button
            aria-label={theme === "dark" ? "Switch to light theme" : "Switch to dark theme"}
            onClick={() => {
              const next = theme === "dark" ? "light" : "dark";
              setTheme(next);
              localStorage.setItem("devopspad-ops-theme", next);
            }}
          >
            {theme === "dark" ? <Sun size={16} /> : <Moon size={16} />}
          </button>
        </div>
      </header>
      <aside className="ops-sidebar archive-sidebar">
        {summary && (
          <>
            <label className="archive-search">
              <Search size={14} />
              <input
                aria-label="Search the archive"
                placeholder="Search products, domains…"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
              />
            </label>
            <div className="ops-nav-group">
              <p>Archive</p>
              <button className={view.page === "overview" ? "selected" : ""} onClick={() => setView({ page: "overview" })}>
                Overview
              </button>
              <button className={view.page === "issues" ? "selected" : ""} onClick={() => setView({ page: "issues" })}>
                Validation
                <span className={summary.errors.length ? "archive-count bad" : "archive-count"}>{summary.errors.length || "ok"}</span>
              </button>
            </div>
            <div className="ops-nav-group">
              <p>Resources</p>
              {KINDS.map((k) => (
                <button
                  key={k}
                  className={view.page === "kind" && view.kind === k ? "selected" : ""}
                  onClick={() => setView({ page: "kind", kind: k })}
                >
                  {PLURAL[k]}
                  <span className="archive-count">{counts[k] || 0}</span>
                </button>
              ))}
            </div>
            <div className="ops-sidebar-bottom">
              <span title={summary.dir}>
                <Archive size={13} /> {summary.dir.split("/").slice(-2).join("/")}
              </span>
            </div>
          </>
        )}
      </aside>
      <main className="ops-main archive-main">
        {notice && (
          <div className="archive-notice" role="status">
            {notice}
            <button onClick={() => setNotice("")}>Dismiss</button>
          </div>
        )}
        {!summary ? (
          <EmptyArchive error={error} busy={Boolean(busy)} onSelect={(create) => run(create ? "Creating" : "Opening", async () => {
            const s = await archiveApi.SelectArchive(create);
            setSummary(s);
          })} />
        ) : (
          <>
            {gitOpen && <GitPanel summary={summary} busy={Boolean(busy)} run={run} />}
            {sync && (
              <SyncPanel
                items={sync}
                busy={Boolean(busy)}
                onApply={() =>
                  run("Syncing", async () => {
                    const done = await archiveApi.SyncArchiveConnections(false);
                    setSync(undefined);
                    const changed = done.filter((i) => i.action === "create" || i.action === "update").length;
                    return `Desktop connections updated (${changed} changed).`;
                  })
                }
                onClose={() => setSync(undefined)}
              />
            )}
            {query.trim() ? (
              <SearchResults summary={summary} query={query} open={open} />
            ) : view.page === "overview" ? (
              <Overview summary={summary} open={open} />
            ) : view.page === "issues" ? (
              <Issues issues={summary.errors} warnings={summary.warnings} />
            ) : view.page === "kind" ? (
              <KindList summary={summary} kind={view.kind} open={open} onCreated={(msg) => run("Creating", async () => msg)} />
            ) : (
              <ResourceDetail key={view.ref} refName={view.ref} summary={summary} open={open} onSaved={reload} back={() => setView({ page: "overview" })} />
            )}
          </>
        )}
      </main>
    </div>
  );
}

function EmptyArchive({ error, busy, onSelect }: { error: string; busy: boolean; onSelect: (create: boolean) => void }) {
  return (
    <section className="ops-empty archive-empty">
      <Archive size={28} />
      <strong>No archive open</strong>
      <p>
        The archive is a private Git repository that records every product, service, domain, host and account of your
        product matrix. It stores secret references, never secret values.
      </p>
      {error && !error.startsWith("No archive selected") && <p className="archive-error">{error}</p>}
      <div className="archive-actions">
        <button className="ops-empty-action" disabled={busy} onClick={() => onSelect(true)}>
          <Plus size={14} /> Create archive
        </button>
        <button className="ops-empty-action" disabled={busy} onClick={() => onSelect(false)}>
          <FolderOpen size={14} /> Open archive folder
        </button>
      </div>
      <p className="archive-hint">Or from a terminal: devopspadcli archive init ~/opc-archive</p>
    </section>
  );
}

function Overview({ summary, open }: { summary: ArchiveSummary; open: (ref: string) => void }) {
  const domains = summary.resources.filter((r) => r.kind === "Domain").length;
  const problems = summary.drift.filter((d) => d.severity !== "info");
  return (
    <>
      <section className="archive-tiles" aria-label="Summary">
        <Tile label="Products" value={summary.matrix.length} />
        <Tile label="Live" value={summary.matrix.filter((m) => m.stage === "live").length} />
        <Tile label="Domains" value={domains} />
        <Tile label="Hosts" value={summary.resources.filter((r) => r.kind === "Host").length} />
        <Tile label="Due soon" value={summary.renewals.length} tone={summary.renewals.some((r) => r.days < 0) ? "bad" : summary.renewals.length ? "warn" : undefined} />
        <Tile label="Validation errors" value={summary.errors.length} tone={summary.errors.length ? "bad" : undefined} />
      </section>
      <section className="ops-panel">
        <div className="ops-panel-head">
          <div>
            <h2>Due soon</h2>
            <p>Domains, certificates, hosts and subscriptions expiring within the warning window, including the last scan.</p>
          </div>
        </div>
        {summary.renewals.length === 0 ? (
          <p className="archive-quiet">Nothing expires soon.</p>
        ) : (
          <table className="ops-table">
            <thead>
              <tr><th>Due</th><th>Days</th><th>What</th><th>Resource</th><th>Auto-renew</th><th>Used by</th></tr>
            </thead>
            <tbody>
              {summary.renewals.map((r, i) => (
                <tr key={i} className={r.days < 0 ? "archive-row-bad" : ""}>
                  <td>{r.due}</td>
                  <td>{r.days}</td>
                  <td>{r.what}</td>
                  <td><RefLink value={r.ref} open={open} /></td>
                  <td>{r.autoRenew == null ? "–" : r.autoRenew ? "yes" : "no"}</td>
                  <td>{short(r.usedBy)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
      <section className="ops-panel">
        <div className="ops-panel-head">
          <div>
            <h2>Drift</h2>
            <p>Differences between the archive and the last scan. Scan reads live data; results are never committed.</p>
          </div>
        </div>
        <DriftTable drift={problems.length ? problems : summary.drift.slice(0, 50)} open={open} />
      </section>
    </>
  );
}

function Tile({ label, value, tone }: { label: string; value: number; tone?: "bad" | "warn" }) {
  return (
    <div className={`archive-tile ${tone || ""}`}>
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function DriftTable({ drift, open }: { drift: ArchiveSummary["drift"]; open: (ref: string) => void }) {
  if (!drift.length) return <p className="archive-quiet"><CircleCheck size={14} /> The archive matches the last scan.</p>;
  return (
    <table className="ops-table">
      <thead>
        <tr><th>Severity</th><th>Resource</th><th>Field</th><th>Archive</th><th>Observed</th><th>Message</th></tr>
      </thead>
      <tbody>
        {drift.map((d, i) => (
          <tr key={i}>
            <td><span className={`archive-severity ${d.severity}`}>{d.severity}</span></td>
            <td><RefLink value={d.ref} open={open} /></td>
            <td>{d.field || "–"}</td>
            <td>{d.archive || "–"}</td>
            <td>{d.observed || "–"}</td>
            <td>{d.message}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function Issues({ issues, warnings }: { issues: ArchiveIssue[]; warnings: ArchiveIssue[] }) {
  return (
    <section className="ops-panel">
      <div className="ops-panel-head">
        <div>
          <h2>Validation</h2>
          <p>Commits are refused while there are errors. Warnings do not block commits.</p>
        </div>
      </div>
      {issues.length === 0 && warnings.length === 0 ? (
        <p className="archive-quiet"><CircleCheck size={14} /> The archive is valid.</p>
      ) : (
        <table className="ops-table">
          <thead><tr><th></th><th>File</th><th>Field</th><th>Code</th><th>Message</th></tr></thead>
          <tbody>
            {[...issues.map((i) => ({ ...i, level: "error" })), ...warnings.map((i) => ({ ...i, level: "warn" }))].map((i, n) => (
              <tr key={n}>
                <td><span className={`archive-severity ${i.level}`}>{i.level}</span></td>
                <td>{i.file}{i.line ? `:${i.line}` : ""}</td>
                <td>{i.path || "–"}</td>
                <td><code>{i.code}</code></td>
                <td>{i.message}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}

function KindList({ summary, kind, open, onCreated }: { summary: ArchiveSummary; kind: string; open: (ref: string) => void; onCreated: (msg: string) => void }) {
  const [stage, setStage] = useState("");
  const [name, setName] = useState("");
  const [product, setProduct] = useState("");
  const [createError, setCreateError] = useState("");
  const list = summary.resources.filter((r) => r.kind === kind);
  const products = summary.resources.filter((r) => r.kind === "Product");
  async function create() {
    setCreateError("");
    try {
      const file = await archiveApi.NewArchiveResource(kind.toLowerCase(), name.trim(), product);
      setName("");
      onCreated(`Created ${file}. Edit it, then commit from the Git panel.`);
      open(kind === "Service" ? `service:${product}/${name.trim()}` : `${kind.toLowerCase()}:${name.trim()}`);
    } catch (e) {
      setCreateError(errorText(e));
    }
  }
  return (
    <section className="ops-panel">
      <div className="ops-panel-head">
        <div>
          <h2>{PLURAL[kind]}</h2>
          <p>{list.length} in the archive.</p>
        </div>
      </div>
      <div className="ops-toolbar">
        {kind === "Product" && (
          <select aria-label="Stage" value={stage} onChange={(e) => setStage(e.target.value)}>
            <option value="">All stages</option>
            {STAGES.map((s) => <option key={s}>{s}</option>)}
          </select>
        )}
        <span className="ops-toolbar-spacer" />
        {kind === "Service" && (
          <select aria-label="Product of the new service" value={product} onChange={(e) => setProduct(e.target.value)}>
            <option value="">Product…</option>
            {products.map((p) => <option key={p.name}>{p.name}</option>)}
          </select>
        )}
        <input aria-label={`New ${kind} name`} placeholder={kind === "Domain" ? "example.com" : "name"} value={name} onChange={(e) => setName(e.target.value)} />
        <button disabled={!name.trim() || (kind === "Service" && !product)} onClick={create}>
          <Plus size={14} /> New {kind.toLowerCase()}
        </button>
      </div>
      {createError && <p className="archive-error">{createError}</p>}
      {kind === "Product" ? (
        <table className="ops-table">
          <thead><tr><th>Product</th><th>Stage</th><th>Services</th><th>Domains</th><th>Environments</th><th>Hosts</th></tr></thead>
          <tbody>
            {summary.matrix.filter((m) => !stage || m.stage === stage).map((m) => (
              <tr key={m.product} className="archive-click" onClick={() => open(`product:${m.product}`)}>
                <td><strong>{m.title || m.product}</strong></td>
                <td><span className={`archive-stage ${m.stage}`}>{m.stage}</span></td>
                <td>{short(m.services)}</td>
                <td>{short(m.domains)}</td>
                <td>{short(m.environments)}</td>
                <td>{short(m.hosts)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : (
        <ResourceTable list={list} open={open} />
      )}
    </section>
  );
}

function ResourceTable({ list, open }: { list: ArchiveResource[]; open: (ref: string) => void }) {
  if (!list.length) return <p className="archive-quiet">None yet.</p>;
  return (
    <table className="ops-table">
      <thead>
        <tr>{list[0].header.map((h) => <th key={h}>{h}</th>)}<th>USED BY</th></tr>
      </thead>
      <tbody>
        {list.map((r) => (
          <tr key={r.ref} className="archive-click" onClick={() => open(r.ref)}>
            {r.row.map((c, i) => <td key={i}>{i === 0 ? <strong>{c}</strong> : c}</td>)}
            <td>{short(r.usedBy)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function SearchResults({ summary, query, open }: { summary: ArchiveSummary; query: string; open: (ref: string) => void }) {
  const q = query.trim().toLowerCase();
  const hits = summary.resources.filter(
    (r) =>
      r.ref.toLowerCase().includes(q) ||
      r.title.toLowerCase().includes(q) ||
      Object.entries(r.labels || {}).some(([k, v]) => `${k}=${v}`.toLowerCase().includes(q)),
  );
  return (
    <section className="ops-panel">
      <div className="ops-panel-head">
        <div>
          <h2>Search</h2>
          <p>{hits.length} matches for “{query}”. Labels match as key=value, for example line=tools.</p>
        </div>
      </div>
      <table className="ops-table">
        <thead><tr><th>Resource</th><th>Kind</th><th>Title</th><th>Used by</th></tr></thead>
        <tbody>
          {hits.slice(0, 200).map((r) => (
            <tr key={r.ref} className="archive-click" onClick={() => open(r.ref)}>
              <td><strong>{r.ref}</strong></td>
              <td>{r.kind}</td>
              <td>{r.title || "–"}</td>
              <td>{short(r.usedBy)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}

function ResourceDetail({ refName, summary, open, onSaved, back }: { refName: string; summary: ArchiveSummary; open: (ref: string) => void; onSaved: () => Promise<void>; back: () => void }) {
  const [detail, setDetail] = useState<ArchiveDetail>();
  const [text, setText] = useState("");
  const [revision, setRevision] = useState("");
  const [issues, setIssues] = useState<ArchiveIssue[]>([]);
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  useEffect(() => {
    archiveApi
      .DescribeArchiveResource(refName)
      .then((d) => {
        setDetail(d);
        setText(d.text);
        setRevision(d.revision);
        setIssues(summary.errors.filter((i) => i.file === d.description.file));
      })
      .catch((e) => setError(errorText(e)));
  }, [refName]);
  if (error) return <section className="ops-panel"><p className="archive-error">{error}</p></section>;
  if (!detail) return <section className="ops-panel"><p className="archive-quiet">Loading…</p></section>;
  const d = detail.description;
  const dirty = text !== detail.text;
  async function save() {
    setSaving(true);
    setError("");
    try {
      const r = await archiveApi.SaveArchiveFile(d.file, text, revision);
      setRevision(r.revision);
      setIssues(r.issues);
      setDetail({ ...detail!, text });
      await onSaved();
    } catch (e) {
      setError(errorText(e));
    } finally {
      setSaving(false);
    }
  }
  return (
    <>
      <section className="ops-panel">
        <div className="ops-panel-head">
          <div>
            <button className="archive-back" onClick={back}><ArrowLeft size={14} /> Overview</button>
            <h2>{d.ref}</h2>
            <p>{d.file} · used by {short(d.usedByProducts)}</p>
          </div>
          <button disabled={!dirty || saving} onClick={save}>
            <Save size={14} /> {saving ? "Saving…" : "Save"}
          </button>
        </div>
        {issues.length > 0 && (
          <ul className="archive-issues">
            {issues.map((i, n) => (
              <li key={n}><TriangleAlert size={13} /> {i.line ? `Line ${i.line}: ` : ""}{i.path ? `${i.path} – ` : ""}{i.message} <code>{i.code}</code></li>
            ))}
          </ul>
        )}
        <ConfigCodeEditor filename={d.file} label={`${d.ref} YAML`} value={text} editable onChange={setText} />
        <p className="archive-hint">Saved files are validated immediately. Commit them from the Git panel; never write secret values here.</p>
      </section>
      {d.deployments && d.deployments.length > 0 && (
        <section className="ops-panel">
          <div className="ops-panel-head"><div><h2>Deployments</h2><p>Environment defaults applied.</p></div></div>
          <table className="ops-table">
            <thead><tr><th>Environment</th><th>Runtime</th><th>Where</th><th>What</th></tr></thead>
            <tbody>
              {d.deployments.map((dep, i) => (
                <tr key={i}>
                  <td>{dep.environment}</td>
                  <td>{dep.runtime}</td>
                  <td>{dep.runtime === "host" ? short(dep.hosts || []) : dep.runtime === "static" ? dep.provider : `${dep.cluster || "–"}/${dep.namespace || "–"}`}</td>
                  <td>{dep.workload ? `${dep.workload.kind}/${dep.workload.name}` : dep.selector || dep.unit || "–"}</td>
                </tr>
              ))}
            </tbody>
          </table>
          {d.releaseNotes && <p className="archive-hint">Release notes: {d.releaseNotes}</p>}
        </section>
      )}
      <section className="ops-panel archive-relations">
        <div>
          <h3>References</h3>
          {d.references.length ? d.references.map((u, i) => (
            <p key={i}><RefLink value={u.to} open={open} />{u.part ? `#${u.part}` : ""} <span>{u.field}</span></p>
          )) : <p className="archive-quiet">None</p>}
        </div>
        <div>
          <h3>Referenced by</h3>
          {d.referencedBy.length ? d.referencedBy.map((u, i) => (
            <p key={i}><RefLink value={u.from} open={open} /> <span>{u.field}</span></p>
          )) : <p className="archive-quiet">Nothing references this.</p>}
        </div>
        {d.secrets.length > 0 && (
          <div>
            <h3>Secret references</h3>
            {d.secrets.map((s, i) => <p key={i}><code>{s.ref}</code> <span>{s.field}</span></p>)}
          </div>
        )}
        {d.notes.length > 0 && (
          <div>
            <h3>Notes</h3>
            {d.notes.map((n) => <p key={n}>{n}</p>)}
          </div>
        )}
      </section>
      {d.recentRecords.length > 0 && (
        <section className="ops-panel">
          <div className="ops-panel-head"><div><h2>Recent records</h2></div></div>
          <table className="ops-table">
            <thead><tr><th>Started</th><th>Action</th><th>Status</th><th>Actor</th><th>Message</th></tr></thead>
            <tbody>
              {d.recentRecords.map((r) => (
                <tr key={r.id}>
                  <td>{r.startedAt}</td>
                  <td>{r.action}</td>
                  <td><span className={`ops-status ${r.status}`}>{r.status}</span></td>
                  <td>{r.actor.type}/{r.actor.name}</td>
                  <td>{r.message || "–"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      )}
      {detail.observation && <DomainEdge observation={detail.observation} />}
      {detail.observation && (
        <section className="ops-panel">
          <div className="ops-panel-head"><div><h2>Last scan</h2><p>{detail.observation.observedAt} · local only, not committed</p></div></div>
          <pre className="archive-json">{JSON.stringify(detail.observation, null, 2)}</pre>
        </section>
      )}
    </>
  );
}

type ObservedDomain = {
  workerRoutes?: { pattern: string; script?: string }[];
  records?: { name: string; type: string; content: string; proxied?: boolean }[];
};

function DomainEdge({ observation }: { observation: Record<string, unknown> }) {
  const domain = observation.domain as ObservedDomain | undefined;
  if (!domain || (!domain.workerRoutes?.length && !domain.records?.length)) return null;
  return (
    <section className="ops-panel">
      <div className="ops-panel-head">
        <div>
          <h2>Cloudflare</h2>
          <p>Workers routes and DNS records from the last scan. Change them with devopspadcli edge routes|dns (recorded).</p>
        </div>
      </div>
      {domain.workerRoutes && domain.workerRoutes.length > 0 && (
        <table className="ops-table" aria-label="Workers routes">
          <thead><tr><th>Route pattern</th><th>Worker</th></tr></thead>
          <tbody>
            {domain.workerRoutes.map((r) => (
              <tr key={r.pattern}><td>{r.pattern}</td><td>{r.script || "none (origin)"}</td></tr>
            ))}
          </tbody>
        </table>
      )}
      {domain.records && domain.records.length > 0 && (
        <table className="ops-table" aria-label="DNS records">
          <thead><tr><th>Name</th><th>Type</th><th>Content</th><th>Proxied</th></tr></thead>
          <tbody>
            {domain.records.map((r, i) => (
              <tr key={i}><td>{r.name}</td><td>{r.type}</td><td>{r.content}</td><td>{r.proxied ? "yes" : "no"}</td></tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}

function GitPanel({ summary, busy, run }: { summary: ArchiveSummary; busy: boolean; run: (label: string, fn: () => Promise<string | void>) => Promise<void> }) {
  const [message, setMessage] = useState("archive(desktop): update archive");
  const git = summary.git;
  return (
    <section className="ops-panel archive-git">
      <div className="ops-panel-head">
        <div>
          <h2>Git</h2>
          <p>
            {git
              ? `${git.branch}${git.upstream ? ` → ${git.upstream}` : " (no upstream)"} · ${git.changes.length} changed files`
              : summary.gitError}
          </p>
        </div>
        <div className="archive-actions">
          <button disabled={busy || !git} onClick={() => run("Fetching", () => archiveApi.ArchiveGit("fetch", ""))}>
            <RefreshCw size={14} /> Check remote
          </button>
          <button disabled={busy || !git} onClick={() => run("Pulling", () => archiveApi.ArchiveGit("pull", ""))}>
            <Download size={14} /> Pull
          </button>
          <button disabled={busy || !git} onClick={() => run("Pushing", () => archiveApi.ArchiveGit("push", ""))}>
            <Upload size={14} /> Push
          </button>
        </div>
      </div>
      {git && git.changes.length > 0 && <pre className="archive-json">{git.changes.join("\n")}</pre>}
      <div className="ops-toolbar">
        <input aria-label="Commit message" className="archive-commit-message" value={message} onChange={(e) => setMessage(e.target.value)} />
        <button disabled={busy || !git || summary.errors.length > 0 || !git.changes.length} onClick={() => run("Committing", () => archiveApi.ArchiveGit("commit", message))}>
          Commit
        </button>
      </div>
      <p className="archive-hint">Format: type(scope): summary — types archive, config, record, note. Commit validates the archive and regenerates INDEX.md.</p>
    </section>
  );
}

function SyncPanel({ items, busy, onApply, onClose }: { items: ArchiveSyncItem[]; busy: boolean; onApply: () => void; onClose: () => void }) {
  const changes = items.filter((i) => i.action === "create" || i.action === "update");
  return (
    <section className="ops-panel">
      <div className="ops-panel-head">
        <div>
          <h2>Sync connections</h2>
          <p>Desktop connections for the archive's clusters and datastores. Secrets come from the referenced keychain entries.</p>
        </div>
        <div className="archive-actions">
          <button onClick={onClose} disabled={busy}>Close</button>
          <button disabled={busy || !changes.length} onClick={onApply}>Apply {changes.length} changes</button>
        </div>
      </div>
      <table className="ops-table">
        <thead><tr><th>Resource</th><th>Action</th><th>Note</th></tr></thead>
        <tbody>
          {items.map((i) => (
            <tr key={i.ref}><td>{i.ref}</td><td>{i.action}</td><td>{i.reason || "–"}</td></tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}

function RefLink({ value, open }: { value: string; open: (ref: string) => void }) {
  if (!/^(provider|host|cluster|datastore|registry|domain|product|service):/.test(value)) return <>{value}</>;
  const ref = value.split("#")[0].split("@")[0];
  return (
    <button className="archive-ref" onClick={(e) => { e.stopPropagation(); open(ref); }}>
      {value}
    </button>
  );
}

function short(list: string[]) {
  if (!list.length) return "–";
  const names = list.map((s) => s.replace(/^product:/, ""));
  return names.length > 4 ? `${names.slice(0, 4).join(", ")} +${names.length - 4}` : names.join(", ");
}
