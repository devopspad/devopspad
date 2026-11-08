import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type FormEvent,
  type ReactNode,
} from "react";
import {
  Activity,
  ArrowDownToLine,
  ArrowLeft,
  ArrowUpFromLine,
  Bell,
  Boxes,
  Check,
  ChevronDown,
  ChevronRight,
  CircleAlert,
  Database,
  FileCode2,
  KeyRound,
  FileText,
  FolderOpen,
  GitBranch,
  HardDrive,
  Layers3,
  LoaderCircle,
  LockKeyhole,
  Package,
  Plus,
  RefreshCw,
  Rocket,
  Search,
  Server,
  Sun,
  Terminal,
  PlugZap,
  Moon,
  Settings2,
  X,
} from "lucide-react";
import { api, desktop, errorText } from "./api";
import { Modal } from "./Modal";
import type { Connection, QueryResult, Workload } from "./types";
import {
  type OpsApplication,
  type OpsArtifact,
  type OpsCluster,
  type OpsEnvironment,
  type OpsMetric,
  type OpsPlan,
  type OpsPod,
  type OpsRecord,
  type OpsSnapshot,
} from "./ops-api";
import { InfraCluster } from "./InfraCluster";
import { ClusterInspector } from "./ClusterInspector";
import type { ClusterRef } from "./types";
import { WorkspaceNavigation } from "./WorkspaceNavigation";
import { OpsApplicationConfig } from "./OpsApplicationConfig";
import { OpsNginxConfig } from "./OpsNginxConfig";
import { OpsSecrets } from "./OpsSecrets";
import { parseDocument } from "yaml";
import { OpsRuns } from "./OpsRuns";
import { OpsConnectDialog } from "./OpsConnectDialog";
import "./ops.css";
type Page =
  | "nodes"
  | "runs"
  | "artifacts"
  | "releases"
  | "deployments"
  | "pods"
  | "metrics"
  | "logs"
  | "events"
  | "config"
  | "nginx"
  | "secrets"
  | "project"
  | "sharing";
const nav = [
  {
    group: "Cluster",
    items: [{ id: "nodes", label: "Nodes", icon: Server }],
  },
  {
    group: "Deploy",
    items: [
      { id: "runs", label: "AI runs", icon: Terminal },
      { id: "artifacts", label: "Artifacts", icon: Package },
      { id: "releases", label: "Releases", icon: Rocket },
      { id: "deployments", label: "Deployment", icon: Boxes },
      { id: "pods", label: "Pods", icon: Server },
    ],
  },
  {
    group: "Observe",
    items: [
      { id: "metrics", label: "Metrics", icon: Activity },
      { id: "logs", label: "Logs", icon: FileText },
      { id: "events", label: "Events", icon: Bell },
    ],
  },
  {
    group: "Configure",
    items: [
      { id: "config", label: "App config", icon: Settings2 },
      { id: "nginx", label: "Nginx config", icon: FileCode2 },
      { id: "secrets", label: "Secrets", icon: KeyRound },
      { id: "project", label: "Project mapping", icon: FileCode2 },
      { id: "sharing", label: "Sharing & sync", icon: GitBranch },
    ],
  },
];
const titles: Record<Page, string> = {
  nodes: "Nodes",
  runs: "AI runs",
  artifacts: "Artifacts",
  releases: "Releases",
  deployments: "Deployment",
  pods: "Pods",
  metrics: "Metrics",
  logs: "Logs",
  events: "Events",
  config: "App config",
  nginx: "Nginx config",
  secrets: "Secrets",
  project: "Project mapping",
  sharing: "Sharing & sync",
};
function operationName(record: Pick<OpsRecord, "action">) {
  if (record.action === "nginx") return "Nginx config";
  if (record.action === "config") return "Config update";
  if (record.action === "secret") return "Secret update";
  if (record.action === "patch") return "Deployment settings";
  if (record.action === "job") return "Job";
  return record.action === "scale"
    ? "Scale"
    : record.action === "restart"
      ? "Rolling restart"
      : record.action === "rollback"
        ? "Rollback"
        : "Release";
}
function operationStatus(record: Pick<OpsRecord, "action" | "status" | "origin">) {
  if (record.origin === "external" && record.status === "succeeded") return "Reported";
  if (record.status === "cancelled") return "Cancelled";
  const release = !record.action || record.action === "release";
  return record.status === "succeeded"
    ? release ? "Release complete" : "Done"
    : record.status === "running"
      ? "Running / unverified"
      : release ? "Release incomplete" : "Incomplete";
}
function date(s: string) {
  return s ? new Date(s).toLocaleString("en-US", { hour12: false }) : "—";
}
function age(s: string) {
  const h = Math.max(0, Math.floor((Date.now() - Date.parse(s)) / 3600000));
  return h < 24 ? `${h}h` : `${Math.floor(h / 24)}d ${h % 24}h`;
}
function Empty({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <div className="ops-empty">
      <Package size={28} strokeWidth={1.4} />
      <strong>{title}</strong>
      {children && <p>{children}</p>}
    </div>
  );
}
function ErrorNotice({ children }: { children: ReactNode }) {
  return children ? (
    <div className="ops-error" role="alert">
      <CircleAlert size={16} />
      <span>{children}</span>
    </div>
  ) : null;
}
function Status({ ready, total }: { ready: number; total: number }) {
  return (
    <span className={`ops-health ${ready === total ? "good" : "warn"}`}>
      <span>
        {ready} <small>/ {total}</small>
      </span>
      <progress
        max={Math.max(1, total)}
        value={ready}
        aria-label={`${ready} / ${total} ready`}
      />
    </span>
  );
}
function Refresher({ busy, onClick }: { busy: boolean; onClick: () => void }) {
  return (
    <button
      className="ops-icon"
      title="Refresh"
      aria-label="Refresh"
      onClick={onClick}
      disabled={busy}
    >
      {busy ? (
        <LoaderCircle size={18} className="spin" />
      ) : (
        <RefreshCw size={18} />
      )}
    </button>
  );
}
export function OpsConsole({ onWorkbench }: { onWorkbench: () => void }) {
  const [theme, setTheme] = useState(() =>
    localStorage.getItem("devopspad-ops-theme") === "light" ? "light" : "dark",
  );
  const [snapshot, setSnapshot] = useState<OpsSnapshot>();
  const [connections, setConnections] = useState<Connection[]>([]);
  const scopeKey = "devopspad-ops-navigation-local";
  const [navigation] = useState(() => {
    try {
      return JSON.parse(localStorage.getItem(scopeKey) || "{}");
    } catch {
      return {};
    }
  });
  const [environment, setEnvironment] = useState<string>(
    navigation?.environment || "",
  );
  const [application, setApplication] = useState<string>(
    navigation?.application || "",
  );
  const [page, setPage] = useState<Page>(
    Object.hasOwn(titles, navigation?.page) ? navigation.page : "artifacts",
  );
  useEffect(() => {
    localStorage.setItem(
      scopeKey,
      JSON.stringify({ environment, application, page }),
    );
  }, [scopeKey, environment, application, page]);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [connect, setConnect] = useState(false);
  const [inspector, setInspector] = useState<{
    connection: Connection;
    resource: ClusterRef;
  }>();
  useEffect(() => {
    if (!desktop) {
      setLoading(false);
      return;
    }
    let stale = false;
    setLoading(true);
    Promise.all([api.GetOpsProject(), api.ListConnections()])
      .then(([s, cs]) => {
        if (!stale) {
          setSnapshot(s);
          setConnections(cs);
        }
      })
      .catch((e) => {
        if (!stale) setError(errorText(e));
      })
      .finally(() => {
        if (!stale) setLoading(false);
      });
    return () => {
      stale = true;
    };
  }, []);
  useEffect(() => {
    if (
      !desktop ||
      busy ||
      page === "nodes" ||
      page === "config" ||
      page === "nginx" ||
      page === "secrets" ||
      page === "project" ||
      page === "sharing" ||
      connect
    )
      return;
    let stale = false;
    const refreshRecords = () => {
      void api
        .GetOpsProject()
        .then((s) => {
          if (!stale) setSnapshot(s);
        })
        .catch((e) => {
          if (!stale) setNotice(`Could not refresh run records: ${errorText(e)}`);
        });
    };
    const timer = window.setInterval(refreshRecords, 10000);
    window.addEventListener("focus", refreshRecords);
    return () => {
      stale = true;
      clearInterval(timer);
      window.removeEventListener("focus", refreshRecords);
    };
  }, [busy, page, connect]);
  function openConnect() {
    setConnect(true);
  }
  const env =
    snapshot?.project.environments.find((e) => e.name === environment) ||
    snapshot?.project.environments[0];
  const app =
    snapshot?.project.applications.find((a) => a.name === application) ||
    snapshot?.project.applications[0];
  const matches = connections.filter(
    (c) => c.name === env?.connection && (c.kind === "k8s" || c.kind === "k3s"),
  );
  const connection = matches.length === 1 ? matches[0] : undefined;
  async function reload() {
    setLoading(true);
    setError("");
    try {
      setSnapshot(await api.GetOpsProject());
      setConnections(await api.ListConnections());
    } catch (e) {
      setError(errorText(e));
    } finally {
      setLoading(false);
    }
  }
  return (
    <div className="ops-console" data-theme={theme}>
      <nav className="ops-workspace-rail" aria-label="Main navigation">
        <WorkspaceNavigation active="infra" disabled={busy} />
      </nav>
      <header className="ops-top">
        <a className="ops-brand" href="#infra">
          DevOps<span>Pad</span>
        </a>
        <nav aria-label="Workspace">
          <button
            className="active"
            disabled={busy}
            onClick={() => setPage("artifacts")}
          >
            Infra
          </button>
          <button
            onClick={() => {
              window.location.hash = "infra/resources";
            }}
            disabled={busy}
          >
            Cluster resources <ChevronDown size={13} />
          </button>
          <button disabled={busy} onClick={() => setPage("project")}>
            Project settings <ChevronDown size={13} />
          </button>
        </nav>
        <div className="ops-top-right">
          <button disabled={busy || !snapshot} onClick={openConnect}>
            <PlugZap size={15} />
            Connect K3s
          </button>
          <button
            aria-label={theme === "dark" ? "Switch to light theme" : "Switch to dark theme"}
            title={theme === "dark" ? "Switch to light theme" : "Switch to dark theme"}
            onClick={() => {
              const next = theme === "dark" ? "light" : "dark";
              setTheme(next);
              localStorage.setItem("devopspad-ops-theme", next);
            }}
          >
            {theme === "dark" ? <Sun size={16} /> : <Moon size={16} />}
          </button>
          <span>
            <HardDrive size={14} />
            Local project
          </span>
        </div>
      </header>
      <aside className="ops-sidebar">
        <div className="ops-context-select">
          <label>
            <span className="sr-only">Environment</span>
            <select
              aria-label="Environment"
              disabled={busy}
              value={env?.name || ""}
              onChange={(e) => setEnvironment(e.target.value)}
            >
              {snapshot?.project.environments.map((e) => (
                <option key={e.name}>{e.name}</option>
              ))}
              {!env && <option value="">Select environment</option>}
            </select>
            <LockKeyhole size={13} />
          </label>
          <label>
            <span className="sr-only">Application</span>
            <select
              aria-label="Application"
              disabled={busy}
              value={app?.name || ""}
              onChange={(e) => setApplication(e.target.value)}
            >
              {snapshot?.project.applications.map((a) => (
                <option key={a.name}>{a.name}</option>
              ))}
              {!app && <option value="">Select application</option>}
            </select>
          </label>
        </div>
        <div className="ops-app-meta">
          <GitBranch size={14} />
          <span>Kubernetes</span>
          <span className="ops-meta-dot" />
        </div>
        {nav.map((group) => (
          <div className="ops-nav-group" key={group.group}>
            <p>{group.group}</p>
            {group.items.map((item) => (
              <button
                key={item.id}
                className={page === item.id ? "selected" : ""}
                aria-current={page === item.id ? "page" : undefined}
                disabled={busy}
                onClick={() => setPage(item.id as Page)}
              >
                <item.icon size={18} strokeWidth={1.7} />
                {item.label}
              </button>
            ))}
          </div>
        ))}
        <div className="ops-sidebar-bottom">
          <span>
            <HardDrive size={14} />
            Data stays on this device
          </span>
          <button onClick={onWorkbench} disabled={busy}>
            <ArrowLeft size={14} />
            Database workbench
          </button>
        </div>
      </aside>
      <main className="ops-main">
        {notice && (
          <div className="ops-notice" role="status">
            {notice}
            <button aria-label="Dismiss notice" onClick={() => setNotice("")}>
              <X size={14} />
            </button>
          </div>
        )}
        {error && (
          <>
            <ErrorNotice>{error}</ErrorNotice>
            <button className="ops-btn" onClick={() => void reload()}>
              Reload project
            </button>
          </>
        )}
        {!desktop ? (
          <section className="ops-panel">
            <div className="ops-panel-head">
              <h1>Infra</h1>
            </div>
            <Empty title="Connect your infrastructure">
              <span>Open the DevOpsPad desktop app, add a cluster connection and connect an application.</span>
            </Empty>
          </section>
        ) : loading && !snapshot ? (
          <div className="ops-loading">
            <LoaderCircle size={20} className="spin" />
            Opening project…
          </div>
        ) : (
          snapshot && (
            <>
              {page === "project" || page === "sharing" ? (
                <ProjectSettings
                  key={snapshot.root + page}
                  page={page}
                  snapshot={snapshot}
                  busy={busy}
                  onSnapshot={setSnapshot}
                  onNotice={setNotice}
                  onBusy={setBusy}
                />
              ) : page === "nodes" ? (
                <section className="ops-panel ops-nodes-panel">
                  {env && connection ? (
                    <InfraCluster
                      key={`${connection.id}:${connection.context}:${env.name}`}
                      connection={connection}
                      namespace={env.namespace}
                      namespaces={[env.namespace]}
                      onNamespace={() => {}}
                      nodesOnly
                      onNode={(resource) =>
                        setInspector({ connection, resource })
                      }
                      onOpen={(item) =>
                        setInspector({
                          connection,
                          resource: {
                            context: connection.context,
                            namespace: item.namespace,
                            name: item.name,
                            uid: item.uid,
                            resource: item.resource,
                          },
                        })
                      }
                    />
                  ) : (
                    <>
                      <div className="ops-panel-head">
                        <h1>Nodes</h1>
                      </div>
                      <Empty title="Choose the cluster for this environment">
                        <span>
                          {matches.length > 1
                            ? "Two cluster connections share this name. Rename one in Cluster resources, then refresh."
                            : "Connect K3s, or map environments to cluster connections in Project mapping."}
                        </span>
                      </Empty>
                      <div className="ops-empty-action">
                        <button className="ops-btn" onClick={openConnect}>
                          Connect K3s
                        </button>
                      </div>
                    </>
                  )}
                </section>
              ) : page === "config" && env && app ? (
                <OpsApplicationConfig
                  projectRoot={snapshot.root}
                  saving={busy}
                  onBusy={setBusy}
                  key={`${snapshot.root}:${env.name}:${app.name}:${connection?.id}:${snapshot.revision}`}
                  app={app}
                  env={env}
                  connection={connection}
                  connectionError={
                    matches.length > 1
                      ? "Two cluster connections share this name. Rename one in Cluster resources, then refresh"
                      : "No cluster connection matches this environment. Add one in Cluster resources, then reload the project"
                  }
                />
              ) : page === "nginx" && env && app ? (
                <section className="ops-panel">
                  <div className="ops-panel-head"><div><h1>Nginx config</h1><p>{app.name} · {env.name} · Host config</p></div></div>
                  <div className="ops-application-config">
                    <OpsNginxConfig key={`${snapshot.root}:${env.name}:${app.name}:${app.nginxPath || ""}`} projectRoot={snapshot.root} environment={env.name} application={app.name} path={app.nginxPath} saving={busy} onBusy={setBusy} />
                  </div>
                </section>
              ) : page === "secrets" && env && app ? (
                <section className="ops-panel">
                  <div className="ops-panel-head"><div><h1>Secrets</h1><p>{app.name} · {env.name} · namespace {env.namespace}</p></div></div>
                  <OpsSecrets key={`${snapshot.root}:${env.name}:${app.name}`} environment={env.name} application={app.name} busy={busy} onBusy={setBusy} />
                </section>
              ) : env && app ? (
                <OpsScope
                  key={`${snapshot.root}:${env.name}:${app.name}:${connection?.id}`}
                  page={page}
                  onPage={setPage}
                  snapshot={snapshot}
                  onSnapshot={setSnapshot}
                  env={env}
                  app={app}
                  connection={connection}
                  connectionError={
                    matches.length > 1
                      ? "Two cluster connections share this name. Rename one in Cluster resources, then refresh"
                      : "No cluster connection matches this environment. Add one in Cluster resources, then reload the project"
                  }
                  busy={busy}
                  onBusy={setBusy}
                  onNotice={setNotice}
                  onReload={reload}
                />
              ) : (
                <section className="ops-panel">
                  <div className="ops-panel-head">
                    <h1>{titles[page]}</h1>
                  </div>
                  <Empty title="Create your first application">
                    <span>Pick a saved K3s connection, read its workloads and connect them as local applications.</span>
                  </Empty>
                  <div className="ops-empty-action">
                    <button className="ops-btn primary" onClick={openConnect}>
                      <PlugZap size={16} />
                      Connect cluster app
                    </button>
                  </div>
                </section>
              )}
            </>
          )
        )}
      </main>
      {inspector && (
        <ClusterInspector
          {...inspector}
          onClose={() => setInspector(undefined)}
        />
      )}
      {connect && snapshot && (
        <OpsConnectDialog
          snapshot={snapshot}
          connections={connections}
          onConnections={setConnections}
          onClose={() => setConnect(false)}
          onDone={(next, environmentName, appName) => {
            setSnapshot(next);
            setEnvironment(environmentName);
            setApplication(appName);
            setPage("deployments");
            setConnect(false);
            setNotice("K3s app connected. Credentials stay on this device.");
          }}
        />
      )}
    </div>
  );
}
function OpsScope({
  page,
  onPage,
  snapshot,
  onSnapshot,
  env,
  app,
  connection,
  connectionError,
  busy,
  onBusy,
  onNotice,
  onReload,
}: {
  page: Page;
  onPage: (p: Page) => void;
  snapshot: OpsSnapshot;
  onSnapshot: (s: OpsSnapshot) => void;
  env: OpsEnvironment;
  app: OpsApplication;
  connection?: Connection;
  connectionError: string;
  busy: boolean;
  onBusy: (b: boolean) => void;
  onNotice: (s: string) => void;
  onReload: () => void;
}) {
  const [cluster, setCluster] = useState<OpsCluster>();
  const clusterReads = useRef<
    Partial<
      Record<
        "workloads" | "full",
        {
          key: string;
          data?: OpsCluster;
          fetchedAt: number;
          pending?: Promise<OpsCluster>;
        }
      >
    >
  >({});
  const [clusterError, setClusterError] = useState("");
  const [loading, setLoading] = useState(false);
  const [version, setVersion] = useState(0);
  const [poll, setPoll] = useState(false);
  const [query, setQuery] = useState("");
  const [kind, setKind] = useState("");
  const [add, setAdd] = useState(false);
  const [release, setRelease] = useState<OpsArtifact>();
  const [record, setRecord] = useState<OpsRecord>();
  const [runId, setRunId] = useState<string>();
  const [pod, setPod] = useState("");
  const [detail, setDetail] = useState<Workload>();
  const [action, setAction] = useState<
    | {
        kind: "scale" | "restart";
        workload: Workload;
      }
    | {
        kind: "rollback";
        record: OpsRecord;
      }
  >();
  const needsCluster =
    ["deployments", "pods", "metrics", "logs", "events"].includes(page) ||
    !!release ||
    !!action;
  const clusterMode = ["pods", "metrics", "logs", "events"].includes(page)
    ? "full"
    : "workloads";
  // ListConnections returns new objects on refresh; only actual connection or
  // scope changes should invalidate an otherwise identical cluster read.
  const clusterKey = JSON.stringify([
    connection,
    connectionError,
    app.name,
    app.selector,
    env.namespace,
    env.name,
    snapshot.revision,
    version,
    null,
    clusterMode,
  ]);
  useEffect(() => {
    if (!needsCluster) {
      setLoading(false);
      return;
    }
    let stale = false;
    let timer: ReturnType<typeof setTimeout>;
    async function read(force = false) {
      setClusterError("");
      try {
        if (!connection) throw new Error(connectionError);
        if (clusterReads.current[clusterMode]?.key !== clusterKey) {
          clusterReads.current[clusterMode] = { key: clusterKey, fetchedAt: 0 };
        }
        const entry = clusterReads.current[clusterMode]!;
        setCluster(entry.data);
        if (!force && entry.data && Date.now() - entry.fetchedAt < 15000) {
          setCluster(entry.data);
          return;
        }
        setLoading(true);
        // Re-entering a page while its request is pending shares that request.
        if (!entry.pending) {
          entry.pending = (
            clusterMode === "full" ? api.ReadOpsCluster : api.ReadOpsWorkloads
          )(connection!.id, env.namespace, app.selector)
            .then((data) => {
              entry.data = data;
              entry.fetchedAt = Date.now();
              return data;
            })
            .catch((e) => {
              entry.data = undefined;
              throw e;
            })
            .finally(() => {
              entry.pending = undefined;
            });
        }
        const data = await entry.pending;
        if (!stale) setCluster(data);
      } catch (e) {
        if (!stale) {
          setClusterError(errorText(e));
          setCluster(undefined);
        }
      } finally {
        if (!stale) {
          setLoading(false);
          if (poll) timer = setTimeout(() => void read(true), 10000);
        }
      }
    }
    void read();
    return () => {
      stale = true;
      clearTimeout(timer);
    };
  }, [needsCluster, clusterKey, clusterMode, poll]);
  const artifacts = snapshot.artifacts.filter(
    (a) => a.application === app.name,
  );
  const externalReleases: OpsRecord[] = (snapshot.runs || []).flatMap(r => {
    if (r.kind !== "release" || !r.target || snapshot.releases.some(saved => saved.id === r.id)) return [];
    return [{ id: r.id, runId: r.id, origin: "external", action: "release", application: r.application,
      environment: r.target.environment, connection: r.connection || "", namespace: r.namespace || "",
      workload: r.target.workload, container: r.target.container, image: r.image || snapshot.artifacts.find(a => a.id === r.target!.buildId)?.image || "",
      beforeImage: "", version: r.version, status: r.status, startedAt: r.startedAt, finishedAt: r.finishedAt || "", message: r.message,
      steps: r.events.flatMap(e => e.event.step ? [`${e.event.step.status === "succeeded" ? "Done" : "Failed"} · ${e.event.step.name}${e.event.step.message ? `: ${e.event.step.message}` : ""}`] : []) }];
  });
  const releases = [...snapshot.releases, ...externalReleases].filter(
    r => r.application === app.name && r.environment === env.name,
  ).sort((a, b) => b.startedAt.localeCompare(a.startedAt));
  const match = (s: string) => s.toLowerCase().includes(query.toLowerCase());
  const workloads = (cluster?.workloads || []).filter(
    (w) =>
      (!kind || w.kind === kind) &&
      (match(w.name + " " + w.image) ||
        (page === "pods" &&
          cluster?.pods.some((p) => p.workload === w.name && match(p.name)))),
  );
  const pods = (cluster?.pods || []).filter(
    (p) => match(p.name) || match(p.workload),
  );
  const selectedPod =
    cluster?.pods.find((p) => p.name === pod) || cluster?.pods[0];
  function openLogs(p: OpsPod) {
    setPod(p.name);
    onPage("logs");
  }
  function refresh() {
    setVersion((v) => v + 1);
    onReload();
  }
  return (
    <section
      className={`ops-panel ${["deployments", "pods"].includes(page) ? "ops-resource-page" : ""}`}
    >
      <div className="ops-panel-head">
        <div>
          <h1>{titles[page]}</h1>
          {page === "artifacts" && <p>{artifacts.length} {artifacts.length === 1 ? "artifact" : "artifacts"} · all environments</p>}
          {page === "releases" && (
            <p>
              {releases.length} {releases.length === 1 ? "release record" : "release records"} · {env.name}
            </p>
          )}
        </div>
        <div className="ops-actions">
          {page === "artifacts" && (
            <button className="ops-btn primary" onClick={() => setAdd(true)}>
              <Plus size={16} />
              Register artifact
            </button>
          )}
          {page === "releases" && (
            <button
              className="ops-btn primary"
              disabled={!artifacts.length || busy}
              onClick={() => setRelease(artifacts[0])}
            >
              <Rocket size={16} />
              Release
            </button>
          )}
          <Refresher busy={loading || busy} onClick={refresh} />
        </div>
      </div>
      <div className="ops-toolbar">
        <span className="ops-muted">Current context</span>
        <span className="ops-tag">{env.connection}</span>
        <span>{env.namespace}</span>
        <span className="ops-toolbar-sep" />
        <span>{app.name}</span>
        {["deployments", "pods"].includes(page) && (
          <select
            aria-label="Workload type"
            value={kind}
            onChange={(e) => setKind(e.target.value)}
          >
            <option value="">All workload types</option>
            <option>Deployment</option>
            <option>StatefulSet</option>
            <option>DaemonSet</option>
          </select>
        )}
        <div className="ops-toolbar-spacer" />
        {["deployments", "pods"].includes(page) && (
          <Refresher busy={busy || loading} onClick={refresh} />
        )}{" "}
        {needsCluster && (
          <label className="ops-poll">
            <input
              type="checkbox"
              checked={poll}
              onChange={(e) => setPoll(e.target.checked)}
            />
            Refresh every 10s
          </label>
        )}
        {["artifacts", "releases", "deployments", "pods"].includes(page) && (
          <label className="ops-search">
            <Search size={14} />
            <input
              aria-label="Filter list"
              placeholder="Search name or version"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
          </label>
        )}
      </div>
      {needsCluster && (
        <>
          {loading && !cluster && (
            <div className="ops-loading" role="status">
              <LoaderCircle size={18} className="spin" /> Loading cluster workloads…
            </div>
          )}
          <ErrorNotice>{clusterError}</ErrorNotice>
          {cluster?.warnings.map((w) => (
            <ErrorNotice key={w}>{w}</ErrorNotice>
          ))}
          {cluster?.truncated && (
            <div className="ops-notice">
              Showing the first 500 resources only. Narrow the application selector.
            </div>
          )}
        </>
      )}
      {page === "runs" && (
        <OpsRuns
          runs={(snapshot.runs || []).filter(
            (r) =>
              r.application === app.name &&
              (r.kind === "build" || r.target?.environment === env.name),
          )}
          artifacts={artifacts}
          onRelease={setRelease}
          selectedId={runId}
          onSelected={setRunId}
        />
      )}
      {page === "artifacts" && (
        <>
          {artifacts.filter((a) =>
            match(a.version + " " + a.image + " " + a.source),
          ).length ? (
            <div className="ops-table-wrap">
              <table className="ops-table">
                <thead>
                  <tr>
                    <th>Version / source</th>
                    <th>Image</th>
                    <th>Registered</th>
                    <th className="ops-right">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {artifacts
                    .filter((a) =>
                      match(a.version + " " + a.image + " " + a.source),
                    )
                    .map((a) => (
                      <tr key={a.id}>
                        <td>
                          <div className="ops-title-cell">
                            <Package size={17} />
                            <strong>{a.version}</strong>
                            <span className="ops-badge">Image</span>
                          </div>
                          <small>{a.source || "Registered manually"}</small>
                          {a.runId && (
                            <button
                              className="ops-link"
                              onClick={() => {
                                setRunId(a.runId);
                                onPage("runs");
                              }}
                            >
                              View build record
                            </button>
                          )}
                        </td>
                        <td>
                          <span className="ops-image-name">
                            {a.image.split("@")[0]}
                          </span>
                          <code className="ops-digest" title={a.image}>
                            sha256:{a.image.split("sha256:")[1]?.slice(0, 16)}…
                          </code>
                        </td>
                        <td className="ops-muted">{date(a.createdAt)}</td>
                        <td className="ops-right">
                          <button
                            className="ops-link"
                            onClick={() => setRelease(a)}
                            disabled={busy}
                          >
                            <Rocket size={14} />
                            Release
                          </button>
                        </td>
                      </tr>
                    ))}
                </tbody>
              </table>
            </div>
          ) : (
            <Empty
              title={artifacts.length ? "No matching artifacts" : "No artifacts for this application yet"}
            >
              Register an image that is already in your registry to create a release plan.
            </Empty>
          )}
          <p className="ops-footnote">
            Artifacts store the image address and immutable digest. Image content stays in your OCI registry.
          </p>
        </>
      )}
      {page === "releases" && (
        <>
          {releases.filter((r) => match(r.version + " " + r.workload))
            .length ? (
            <div className="ops-release-list">
              {releases
                .filter((r) => match(r.version + " " + r.workload))
                .map((r) => (
                  <button
                    className="ops-release-row"
                    key={r.id}
                    onClick={() => setRecord(r)}
                  >
                    <div className="ops-release-main">
                      <strong>
                        <span className="ops-muted">#{r.id.slice(0, 8)}</span>{" "}
                        {r.workload}{" "}
                        <span className="ops-version">
                          {operationName(r)} {r.version}
                        </span>
                      </strong>
                      <small>
                        {r.environment}
                        <span>
                          {r.action === "scale" || r.action === "restart"
                            ? `Replicas ${r.beforeReplicas} → ${r.replicas}`
                            : r.action === "config"
                              ? r.replicas != null
                                ? `ConfigMap · restarted ${r.workload}`
                                : "ConfigMap"
                              : r.image.split("@")[0]}
                        </span>
                        <Boxes size={13} />
                        {r.namespace}
                      </small>
                    </div>
                    <Status
                      ready={
                        r.status === "succeeded"
                          ? 3
                          : r.status === "running"
                            ? 1
                            : 0
                      }
                      total={3}
                    />
                    <span className={`ops-status ${r.status}`}>
                      {operationStatus(r)}
                    </span>
                    <span className="ops-release-time">
                      {r.origin === "external" ? "External release" : "Local client"}<small>{date(r.startedAt)}</small>
                    </span>
                    <ChevronRight size={16} />
                  </button>
                ))}
            </div>
          ) : (
            <Empty title="No release records in this context">
              Pick a version in Artifacts, review the release plan, then run it.
            </Empty>
          )}
        </>
      )}
      {page === "deployments" && (
        <div className="ops-table-pad">
          <div className="ops-table-wrap bordered">
            <table className="ops-table compact">
              <thead>
                <tr>
                  <th>Kind</th>
                  <th>workloadName</th>
                  <th>Namespace</th>
                  <th>Cluster · replicas</th>
                  <th className="ops-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {workloads.map((w) => (
                  <tr key={w.uid}>
                    <td>{w.kind}</td>
                    <td>
                      <button
                        className="ops-link neutral"
                        title={w.image}
                        onClick={() => setDetail(w)}
                      >
                        {w.name}
                      </button>
                    </td>
                    <td>{w.namespace}</td>
                    <td>
                      <div className="ops-inline">
                        <span className="ops-tag">{env.connection}</span>
                        <Status ready={w.ready} total={w.desired} />
                      </div>
                    </td>
                    <td className="ops-right">
                      <button
                        className="ops-link"
                        onClick={() => {
                          setQuery(w.name);
                          onPage("pods");
                        }}
                      >
                        View Pods
                      </button>
                      {w.kind === "Deployment" && (
                        <>
                          <button
                            className="ops-link"
                            disabled={busy}
                            onClick={() =>
                              setAction({ kind: "scale", workload: w })
                            }
                          >
                            Scale
                          </button>
                          <button
                            className="ops-link"
                            disabled={busy}
                            onClick={() =>
                              setAction({ kind: "restart", workload: w })
                            }
                          >
                            Restart
                          </button>
                        </>
                      )}
                      <button className="ops-link" onClick={() => setDetail(w)}>
                        Details
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            {!workloads.length && !loading && (
              <Empty title="No matching workloads" />
            )}
          </div>
        </div>
      )}
      {page === "pods" && (
        <div className="ops-pod-groups">
          {workloads.map((w) => (
            <details open className="ops-pod-group" key={w.uid}>
              <summary>
                <ChevronDown size={14} />
                <strong>{w.name}</strong>
                <span>{w.kind}</span>
                <span>{env.connection}</span>
                <Status ready={w.ready} total={w.desired} />
              </summary>
              <div className="ops-pod-group-body">
                <span className="ops-tab-active">Pods</span>
                <PodTable
                  pods={pods.filter((p) => p.workload === w.name)}
                  onLogs={openLogs}
                />
              </div>
            </details>
          ))}
          {pods.some((p) => !p.workload) && (
            <div className="ops-pod-group-body">
              <h3>Other Pods</h3>
              <PodTable
                pods={pods.filter((p) => !p.workload)}
                onLogs={openLogs}
              />
            </div>
          )}
          {!pods.length && !loading && <Empty title="No Pods for this application" />}
        </div>
      )}
      {page === "metrics" && (
        <Metrics
          key={`${connection?.id}:${app.name}:${env.name}`}
          connection={connection}
          env={env}
          app={app}
          cluster={cluster}
          version={version}
          poll={poll}
        />
      )}
      {(page === "logs" || page === "events") && (
        <div className="ops-observe">
          <div className="ops-observe-toolbar">
            <label>
              Pod{" "}
              <select
                aria-label="Select Pod"
                value={selectedPod?.name || ""}
                onChange={(e) => setPod(e.target.value)}
              >
                {cluster?.pods.map((p) => (
                  <option key={p.uid}>{p.name}</option>
                ))}
                {!cluster?.pods.length && <option value="">No Pods</option>}
              </select>
            </label>
            {selectedPod && (
              <span className="ops-tag">{selectedPod.phase}</span>
            )}
          </div>
          {selectedPod ? (
            <PodObservation
              key={`${selectedPod.uid}:${page}`}
              page={page}
              pod={selectedPod}
              connection={connection}
              poll={poll}
              version={version}
            />
          ) : (
            <Empty
              title={
                loading
                  ? "Loading Pods…"
                  : "Select a Pod to view its " + (page === "logs" ? "logs" : "events")
              }
            />
          )}
        </div>
      )}
      {needsCluster && cluster && (
        <p className="ops-footnote">
          Last read: {date(cluster.fetchedAt)} · {cluster.workloads.length}{" "}
          {cluster.workloads.length === 1 ? "workload" : "workloads"}
          {clusterMode === "full" ? ` · ${cluster.pods.length} ${cluster.pods.length === 1 ? "Pod" : "Pods"}` : ""}
        </p>
      )}
      {add && (
        <ArtifactDialog
          app={app.name}
          snapshot={snapshot}
          onSnapshot={onSnapshot}
          onClose={() => setAdd(false)}
        />
      )}
      {release && (
        <ReleaseDialog
          artifact={release}
          snapshot={snapshot}
          env={env}
          app={app}
          workloads={cluster?.workloads || []}
          connection={connection}
          onSnapshot={onSnapshot}
          onBusy={onBusy}
          onClose={() => setRelease(undefined)}
          onDone={(r) => {
            setRecord(r);
            setRelease(undefined);
            onPage("releases");
            setVersion((v) => v + 1);
          }}
        />
      )}
      {record && (
        <Modal
          title={`${operationName(record)} ${record.version}`}
          subtitle={`${record.workload} · ${record.environment} / ${record.namespace}`}
          onClose={() => setRecord(undefined)}
        >
          <div className="ops-dialog-body">
            <span className={`ops-status ${record.status}`}>
              {operationStatus(record)}
            </span>
            <p>
              {record.message ||
                "If the client was interrupted, refresh the cluster to check the actual state. Do not just retry."}
            </p>
            {record.action === "nginx" ? <p>Host Nginx config test and reload record</p> : record.action === "config" ? (
              <p>
                ConfigMap key {record.version}
                {record.replicas != null
                  ? ` · Deployment ${record.workload} restarted`
                  : ""}
                . Contents are not recorded; see the steps for line counts and hashes.
              </p>
            ) : record.action === "scale" || record.action === "restart" ? (
              <p>
                Replicas: {record.beforeReplicas} → {record.replicas}
                {record.action === "restart"
                  ? " · Pods recreated by the Deployment strategy"
                  : ""}
              </p>
            ) : (
              <div className="ops-release-images">
                <label>
                  Previous image<code>{record.beforeImage || "Not recorded"}</code>
                </label>
                <label>
                  Target image<code>{record.image}</code>
                </label>
              </div>
            )}
            {record.runId && (
              <button
                className="ops-link"
                onClick={() => {
                  setRunId(record.runId);
                  setRecord(undefined);
                  onPage("runs");
                }}
              >
                {record.origin === "external" ? "View release record" : "View build record"}
              </button>
            )}
            <ol className="ops-steps">
              {record.steps.map((s, i) => (
                <li key={i}>
                  <Check size={15} />
                  {s}
                </li>
              ))}
            </ol>
            <p className="ops-muted">
              {date(record.startedAt)} → {date(record.finishedAt)}
            </p>
          </div>
          {record.status === "succeeded" && record.image && record.origin !== "external" && (
            <div className="ops-dialog-footer">
              <button
                className="ops-btn"
                onClick={() => {
                  setAction({ kind: "rollback", record });
                  setRecord(undefined);
                }}
              >
                Roll back to this version
              </button>
            </div>
          )}
        </Modal>
      )}
      {action && (
        <WorkloadActionDialog
          action={action}
          snapshot={snapshot}
          env={env}
          app={app}
          cluster={cluster}
          onClose={() => setAction(undefined)}
          onBusy={onBusy}
          onSnapshot={onSnapshot}
          onDone={(r) => {
            setAction(undefined);
            setRecord(r);
            setVersion((v) => v + 1);
          }}
        />
      )}
      {detail && (
        <WorkloadDetail
          workload={detail}
          connection={connection}
          onClose={() => setDetail(undefined)}
        />
      )}
    </section>
  );
}
function PodTable({
  pods,
  onLogs,
}: {
  pods: OpsPod[];
  onLogs: (p: OpsPod) => void;
}) {
  return (
    <div className="ops-table-wrap bordered">
      <table className="ops-table ops-pods-table">
        <thead>
          <tr>
            <th>Name</th>
            <th>Pod IP</th>
            <th>Node IP</th>
            <th>Restarts</th>
            <th>Age</th>
            <th>Ports</th>
            <th>Status</th>
            <th className="ops-right">Actions</th>
          </tr>
        </thead>
        <tbody>
          {pods.map((p) => (
            <tr key={p.uid}>
              <td>
                <button className="ops-link neutral" onClick={() => onLogs(p)}>
                  {p.name}
                </button>
                <small>{p.node}</small>
              </td>
              <td>
                <code>{p.ip || "—"}</code>
              </td>
              <td>
                <code>{p.nodeIP || "—"}</code>
              </td>
              <td>
                <span className={`ops-tag ${p.restarts ? "amber" : "green"}`}>
                  {p.restarts}
                </span>
              </td>
              <td>{age(p.createdAt)}</td>
              <td>
                <div className="ops-port-list">
                  {p.ports.map((port, i) => (
                    <span className="ops-tag" key={i}>
                      {port}
                    </span>
                  ))}
                </div>
              </td>
              <td>
                <span
                  className={
                    p.phase === "Running" ? "ops-running" : "ops-muted"
                  }
                >
                  {p.phase}
                </span>
                <small>
                  {p.ready} / {p.total} ready
                </small>
              </td>
              <td className="ops-right">
                <button
                  className="ops-icon"
                  aria-label={`View ${p.name} logs`}
                  title="View logs"
                  onClick={() => onLogs(p)}
                >
                  <FileText size={16} />
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {!pods.length && (
        <div className="ops-inline-empty">No Pods match this workload</div>
      )}
    </div>
  );
}
function Metrics({
  connection,
  env,
  app,
  cluster,
  version,
  poll,
}: {
  connection?: Connection;
  env: OpsEnvironment;
  app: OpsApplication;
  cluster?: OpsCluster;
  version: number;
  poll: boolean;
}) {
  const [metrics, setMetrics] = useState<OpsMetric[]>([]);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  useEffect(() => {
    let stale = false;
    let timer: ReturnType<typeof setTimeout>;
    async function read() {
      setLoading(true);
      setError("");
      try {
        if (!connection) throw new Error("Configure a cluster connection first");
        const data = await api.ReadOpsMetrics(
          connection!.id,
          env.namespace,
          app.selector,
        );
        if (!stale) setMetrics(data);
      } catch (e) {
        if (!stale) {
          setError(errorText(e));
          setMetrics([]);
        }
      } finally {
        if (!stale) {
          setLoading(false);
          if (poll) timer = setTimeout(() => void read(), 10000);
        }
      }
    }
    void read();
    return () => {
      stale = true;
      clearTimeout(timer);
    };
  }, [
    connection,
    env.namespace,
    app.selector,
    cluster?.fetchedAt,
    version,
    poll,
  ]);
  return (
    <div className="ops-metrics">
      <ErrorNotice>{error}</ErrorNotice>
      <div className="ops-metric-summary">
        <div>
          <span>CPU usage</span>
          <strong>
            {metrics.length
              ? metrics.reduce((sum, m) => sum + m.cpu, 0).toFixed(0)
              : "—"}
            <small>mCPU</small>
          </strong>
        </div>
        <div>
          <span>Memory usage</span>
          <strong>
            {metrics.length
              ? metrics.reduce((sum, m) => sum + m.memory, 0).toFixed(1)
              : "—"}
            <small>MiB</small>
          </strong>
        </div>
        <div>
          <span>Pods ready</span>
          <strong>
            {cluster?.pods.filter((p) => p.ready === p.total && p.total > 0)
              .length ?? "—"}
            <small>/ {cluster?.pods.length ?? "—"}</small>
          </strong>
        </div>
      </div>
      <div className="ops-table-wrap bordered">
        <table className="ops-table">
          <thead>
            <tr>
              <th>Pod</th>
              <th>CPU (mCPU)</th>
              <th>Memory (MiB)</th>
              <th>Sampled at</th>
            </tr>
          </thead>
          <tbody>
            {metrics.map((m) => (
              <tr key={m.pod}>
                <td>{m.pod}</td>
                <td>{m.cpu.toFixed(0)}</td>
                <td>{m.memory.toFixed(1)}</td>
                <td className="ops-muted">{date(m.timestamp)}</td>
              </tr>
            ))}
          </tbody>
        </table>
        {!metrics.length && !loading && (
          <Empty title="No metrics available">
            The cluster needs metrics-server installed and read access granted.
          </Empty>
        )}
      </div>
      <p className="ops-footnote">
        Current usage snapshot · 1,000 mCPU = 1 core · no history is stored
      </p>
    </div>
  );
}
function PodObservation({
  page,
  pod,
  connection,
  poll,
  version,
}: {
  page: "logs" | "events";
  pod: OpsPod;
  connection?: Connection;
  poll: boolean;
  version: number;
}) {
  const [container, setContainer] = useState(pod.containers[0]?.name || "");
  const [tail, setTail] = useState(200);
  const [previous, setPrevious] = useState(false);
  const [text, setText] = useState("");
  const [events, setEvents] = useState<QueryResult>();
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [truncated, setTruncated] = useState(false);
  const [search, setSearch] = useState("");
  useEffect(() => {
    let stale = false;
    let timer: ReturnType<typeof setTimeout>;
    async function read() {
      setLoading(true);
      setError("");
      setText("");
      setEvents(undefined);
      try {
        const ref = {
          namespace: pod.namespace,
          context: connection?.context || "",
          name: pod.name,
          uid: pod.uid,
          resource: "pods",
        };
        if (page === "logs") {
          if (!container) throw new Error("This Pod has no container with readable logs");
          const r = await api.ReadPodLogs(connection!.id, {
            ref,
            container,
            tailLines: tail,
            previous,
          });
          if (!stale) {
            setText(r.text);
            setTruncated(r.truncated);
          }
        } else {
          const r = await api.ClusterEvents(connection!.id, ref);
          if (!stale) setEvents(r);
        }
      } catch (e) {
        if (!stale) setError(errorText(e));
      } finally {
        if (!stale) {
          setLoading(false);
          if (poll) timer = setTimeout(() => void read(), 10000);
        }
      }
    }
    void read();
    return () => {
      stale = true;
      clearTimeout(timer);
    };
  }, [page, pod, connection, container, tail, previous, poll, version]);
  const visible = text
    .split("\n")
    .filter((line) => line.toLowerCase().includes(search.toLowerCase()))
    .join("\n");
  return (
    <>
      <ErrorNotice>{error}</ErrorNotice>
      {page === "logs" ? (
        <>
          <div className="ops-log-controls">
            <label>
              Container{" "}
              <select
                aria-label="Log container"
                value={container}
                onChange={(e) => setContainer(e.target.value)}
              >
                {pod.containers.map((c) => (
                  <option key={c.name} value={c.name}>
                    {c.name} · {c.kind}
                  </option>
                ))}
              </select>
            </label>
            <label>
              Lines{" "}
              <select
                aria-label="Log lines"
                value={tail}
                onChange={(e) => setTail(Number(e.target.value))}
              >
                {[100, 200, 500, 1000, 2000].map((n) => (
                  <option key={n}>{n}</option>
                ))}
              </select>
            </label>
            <label>
              <input
                type="checkbox"
                checked={previous}
                onChange={(e) => setPrevious(e.target.checked)}
              />
              Previous instance
            </label>
            <label className="ops-search">
              <Search size={14} />
              <input
                aria-label="Filter logs"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder="Filter current logs"
              />
            </label>
          </div>
          <pre className="ops-log-output" aria-label="Log output">
            {loading
              ? "Loading logs…"
              : visible || (!error ? "No log output" : "Read failed")}
          </pre>
          {truncated && (
            <p className="ops-notice">
              Logs hit the 256 KiB limit. Read fewer lines.
            </p>
          )}
          <p className="ops-footnote">
            Log snapshot · {poll ? "re-read every 10s" : "manual refresh"} ·
            log text is not written to the shared project
          </p>
        </>
      ) : (
        <div className="ops-table-wrap bordered">
          <table className="ops-table">
            <thead>
              <tr>
                {events?.columns.map((c) => (
                  <th key={c}>{c}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {events?.rows.map((r, i) => (
                <tr key={i}>
                  {r.map((v, j) => (
                    <td key={j}>{String(v ?? "—")}</td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
          {!loading && !events?.rows.length && (
            <Empty title="No retained events for this Pod" />
          )}
          {events?.truncated && (
            <p className="ops-notice">The event list hit the preview limit.</p>
          )}
        </div>
      )}
    </>
  );
}
function ArtifactDialog({
  app,
  snapshot,
  onSnapshot,
  onClose,
}: {
  app: string;
  snapshot: OpsSnapshot;
  onSnapshot: (s: OpsSnapshot) => void;
  onClose: () => void;
}) {
  const [version, setVersion] = useState("");
  const [image, setImage] = useState("");
  const [source, setSource] = useState("");
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  async function save(e: FormEvent) {
    e.preventDefault();
    setError("");
    if (
      !/^[a-zA-Z0-9][a-zA-Z0-9._:/-]*@sha256:[a-f0-9]{64}$/.test(image) ||
      image.includes(":latest@")
    ) {
      setError("The image must include a full sha256 digest; latest is not allowed");
      return;
    }
    setSaving(true);
    try {
      const artifact: OpsArtifact = {
        id: crypto.randomUUID(),
        application: app,
        version: version.trim(),
        image,
        source,
        createdAt: new Date().toISOString(),
      };
      const next = await api.AddOpsArtifact(artifact);
      onSnapshot(next);
      onClose();
    } catch (e) {
      setError(errorText(e));
    } finally {
      setSaving(false);
    }
  }
  return (
    <Modal
      title="Register artifact"
      subtitle={`${app} · existing OCI image`}
      onClose={() => {
        if (!saving) onClose();
      }}
    >
      <form onSubmit={save}>
        <div className="ops-dialog-body ops-form">
          <ErrorNotice>{error}</ErrorNotice>
          <label>
            Version
            <input
              required
              maxLength={120}
              autoFocus
              placeholder="e.g. 1.4.3"
              value={version}
              onChange={(e) => setVersion(e.target.value)}
            />
          </label>
          <label>
            Image
            <textarea
              aria-label="Image"
              required
              rows={3}
              maxLength={1024}
              placeholder="registry.local:5000/app@sha256:…"
              value={image}
              onChange={(e) => setImage(e.target.value.trim())}
            />
          </label>
          <label>
            Source / notes
            <input
              maxLength={500}
              placeholder="Commit message or build number"
              value={source}
              onChange={(e) => setSource(e.target.value)}
            />
          </label>
          <p className="ops-muted">
            Registering does not build or push an image. Enter the digest of an image that exists and the cluster can pull.
          </p>
        </div>
        <div className="ops-dialog-footer">
          <button
            type="button"
            className="ops-btn"
            disabled={saving}
            onClick={onClose}
          >
            Cancel
          </button>
          <button className="ops-btn primary" disabled={saving}>
            {saving ? (
              <LoaderCircle size={15} className="spin" />
            ) : (
              <Plus size={15} />
            )}
            Save artifact
          </button>
        </div>
      </form>
    </Modal>
  );
}
function ReleaseDialog({
  artifact,
  snapshot,
  env,
  app,
  workloads,
  connection,
  onSnapshot,
  onBusy,
  onClose,
  onDone,
}: {
  artifact: OpsArtifact;
  snapshot: OpsSnapshot;
  env: OpsEnvironment;
  app: OpsApplication;
  workloads: Workload[];
  connection?: Connection;
  onSnapshot: (s: OpsSnapshot) => void;
  onBusy: (b: boolean) => void;
  onClose: () => void;
  onDone: (r: OpsRecord) => void;
}) {
  const [target, setTarget] = useState("");
  const deployments = workloads.filter((w) => w.kind === "Deployment");
  const workload = deployments.find((w) => w.name === target) || deployments[0];
  const [containers, setContainers] = useState<string[]>([]);
  const [container, setContainer] = useState("");
  const [plan, setPlan] = useState<OpsPlan>();
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [detailsLoading, setDetailsLoading] = useState(false);
  useEffect(() => {
    let stale = false;
    setPlan(undefined);
    setContainers([]);
    setContainer("");
    if (!workload) return;
    setDetailsLoading(true);
    api
      .InspectClusterResource(connection!.id, {
        resource: "deployments",
        name: workload.name,
        namespace: env.namespace,
        uid: workload.uid,
        context: connection!.context,
      })
      .then((d) => {
        const obj = JSON.parse(d.definition);
        return (obj.spec?.template?.spec?.containers || []).map(
          (c: { name: string }) => c.name,
        ) as string[];
      })
      .then((names) => {
        if (!stale) {
          setContainers(names);
          setContainer(names[0] || "");
        }
      })
      .catch((e) => {
        if (!stale) setError(errorText(e));
      })
      .finally(() => {
        if (!stale) setDetailsLoading(false);
      });
    return () => {
      stale = true;
    };
  }, [workload?.uid, connection, env.namespace]);
  async function prepare(e: FormEvent) {
    e.preventDefault();
    if (!workload || !container) return;
    setBusy(true);
    setError("");
    try {
      if (!connection) throw new Error("Configure a cluster connection first");
      const p = await api.PlanOpsRelease(
        env.name,
        app.name,
        workload.name,
        container,
        artifact.id,
      );
      setPlan(p);
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  }
  async function apply() {
    if (!plan) return;
    setBusy(true);
    onBusy(true);
    setError("");
    try {
      let r: OpsRecord;
      {
        r = await api.ApplyOpsRelease(plan.id);
        try {
          onSnapshot(await api.GetOpsProject());
        } catch {
          onSnapshot({ ...snapshot, releases: [r, ...snapshot.releases] });
        }
      }
      onDone(r);
    } catch (e) {
      setError(errorText(e));
      setPlan(undefined);
    } finally {
      setBusy(false);
      onBusy(false);
    }
  }
  return (
    <Modal
      title={plan ? "Review release plan" : "Release artifact"}
      subtitle={`${artifact.version} → ${env.name} / ${env.namespace}`}
      onClose={() => {
        if (!busy) onClose();
      }}
    >
      <form onSubmit={prepare}>
        <div className="ops-dialog-body ops-form">
          <ErrorNotice>{error}</ErrorNotice>

          <label>
            Deployment
            <select
              aria-label="Release target"
              value={workload?.name || ""}
              disabled={busy || !!plan}
              onChange={(e) => setTarget(e.target.value)}
            >
              {deployments.map((w) => (
                <option key={w.uid}>{w.name}</option>
              ))}
              {!deployments.length && (
                <option value="">No Deployments available</option>
              )}
            </select>
          </label>
          <label>
            Container
            <select
              aria-label="Release container"
              value={container}
              disabled={busy || !!plan || detailsLoading}
              onChange={(e) => setContainer(e.target.value)}
            >
              {containers.map((c) => (
                <option key={c}>{c}</option>
              ))}
              {!containers.length && (
                <option value="">
                  {detailsLoading ? "Loading containers…" : "No containers"}
                </option>
              )}
            </select>
          </label>
          <div className="ops-release-images">
            {plan && (
              <label>
                Current image<code>{plan.beforeImage}</code>
              </label>
            )}
            <label>
              Target image<code>{artifact.image}</code>
            </label>
          </div>
          {plan ? (
            <p className="ops-muted">
              Dry-run passed. On confirm the image is updated and DevOpsPad waits up to 180
              seconds for the rollout. Failures are not rolled back automatically. Plan valid until {date(plan.expiresAt)}.
            </p>
          ) : (
            <p className="ops-muted">
              DevOpsPad reads the target state and runs a server-side dry-run before you confirm.
            </p>
          )}
          {busy && plan && (
            <p role="status">
              <LoaderCircle size={16} className="spin" />
              Releasing and waiting for the rollout…
            </p>
          )}
        </div>
        <div className="ops-dialog-footer">
          <button
            type="button"
            className="ops-btn"
            disabled={busy}
            onClick={() => (plan ? setPlan(undefined) : onClose())}
          >
            {plan ? "Back" : "Cancel"}
          </button>
          {plan ? (
            <button
              type="button"
              className="ops-btn primary"
              disabled={busy}
              onClick={() => void apply()}
            >
              <Rocket size={15} />
              {"Confirm release"}
            </button>
          ) : (
            <button
              className="ops-btn primary"
              disabled={busy || detailsLoading || !workload || !container}
            >
              Create release plan
            </button>
          )}
        </div>
      </form>
    </Modal>
  );
}
function WorkloadActionDialog({
  action,
  snapshot,
  env,
  app,
  cluster,
  onClose,
  onBusy,
  onSnapshot,
  onDone,
}: {
  action:
    | {
        kind: "scale" | "restart";
        workload: Workload;
      }
    | {
        kind: "rollback";
        record: OpsRecord;
      };
  snapshot: OpsSnapshot;
  env: OpsEnvironment;
  app: OpsApplication;
  cluster?: OpsCluster;
  onClose: () => void;
  onBusy: (b: boolean) => void;
  onSnapshot: (s: OpsSnapshot) => void;
  onDone: (r: OpsRecord) => void;
}) {
  const name =
    action.kind === "rollback" ? action.record.workload : action.workload.name;
  const current = cluster?.workloads.find((w) => w.name === name);
  const title = operationName({ action: action.kind });
  const [replicas, setReplicas] = useState(
    action.kind === "scale" ? String(action.workload.desired) : "0",
  );
  const [plan, setPlan] = useState<OpsPlan>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function prepare(e: FormEvent) {
    e.preventDefault();
    setError("");
    setBusy(true);
    onBusy(true);
    try {
      if (
        action.kind === "scale" &&
        (!/^\d+$/.test(replicas) || Number(replicas) > 1000)
      )
        throw new Error("Replicas must be a whole number from 0 to 1000");
      let next: OpsPlan;
      {
        next =
          action.kind === "rollback"
            ? await api.PlanOpsRollback(env.name, app.name, action.record.id)
            : await api.PlanOpsWorkloadAction(
                env.name,
                app.name,
                name,
                action.kind,
                Number(replicas),
              );
      }
      setPlan(next);
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
      onBusy(false);
    }
  }
  async function apply() {
    if (!plan) return;
    setBusy(true);
    onBusy(true);
    setError("");
    try {
      let result: OpsRecord;
      {
        result = await api.ApplyOpsRelease(plan.id);
        // The operation result remains visible even if reloading the local index fails.
        try {
          onSnapshot(await api.GetOpsProject());
        } catch {
          onSnapshot({ ...snapshot, releases: [result, ...snapshot.releases] });
        }
      }
      onDone(result);
    } catch (e) {
      setError(errorText(e));
      setPlan(undefined);
    } finally {
      setBusy(false);
      onBusy(false);
    }
  }
  return (
    <Modal
      title={plan ? `Review ${title.toLowerCase()} plan` : title}
      subtitle={`${name} · ${env.connection} / ${env.namespace}`}
      onClose={() => {
        if (!busy) onClose();
      }}
    >
      <form onSubmit={prepare}>
        <div className="ops-dialog-body ops-form">
          <ErrorNotice>{error}</ErrorNotice>

          {action.kind === "scale" && (
            <>
              <label>
                Target replicas
                <input
                  aria-label="Target replicas"
                  type="number"
                  min={0}
                  max={1000}
                  step={1}
                  required
                  value={replicas}
                  disabled={busy || !!plan}
                  onChange={(e) => setReplicas(e.target.value)}
                  autoFocus
                />
              </label>
              <p className="ops-muted">
                {plan
                  ? `Replicas: ${plan.beforeReplicas} → ${plan.replicas}`
                  : `Current replicas: ${current?.desired ?? action.workload.desired}`}
                . If an HPA manages this workload, it may change the replica count again.
              </p>
              {Number(replicas) === 0 && (
                <div className="ops-notice">
                  Setting 0 stops every Pod of this Deployment.
                </div>
              )}
            </>
          )}
          {action.kind === "restart" && (
            <p>
              Updates a Pod template annotation so the Deployment recreates its
              Pods with its update strategy. Images and replicas are kept.
            </p>
          )}
          {action.kind === "rollback" && (
            <div className="ops-release-images">
              <label>
                Version
                <code>
                  {action.record.version} · {action.record.container}
                </code>
              </label>
              {plan && (
                <label>
                  Current image<code>{plan.beforeImage}</code>
                </label>
              )}
              <label>
                Target image<code>{action.record.image}</code>
              </label>
              <p className="ops-muted">
                Rolls back this container image only. Other settings and containers are kept.
              </p>
            </div>
          )}
          <p className="ops-muted">
            {plan
              ? `Dry-run passed. DevOpsPad waits up to 180 seconds for the rollout. Plan valid until ${date(plan.expiresAt)}.`
              : "Creating a plan reads the latest state and runs a server-side dry-run."}
          </p>
          {busy && (
            <p role="status">
              <LoaderCircle size={15} className="spin" />
              {plan ? "Running and waiting for the rollout…" : "Creating plan…"}
            </p>
          )}
        </div>
        <div className="ops-dialog-footer">
          <button
            type="button"
            className="ops-btn"
            disabled={busy}
            onClick={() => (plan ? setPlan(undefined) : onClose())}
          >
            {plan ? "Back" : "Cancel"}
          </button>
          {plan ? (
            <button
              type="button"
              className="ops-btn primary"
              disabled={busy}
              onClick={() => void apply()}
            >
              {`Confirm ${title.toLowerCase()}`}
            </button>
          ) : (
            <button className="ops-btn primary" disabled={busy}>
              Create plan
            </button>
          )}
        </div>
      </form>
    </Modal>
  );
}
function WorkloadDetail({
  workload,
  connection,
  onClose,
}: {
  workload: Workload;
  connection?: Connection;
  onClose: () => void;
}) {
  const [text, setText] = useState("");
  const [error, setError] = useState("");
  useEffect(() => {
    let stale = false;
    api
      .InspectClusterResource(connection!.id, {
        resource: workload.resource,
        name: workload.name,
        namespace: workload.namespace,
        uid: workload.uid,
        context: connection!.context,
      })
      .then((d) => d.definition)
      .then((t) => {
        if (!stale) setText(t);
      })
      .catch((e) => {
        if (!stale) setError(errorText(e));
      });
    return () => {
      stale = true;
    };
  }, [workload, connection]);
  return (
    <Modal
      title={workload.name}
      subtitle={`${workload.kind} · ${workload.namespace} · read-only definition`}
      wide
      onClose={onClose}
    >
      <div className="ops-dialog-body">
        <ErrorNotice>{error}</ErrorNotice>
        <pre className="ops-definition">
          {text || (!error ? "Loading…" : "")}
        </pre>
      </div>
    </Modal>
  );
}
function ProjectSettings({
  page,
  snapshot,
  busy,
  onSnapshot,
  onNotice,
  onBusy,
}: {
  page: "project" | "sharing";
  snapshot: OpsSnapshot;
  busy: boolean;
  onSnapshot: (s: OpsSnapshot) => void;
  onNotice: (s: string) => void;
  onBusy: (b: boolean) => void;
}) {
  const [text, setText] = useState(snapshot.yaml);
  const [base, setBase] = useState(snapshot.revision);
  const [error, setError] = useState("");
  const [git, setGit] = useState("");
  async function save() {
    onBusy(true);
    setError("");
    try {
      const next = await api.SaveOpsProject(text, base);
      onSnapshot(next);
      setBase(next.revision);
      onNotice("Project mapping saved");
    } catch (e) {
      setError(errorText(e));
    } finally {
      onBusy(false);
    }
  }
  async function load() {
    onBusy(true);
    setError("");
    try {
      const next = await api.GetOpsProject();
      onSnapshot(next);
      setText(next.yaml);
      setBase(next.revision);
    } catch (e) {
      setError(errorText(e));
    } finally {
      onBusy(false);
    }
  }
  async function choose() {
    onBusy(true);
    setError("");
    try {
      onSnapshot(await api.SelectOpsProject());
    } catch (e) {
      setError(errorText(e));
    } finally {
      onBusy(false);
    }
  }
  async function gitAction(action: string) {
    onBusy(true);
    setError("");
    try {
      setGit(await api.OpsGit(action));
      if (action === "pull") {
        const s = await api.GetOpsProject();
        onSnapshot(s);
        setText(s.yaml);
        setBase(s.revision);
      }
    } catch (e) {
      setError(errorText(e));
    } finally {
      onBusy(false);
    }
  }
  function download() {
    const url = URL.createObjectURL(
      new Blob([text], { type: "application/yaml" }),
    );
    const a = document.createElement("a");
    a.href = url;
    a.download = "project.yaml";
    a.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }
  return (
    <section className="ops-panel">
      <div className="ops-panel-head">
        <div>
          <h1>{titles[page]}</h1>
          <p>{snapshot.project.name}</p>
        </div>
        <div className="ops-actions">
          {page === "project" && (
            <>
              <button
                className="ops-btn"
                disabled={busy}
                onClick={() => {
                  try {
                    const doc = parseDocument(text);
                    if (doc.errors.length) throw doc.errors[0];
                    setText(doc.toString({ collectionStyle: "block" }));
                    setError("");
                  } catch (e) {
                    setError(errorText(e));
                  }
                }}
              >
                Format YAML
              </button>
              <button className="ops-btn" onClick={download}>
                <ArrowDownToLine size={15} />
                Export YAML
              </button>
              <button
                className="ops-btn primary"
                disabled={busy}
                onClick={() => void save()}
              >
                <Check size={15} />
                Save config
              </button>
            </>
          )}
          <Refresher busy={busy} onClick={() => void load()} />
        </div>
      </div>
      <ErrorNotice>{error}</ErrorNotice>
      {page === "project" ? (
        <div className="ops-config">
          <div className="ops-editor-title">
            <FileCode2 size={16} />
            <span>project.yaml</span>
            <span className="ops-muted">
              {text !== snapshot.yaml ? "Unsaved changes" : "Saved"}
            </span>
          </div>
          <textarea
            className="ops-yaml"
            aria-label="Project YAML"
            spellCheck
            value={text}
            onChange={(e) => setText(e.target.value)}
            disabled={busy}
          />
          <div className="ops-config-help">
            <strong>Project and application mapping</strong>
            <p>
              Edit the environments and application selectors for the whole project. To see an application’s ConfigMaps
              and environment variables, open App config.
            </p>
            <p>
              Set connection to the name of a Kubernetes / K3s connection in Cluster
              resources. Everyone can use their own kubeconfig; applications match workloads and Pods
              by selector.
            </p>
            <pre>{`environments:\n  - name: pro\n    connection: local-k3s\n    namespace: default\napplications:\n  - name: svc-user\n    selector: app=svc-user`}</pre>
            <p>
              Saving checks the revision. On a conflict your edits are kept so you can refresh and merge. Passwords and
              kubeconfigs never go into project files.
            </p>
          </div>
        </div>
      ) : (
        <div className="ops-sharing">
          <div className="ops-sharing-section">
            <FolderOpen size={22} />
            <div>
              <h2>Project directory</h2>
              <p>{snapshot.root}</p>
              <p className="ops-muted">
                Configuration, the artifact index and release records live together. Each computer uses its own local Git
                checkout.
              </p>
            </div>
            <button
              className="ops-btn"
              disabled={busy}
              onClick={() => void choose()}
            >
              Choose directory
            </button>
          </div>
          <div className="ops-directory">
            <div>
              <FileCode2 size={16} />
              <code>project.yaml</code>
              <span>Applications, environments and cluster connection names</span>
            </div>
            <div>
              <Package size={16} />
              <code>artifacts/*.yaml</code>
              <span>Artifact versions, image digests and sources</span>
            </div>
            <div>
              <Rocket size={16} />
              <code>releases/*.json</code>
              <span>Steps, result and previous image of every release</span>
            </div>
          </div>
          <div className="ops-sharing-section">
            <GitBranch size={22} />
            <div>
              <h2>Sync with your team</h2>
              <p>
                Clone the shared repository first, then choose its root. Push commits only the project files above.
              </p>
              <p className="ops-muted">
                Pull fast-forwards only. It stops on local changes or divergence so you can resolve them.
              </p>
            </div>
          </div>
          <div className="ops-sharing-actions">
            <button
              className="ops-btn"
              disabled={busy}
              onClick={() => void gitAction("status")}
            >
              <GitBranch size={15} />
              Show status
            </button>
            <button
              className="ops-btn"
              disabled={busy}
              onClick={() => void gitAction("pull")}
            >
              <ArrowDownToLine size={15} />
              Pull
            </button>
            <button
              className="ops-btn primary"
              disabled={busy}
              onClick={() => void gitAction("push")}
            >
              <ArrowUpFromLine size={15} />
              Commit and push
            </button>
          </div>

          {git && (
            <pre className="ops-definition" role="status">
              {git}
            </pre>
          )}
          <div className="ops-sharing-section">
            <LockKeyhole size={22} />
            <div>
              <h2>Credentials stay on each device</h2>
              <p>
                The project shares connection names, not passwords, private keys or
                kubeconfigs. Releases check the cluster lock and resource version first.
              </p>
            </div>
          </div>
        </div>
      )}
    </section>
  );
}
