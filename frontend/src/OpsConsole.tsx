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
  | "project"
  | "sharing";
const nav = [
  {
    group: "集群",
    items: [{ id: "nodes", label: "节点 / 机器", icon: Server }],
  },
  {
    group: "部署",
    items: [
      { id: "runs", label: "AI 执行记录", icon: Terminal },
      { id: "artifacts", label: "制品库", icon: Package },
      { id: "releases", label: "制品发布", icon: Rocket },
      { id: "deployments", label: "Deployment", icon: Boxes },
      { id: "pods", label: "Pod 实例", icon: Server },
    ],
  },
  {
    group: "可观测",
    items: [
      { id: "metrics", label: "监控", icon: Activity },
      { id: "logs", label: "日志", icon: FileText },
      { id: "events", label: "应用事件", icon: Bell },
    ],
  },
  {
    group: "配置",
    items: [
      { id: "config", label: "应用配置", icon: Settings2 },
      { id: "nginx", label: "Nginx 配置", icon: FileCode2 },
      { id: "project", label: "项目映射", icon: FileCode2 },
      { id: "sharing", label: "共享与同步", icon: GitBranch },
    ],
  },
];
const titles: Record<Page, string> = {
  nodes: "节点 / 机器",
  runs: "AI 执行记录",
  artifacts: "制品库",
  releases: "制品发布",
  deployments: "Deployment",
  pods: "Pod 实例",
  metrics: "监控",
  logs: "日志",
  events: "应用事件",
  config: "应用配置",
  nginx: "Nginx 配置",
  project: "项目映射",
  sharing: "共享与同步",
};
function operationName(record: Pick<OpsRecord, "action">) {
  if (record.action === "nginx") return "Nginx 配置";
  return record.action === "scale"
    ? "扩缩容"
    : record.action === "restart"
      ? "滚动重启"
      : record.action === "rollback"
        ? "版本回退"
        : "发布";
}
function operationStatus(record: Pick<OpsRecord, "action" | "status" | "origin">) {
  if (record.origin === "external" && record.status === "succeeded") return "上报成功";
  if (record.status === "cancelled") return "已取消";
  const label = !record.action || record.action === "release" ? "发布" : "操作";
  return record.status === "succeeded"
    ? `${label}完成`
    : record.status === "running"
      ? "执行中 / 待核实"
      : `${label}未完成`;
}
function date(s: string) {
  return s ? new Date(s).toLocaleString("zh-CN", { hour12: false }) : "—";
}
function age(s: string) {
  const h = Math.max(0, Math.floor((Date.now() - Date.parse(s)) / 3600000));
  return h < 24 ? `${h} 小时` : `${Math.floor(h / 24)} 天 ${h % 24} 小时`;
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
        aria-label={`${ready} / ${total} 就绪`}
      />
    </span>
  );
}
function Refresher({ busy, onClick }: { busy: boolean; onClick: () => void }) {
  return (
    <button
      className="ops-icon"
      title="刷新"
      aria-label="刷新"
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
    localStorage.getItem("infradex-ops-theme") === "light" ? "light" : "dark",
  );
  const [snapshot, setSnapshot] = useState<OpsSnapshot>();
  const [connections, setConnections] = useState<Connection[]>([]);
  const scopeKey = "infradex-ops-navigation-local";
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
          if (!stale) setNotice(`执行记录刷新失败：${errorText(e)}`);
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
      <nav className="ops-workspace-rail" aria-label="主导航">
        <WorkspaceNavigation active="infra" disabled={busy} />
      </nav>
      <header className="ops-top">
        <a className="ops-brand" href="#infra">
          Infra<span>dex</span>
        </a>
        <nav aria-label="工作区">
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
            集群资源 <ChevronDown size={13} />
          </button>
          <button disabled={busy} onClick={() => setPage("project")}>
            项目设置 <ChevronDown size={13} />
          </button>
        </nav>
        <div className="ops-top-right">
          <button disabled={busy || !snapshot} onClick={openConnect}>
            <PlugZap size={15} />
            接入 K3s
          </button>
          <button
            aria-label={theme === "dark" ? "切换浅色风格" : "切换深色风格"}
            title={theme === "dark" ? "切换浅色风格" : "切换深色风格"}
            onClick={() => {
              const next = theme === "dark" ? "light" : "dark";
              setTheme(next);
              localStorage.setItem("infradex-ops-theme", next);
            }}
          >
            {theme === "dark" ? <Sun size={16} /> : <Moon size={16} />}
          </button>
          <span>
            <HardDrive size={14} />
            本地项目
          </span>
        </div>
      </header>
      <aside className="ops-sidebar">
        <div className="ops-context-select">
          <label>
            <span className="sr-only">环境</span>
            <select
              aria-label="环境"
              disabled={busy}
              value={env?.name || ""}
              onChange={(e) => setEnvironment(e.target.value)}
            >
              {snapshot?.project.environments.map((e) => (
                <option key={e.name}>{e.name}</option>
              ))}
              {!env && <option value="">选择环境</option>}
            </select>
            <LockKeyhole size={13} />
          </label>
          <label>
            <span className="sr-only">应用</span>
            <select
              aria-label="应用"
              disabled={busy}
              value={app?.name || ""}
              onChange={(e) => setApplication(e.target.value)}
            >
              {snapshot?.project.applications.map((a) => (
                <option key={a.name}>{a.name}</option>
              ))}
              {!app && <option value="">选择应用</option>}
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
            数据保存在本地
          </span>
          <button onClick={onWorkbench} disabled={busy}>
            <ArrowLeft size={14} />
            数据库工作台
          </button>
        </div>
      </aside>
      <main className="ops-main">
        {notice && (
          <div className="ops-notice" role="status">
            {notice}
            <button aria-label="关闭提示" onClick={() => setNotice("")}>
              <X size={14} />
            </button>
          </div>
        )}
        {error && (
          <>
            <ErrorNotice>{error}</ErrorNotice>
            <button className="ops-btn" onClick={() => void reload()}>
              重新加载项目
            </button>
          </>
        )}
        {!desktop ? (
          <section className="ops-panel">
            <div className="ops-panel-head">
              <h1>Infra</h1>
            </div>
            <Empty title="连接你的基础设施">
              <span>请打开 Infradex 桌面应用，添加集群连接并接入应用。</span>
            </Empty>
          </section>
        ) : loading && !snapshot ? (
          <div className="ops-loading">
            <LoaderCircle size={20} className="spin" />
            正在打开项目…
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
                        <h1>节点 / 机器</h1>
                      </div>
                      <Empty title="选择环境对应的集群">
                        <span>
                          {matches.length > 1
                            ? "集群连接名称重复，请在集群资源中重命名后刷新。"
                            : "接入 K3s 或在项目映射中配置环境与集群连接。"}
                        </span>
                      </Empty>
                      <div className="ops-empty-action">
                        <button className="ops-btn" onClick={openConnect}>
                          接入 K3s
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
                      ? "发现同名集群连接，请在集群资源重命名后刷新"
                      : "未找到环境对应的集群连接，请在集群资源添加连接，再刷新项目"
                  }
                />
              ) : page === "nginx" && env && app ? (
                <section className="ops-panel">
                  <div className="ops-panel-head"><div><h1>Nginx 配置</h1><p>{app.name} · {env.name} · 宿主机配置</p></div></div>
                  <div className="ops-application-config">
                    <OpsNginxConfig key={`${snapshot.root}:${env.name}:${app.name}:${app.nginxPath || ""}`} projectRoot={snapshot.root} environment={env.name} application={app.name} path={app.nginxPath} saving={busy} onBusy={setBusy} />
                  </div>
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
                      ? "发现同名集群连接，请在集群资源重命名后刷新"
                      : "未找到环境对应的集群连接，请在集群资源添加连接，再刷新项目"
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
                  <Empty title="创建你的第一个应用">
                    <span>选择已有 K3s 连接，读取负载后接入本地应用。</span>
                  </Empty>
                  <div className="ops-empty-action">
                    <button className="ops-btn primary" onClick={openConnect}>
                      <PlugZap size={16} />
                      接入集群应用
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
            setNotice("K3s 应用已接入，连接凭据保留在本机。");
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
      steps: r.events.flatMap(e => e.event.step ? [`${e.event.step.status === "succeeded" ? "完成" : "失败"} · ${e.event.step.name}${e.event.step.message ? `：${e.event.step.message}` : ""}`] : []) }];
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
          {page === "artifacts" && <p>{artifacts.length} 个制品 · 全部环境</p>}
          {page === "releases" && (
            <p>
              {releases.length} 条发布记录 · {env.name}
            </p>
          )}
        </div>
        <div className="ops-actions">
          {page === "artifacts" && (
            <button className="ops-btn primary" onClick={() => setAdd(true)}>
              <Plus size={16} />
              登记制品
            </button>
          )}
          {page === "releases" && (
            <button
              className="ops-btn primary"
              disabled={!artifacts.length || busy}
              onClick={() => setRelease(artifacts[0])}
            >
              <Rocket size={16} />
              发布
            </button>
          )}
          <Refresher busy={loading || busy} onClick={refresh} />
        </div>
      </div>
      <div className="ops-toolbar">
        <span className="ops-muted">当前上下文</span>
        <span className="ops-tag">{env.connection}</span>
        <span>{env.namespace}</span>
        <span className="ops-toolbar-sep" />
        <span>{app.name}</span>
        {["deployments", "pods"].includes(page) && (
          <select
            aria-label="负载类型"
            value={kind}
            onChange={(e) => setKind(e.target.value)}
          >
            <option value="">全部负载类型</option>
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
            每 10 秒刷新
          </label>
        )}
        {["artifacts", "releases", "deployments", "pods"].includes(page) && (
          <label className="ops-search">
            <Search size={14} />
            <input
              aria-label="筛选列表"
              placeholder="搜索名称或版本"
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
              <LoaderCircle size={18} className="spin" /> 正在读取集群负载…
            </div>
          )}
          <ErrorNotice>{clusterError}</ErrorNotice>
          {cluster?.warnings.map((w) => (
            <ErrorNotice key={w}>{w}</ErrorNotice>
          ))}
          {cluster?.truncated && (
            <div className="ops-notice">
              仅显示前 500 条资源，请缩小应用选择器。
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
                    <th>版本 / 来源</th>
                    <th>镜像地址</th>
                    <th>登记时间</th>
                    <th className="ops-right">操作</th>
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
                          <small>{a.source || "手动登记"}</small>
                          {a.runId && (
                            <button
                              className="ops-link"
                              onClick={() => {
                                setRunId(a.runId);
                                onPage("runs");
                              }}
                            >
                              查看构建记录
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
                            发布
                          </button>
                        </td>
                      </tr>
                    ))}
                </tbody>
              </table>
            </div>
          ) : (
            <Empty
              title={artifacts.length ? "没有匹配的制品" : "当前应用暂无制品"}
            >
              登记已推送到镜像仓库的制品，即可生成发布计划。
            </Empty>
          )}
          <p className="ops-footnote">
            制品保存镜像地址与不可变摘要，镜像内容保存在你的 OCI 仓库中。
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
                            ? `副本 ${r.beforeReplicas} → ${r.replicas}`
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
                      {r.origin === "external" ? "外部发布" : "本地客户端"}<small>{date(r.startedAt)}</small>
                    </span>
                    <ChevronRight size={16} />
                  </button>
                ))}
            </div>
          ) : (
            <Empty title="当前上下文暂无发布记录">
              从制品库选择版本，检查发布计划后执行。
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
                  <th>集群 · 副本数</th>
                  <th className="ops-right">操作</th>
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
                        实例
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
                            扩缩容
                          </button>
                          <button
                            className="ops-link"
                            disabled={busy}
                            onClick={() =>
                              setAction({ kind: "restart", workload: w })
                            }
                          >
                            重启
                          </button>
                        </>
                      )}
                      <button className="ops-link" onClick={() => setDetail(w)}>
                        详情
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            {!workloads.length && !loading && (
              <Empty title="没有匹配的工作负载" />
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
              <h3>其他 Pod</h3>
              <PodTable
                pods={pods.filter((p) => !p.workload)}
                onLogs={openLogs}
              />
            </div>
          )}
          {!pods.length && !loading && <Empty title="当前应用暂无 Pod" />}
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
                aria-label="选择 Pod"
                value={selectedPod?.name || ""}
                onChange={(e) => setPod(e.target.value)}
              >
                {cluster?.pods.map((p) => (
                  <option key={p.uid}>{p.name}</option>
                ))}
                {!cluster?.pods.length && <option value="">暂无 Pod</option>}
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
                  ? "正在读取 Pod…"
                  : "选择 Pod 后查看" + (page === "logs" ? "日志" : "事件")
              }
            />
          )}
        </div>
      )}
      {needsCluster && cluster && (
        <p className="ops-footnote">
          最近读取：{date(cluster.fetchedAt)} · {cluster.workloads.length}{" "}
          个负载
          {clusterMode === "full" ? ` · ${cluster.pods.length} 个 Pod` : ""}
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
                "如客户端曾中断，请刷新集群核实状态，不要直接重试。"}
            </p>
            {record.action === "nginx" ? <p>宿主机 Nginx 配置校验与重载记录</p> : record.action === "scale" || record.action === "restart" ? (
              <p>
                副本数：{record.beforeReplicas} → {record.replicas}
                {record.action === "restart"
                  ? " · 按 Deployment 策略重建 Pod"
                  : ""}
              </p>
            ) : (
              <div className="ops-release-images">
                <label>
                  原镜像<code>{record.beforeImage || "未记录"}</code>
                </label>
                <label>
                  目标镜像<code>{record.image}</code>
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
                {record.origin === "external" ? "查看发布执行记录" : "查看构建记录"}
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
                回退到此版本
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
            <th>名称</th>
            <th>Pod IP</th>
            <th>节点 IP</th>
            <th>重启</th>
            <th>运行时间</th>
            <th>端口</th>
            <th>状态</th>
            <th className="ops-right">操作</th>
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
                  {p.ready} / {p.total} 就绪
                </small>
              </td>
              <td className="ops-right">
                <button
                  className="ops-icon"
                  aria-label={`查看 ${p.name} 日志`}
                  title="查看日志"
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
        <div className="ops-inline-empty">此负载暂无匹配的 Pod</div>
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
        if (!connection) throw new Error("请先配置集群连接");
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
          <span>CPU 使用量</span>
          <strong>
            {metrics.length
              ? metrics.reduce((sum, m) => sum + m.cpu, 0).toFixed(0)
              : "—"}
            <small>mCPU</small>
          </strong>
        </div>
        <div>
          <span>内存使用量</span>
          <strong>
            {metrics.length
              ? metrics.reduce((sum, m) => sum + m.memory, 0).toFixed(1)
              : "—"}
            <small>MiB</small>
          </strong>
        </div>
        <div>
          <span>Pod 就绪</span>
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
              <th>CPU（mCPU）</th>
              <th>内存（MiB）</th>
              <th>采样时间</th>
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
          <Empty title="暂无可用指标">
            需要目标集群安装 metrics-server，并授予读取权限。
          </Empty>
        )}
      </div>
      <p className="ops-footnote">
        当前资源用量快照 · 1,000 mCPU = 1 核 · 不保存历史监控数据
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
          if (!container) throw new Error("此 Pod 没有可读取日志的容器");
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
              容器{" "}
              <select
                aria-label="日志容器"
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
              行数{" "}
              <select
                aria-label="日志行数"
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
              上一实例
            </label>
            <label className="ops-search">
              <Search size={14} />
              <input
                aria-label="过滤日志"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder="过滤当前日志"
              />
            </label>
          </div>
          <pre className="ops-log-output" aria-label="日志输出">
            {loading
              ? "正在读取日志…"
              : visible || (!error ? "暂无日志输出" : "读取失败")}
          </pre>
          {truncated && (
            <p className="ops-notice">
              日志达到 256 KiB 限制，请减少读取行数。
            </p>
          )}
          <p className="ops-footnote">
            日志快照 · {poll ? "每 10 秒重新读取" : "手动刷新"} ·
            日志正文不写入共享项目
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
            <Empty title="此 Pod 暂无保留事件" />
          )}
          {events?.truncated && (
            <p className="ops-notice">事件列表达到预览限制。</p>
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
      setError("镜像必须包含完整的 sha256 摘要，不能使用 latest");
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
      title="登记制品"
      subtitle={`${app} · 已有 OCI 镜像`}
      onClose={() => {
        if (!saving) onClose();
      }}
    >
      <form onSubmit={save}>
        <div className="ops-dialog-body ops-form">
          <ErrorNotice>{error}</ErrorNotice>
          <label>
            版本
            <input
              required
              maxLength={120}
              autoFocus
              placeholder="例如 1.4.3"
              value={version}
              onChange={(e) => setVersion(e.target.value)}
            />
          </label>
          <label>
            镜像地址
            <textarea
              aria-label="镜像地址"
              required
              rows={3}
              maxLength={1024}
              placeholder="registry.local:5000/app@sha256:…"
              value={image}
              onChange={(e) => setImage(e.target.value.trim())}
            />
          </label>
          <label>
            来源 / 更新说明
            <input
              maxLength={500}
              placeholder="提交说明或构建编号"
              value={source}
              onChange={(e) => setSource(e.target.value)}
            />
          </label>
          <p className="ops-muted">
            登记不会上传或构建镜像。请填写已存在且目标集群可以拉取的镜像摘要。
          </p>
        </div>
        <div className="ops-dialog-footer">
          <button
            type="button"
            className="ops-btn"
            disabled={saving}
            onClick={onClose}
          >
            取消
          </button>
          <button className="ops-btn primary" disabled={saving}>
            {saving ? (
              <LoaderCircle size={15} className="spin" />
            ) : (
              <Plus size={15} />
            )}
            保存制品
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
      if (!connection) throw new Error("请先配置集群连接");
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
      title={plan ? "确认发布计划" : "发布制品"}
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
              aria-label="发布目标"
              value={workload?.name || ""}
              disabled={busy || !!plan}
              onChange={(e) => setTarget(e.target.value)}
            >
              {deployments.map((w) => (
                <option key={w.uid}>{w.name}</option>
              ))}
              {!deployments.length && (
                <option value="">暂无可用 Deployment</option>
              )}
            </select>
          </label>
          <label>
            容器
            <select
              aria-label="发布容器"
              value={container}
              disabled={busy || !!plan || detailsLoading}
              onChange={(e) => setContainer(e.target.value)}
            >
              {containers.map((c) => (
                <option key={c}>{c}</option>
              ))}
              {!containers.length && (
                <option value="">
                  {detailsLoading ? "正在读取容器…" : "暂无容器"}
                </option>
              )}
            </select>
          </label>
          <div className="ops-release-images">
            {plan && (
              <label>
                当前镜像<code>{plan.beforeImage}</code>
              </label>
            )}
            <label>
              目标镜像<code>{artifact.image}</code>
            </label>
          </div>
          {plan ? (
            <p className="ops-muted">
              预检查已通过。确认后更新镜像并等待就绪，最长 180
              秒。失败不会自动回滚。计划有效至 {date(plan.expiresAt)}。
            </p>
          ) : (
            <p className="ops-muted">
              先读取目标状态并执行服务端 dry-run，再确认发布。
            </p>
          )}
          {busy && plan && (
            <p role="status">
              <LoaderCircle size={16} className="spin" />
              正在执行发布并等待就绪…
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
            {plan ? "返回修改" : "取消"}
          </button>
          {plan ? (
            <button
              type="button"
              className="ops-btn primary"
              disabled={busy}
              onClick={() => void apply()}
            >
              <Rocket size={15} />
              {"确认发布"}
            </button>
          ) : (
            <button
              className="ops-btn primary"
              disabled={busy || detailsLoading || !workload || !container}
            >
              生成发布计划
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
        throw new Error("副本数必须是 0 到 1000 之间的整数");
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
      title={plan ? `确认${title}计划` : title}
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
                目标副本数
                <input
                  aria-label="目标副本数"
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
                  ? `副本数：${plan.beforeReplicas} → ${plan.replicas}`
                  : `当前副本数：${current?.desired ?? action.workload.desired}`}
                。若负载由 HPA 管理，自动扩缩容可能再次调整副本数。
              </p>
              {Number(replicas) === 0 && (
                <div className="ops-notice">
                  设为 0 将停止此 Deployment 的所有 Pod。
                </div>
              )}
            </>
          )}
          {action.kind === "restart" && (
            <p>
              更新 Pod 模板注解，按当前 Deployment 的更新策略重建
              Pod，保留镜像和副本配置。
            </p>
          )}
          {action.kind === "rollback" && (
            <div className="ops-release-images">
              <label>
                版本
                <code>
                  {action.record.version} · {action.record.container}
                </code>
              </label>
              {plan && (
                <label>
                  当前镜像<code>{plan.beforeImage}</code>
                </label>
              )}
              <label>
                目标镜像<code>{action.record.image}</code>
              </label>
              <p className="ops-muted">
                回退此容器的镜像版本，保留当前配置和其他容器。
              </p>
            </div>
          )}
          <p className="ops-muted">
            {plan
              ? `预检查已通过。执行后等待就绪，最长 180 秒。计划有效至 ${date(plan.expiresAt)}。`
              : "生成计划会读取最新状态并执行服务端预检查。"}
          </p>
          {busy && (
            <p role="status">
              <LoaderCircle size={15} className="spin" />
              {plan ? "正在执行并等待就绪…" : "正在生成计划…"}
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
            {plan ? "返回修改" : "取消"}
          </button>
          {plan ? (
            <button
              type="button"
              className="ops-btn primary"
              disabled={busy}
              onClick={() => void apply()}
            >
              {`确认${title}`}
            </button>
          ) : (
            <button className="ops-btn primary" disabled={busy}>
              生成操作计划
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
      subtitle={`${workload.kind} · ${workload.namespace} · 只读定义`}
      wide
      onClose={onClose}
    >
      <div className="ops-dialog-body">
        <ErrorNotice>{error}</ErrorNotice>
        <pre className="ops-definition">
          {text || (!error ? "正在读取…" : "")}
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
      onNotice("项目映射已保存");
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
                格式化 YAML
              </button>
              <button className="ops-btn" onClick={download}>
                <ArrowDownToLine size={15} />
                导出 YAML
              </button>
              <button
                className="ops-btn primary"
                disabled={busy}
                onClick={() => void save()}
              >
                <Check size={15} />
                保存配置
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
              {text !== snapshot.yaml ? "有未保存改动" : "已保存"}
            </span>
          </div>
          <textarea
            className="ops-yaml"
            aria-label="项目 YAML"
            spellCheck
            value={text}
            onChange={(e) => setText(e.target.value)}
            disabled={busy}
          />
          <div className="ops-config-help">
            <strong>项目与应用映射</strong>
            <p>
              这里维护整个项目的环境和应用选择器。查看所选应用的 ConfigMap
              和环境变量，请打开“应用配置”。
            </p>
            <p>
              connection 填写集群资源中 Kubernetes / K3s
              连接的名称。每个人可以使用自己的 kubeconfig，应用通过 selector
              匹配负载和 Pod。
            </p>
            <pre>{`environments:\n  - name: pro\n    connection: local-k3s\n    namespace: default\napplications:\n  - name: svc-user\n    selector: app=svc-user`}</pre>
            <p>
              保存时校验版本；出现冲突时先保留编辑内容，再刷新并合并。密码与
              kubeconfig 不放入项目文件。
            </p>
          </div>
        </div>
      ) : (
        <div className="ops-sharing">
          <div className="ops-sharing-section">
            <FolderOpen size={22} />
            <div>
              <h2>项目目录</h2>
              <p>{snapshot.root}</p>
              <p className="ops-muted">
                配置、制品索引和发布记录集中存放。每台电脑使用自己的本地 Git
                副本。
              </p>
            </div>
            <button
              className="ops-btn"
              disabled={busy}
              onClick={() => void choose()}
            >
              选择目录
            </button>
          </div>
          <div className="ops-directory">
            <div>
              <FileCode2 size={16} />
              <code>project.yaml</code>
              <span>应用、环境与集群连接名称</span>
            </div>
            <div>
              <Package size={16} />
              <code>artifacts/*.yaml</code>
              <span>制品版本、镜像摘要与来源</span>
            </div>
            <div>
              <Rocket size={16} />
              <code>releases/*.json</code>
              <span>每次发布的步骤、结果与原镜像</span>
            </div>
          </div>
          <div className="ops-sharing-section">
            <GitBranch size={22} />
            <div>
              <h2>与团队同步</h2>
              <p>
                先克隆共享仓库，再选择仓库根目录。推送只提交上面的项目文件。
              </p>
              <p className="ops-muted">
                拉取采用快进合并；有本地改动或分叉时停止，由你处理冲突。
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
              查看状态
            </button>
            <button
              className="ops-btn"
              disabled={busy}
              onClick={() => void gitAction("pull")}
            >
              <ArrowDownToLine size={15} />
              拉取更新
            </button>
            <button
              className="ops-btn primary"
              disabled={busy}
              onClick={() => void gitAction("push")}
            >
              <ArrowUpFromLine size={15} />
              提交并推送
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
              <h2>每台设备独立保管凭据</h2>
              <p>
                项目共享连接名称，不共享密码、私钥或
                kubeconfig。发布前会检查集群锁和资源版本。
              </p>
            </div>
          </div>
        </div>
      )}
    </section>
  );
}
