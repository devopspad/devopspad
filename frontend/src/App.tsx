import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  PanelRight,
  ArrowDownToLine,
  ArrowRight,
  ArrowUpDown,
  Bookmark,
  Boxes,
  Braces,
  Check,
  ChevronDown,
  ChevronRight,
  ChevronLeft,
  CircleHelp,
  Clock3,
  Copy,
  Database,
  FileCode2,
  History,
  Layers3,
  LoaderCircle,
  Play,
  Plus,
  RefreshCw,
  Search,
  Settings2,
  ShieldCheck,
  Sparkles,
  Square,
  Table2,
  Terminal,
  Trash2,
  X,
} from "lucide-react";
import { api, desktop, display, errorText, toCSV } from "./api";
import {
  defaultQuery,
  engines,
  type Connection,
  type HistoryItem,
  type Resource,
  type ResourcePage,
  type Tab,
  type ClusterRef,
} from "./types";
import { ConnectionDialog, EngineIcon } from "./ConnectionDialog";
import { loadSession, useWorkspaceSession } from "./workspace-session";
import { SettingsDialog } from "./SettingsDialog";
import { Assistant } from "./Assistant";
import { Editor } from "./Editor";
import { engineCapabilities } from "./capabilities";
import { ResourceInspector } from "./ResourceInspector";
import { ConnectionCapabilities } from "./ConnectionCapabilities";
import { ClusterInspector } from "./ClusterInspector";
import { InfraCluster } from "./InfraCluster";
import { WorkspaceNavigation } from "./WorkspaceNavigation";
import { Modal } from "./Modal";
import "./style.css";
import "./desktop.css";
import "./orca.css";
type View = "workbench" | "connections" | "history" | "saved";
type Snippet = {
  id: string;
  title: string;
  query: string;
  kind: string;
};
function loadSnippets(): Snippet[] {
  try {
    const parsed = JSON.parse(
      localStorage.getItem("devopspad-snippets") || "[]",
    );
    return Array.isArray(parsed)
      ? parsed.filter(
          (s) =>
            s &&
            typeof s.id === "string" &&
            typeof s.title === "string" &&
            typeof s.query === "string" &&
            typeof s.kind === "string",
        )
      : [];
  } catch {
    return [];
  }
}
export default function App({
  mode = "workbench",
}: {
  mode?: "workbench" | "infra";
}) {
  const [initialSession] = useState(() => loadSession());
  const initialWorkspace = initialSession.session.workspace;
  const [connections, setConnections] = useState<Connection[]>([]);
  const [view, setView] = useState<View>("workbench");
  const [tabs, setTabs] = useState<Tab[]>(initialWorkspace.tabs);
  const [middlewareActiveId, setMiddlewareActiveId] = useState(
    initialWorkspace.activeId,
  );
  const [infraActiveId, setInfraActiveId] = useState(initialWorkspace.activeId);
  const activeId = mode === "infra" ? infraActiveId : middlewareActiveId;
  const setActiveId =
    mode === "infra" ? setInfraActiveId : setMiddlewareActiveId;
  const [middlewareSelected, setMiddlewareSelected] = useState(
    initialWorkspace.selected,
  );
  const [infraSelected, setInfraSelected] = useState(
    () => localStorage.getItem("devopspad-infra-connection") || "",
  );
  const [infraEnvironment, setInfraEnvironment] = useState(
    () => localStorage.getItem("devopspad-infra-environment") || "",
  );
  const sourceConnections = connections;
  const groupConnections = sourceConnections.filter(
    (c) => (c.kind === "k8s" || c.kind === "k3s") === (mode === "infra"),
  );
  const allConnections = groupConnections.filter(
    (c) =>
      mode !== "infra" ||
      !infraEnvironment ||
      (c.environment || "未分组") === infraEnvironment,
  );
  const selectedID = mode === "infra" ? infraSelected : middlewareSelected;
  const selected =
    mode === "infra" && !allConnections.some((c) => c.id === selectedID)
      ? allConnections[0]?.id || ""
      : selectedID;
  const setSelected =
    mode === "infra" ? setInfraSelected : setMiddlewareSelected;
  useEffect(() => {
    if (mode === "infra") {
      localStorage.setItem("devopspad-infra-connection", selected);
      localStorage.setItem("devopspad-infra-environment", infraEnvironment);
    }
  }, [mode, selected, infraEnvironment]);
  useEffect(() => {
    setView("workbench");
  }, [mode]);
  const recovery = useWorkspaceSession(initialSession, {
    tabs,
    activeId: middlewareActiveId,
    selected: middlewareSelected,
  });
  const [resources, setResources] = useState<Record<string, Resource[]>>({});
  const [databaseScopes, setDatabaseScopes] = useState<Record<string, string>>(
    {},
  );
  const scopeRef = useRef(databaseScopes);
  scopeRef.current = databaseScopes;
  const [databaseLists, setDatabaseLists] = useState<Record<string, string[]>>(
    {},
  );
  const [databaseErrors, setDatabaseErrors] = useState<Record<string, string>>(
    {},
  );
  const [resourcePages, setResourcePages] = useState<
    Record<string, ResourcePage>
  >({});
  const pagesRef = useRef(resourcePages);
  pagesRef.current = resourcePages;
  const [keyPatterns, setKeyPatterns] = useState<Record<string, string>>({});
  const appliedPatternsRef = useRef<Record<string, string>>({});
  const patternsRef = useRef(keyPatterns);
  patternsRef.current = keyPatterns;
  const browseVersions = useRef<Record<string, number>>({});
  const [connectionChecks, setConnectionChecks] = useState<
    Record<
      string,
      {
        state: "checking" | "ready" | "error";
        at?: string;
      }
    >
  >({});
  const [objectInspector, setObjectInspector] = useState<{
    connection: Connection;
    database: string;
    resource: Resource;
  } | null>(null);
  const [capabilityKind, setCapabilityKind] = useState<
    Connection["kind"] | null
  >(null);
  const [browseErrors, setBrowseErrors] = useState<Record<string, string>>({});
  const [loadingId, setLoadingId] = useState("");
  const [connectionDialog, setConnectionDialog] = useState<
    Connection | "new" | null
  >(null);
  const [settings, setSettings] = useState(false);
  const [middlewareAI, setMiddlewareAI] = useState(true);
  const [infraAI, setInfraAI] = useState(false);
  const aiOpen = mode === "infra" ? infraAI : middlewareAI;
  const setAiOpen = mode === "infra" ? setInfraAI : setMiddlewareAI;
  const [help, setHelp] = useState(false);
  const [search, setSearch] = useState("");
  const [tableFilter, setTableFilter] = useState("");
  const [sort, setSort] = useState<{
    column: number;
    direction: number;
  } | null>(null);
  const [page, setPage] = useState(0);
  const [running, setRunning] = useState<Record<string, string>>({});
  const runningRef = useRef<Record<string, string>>({});
  const cancelled = useRef(new Set<string>());
  const [history, setHistory] = useState<HistoryItem[]>([]);
  const [snippets, setSnippets] = useState<Snippet[]>(loadSnippets);
  const [toast, setToast] = useState("");
  const [clusterInspector, setClusterInspector] = useState<{
    connection: Connection;
    resource: ClusterRef;
  } | null>(null);
  const [inspect, setInspect] = useState<{
    columns: string[];
    row: unknown[];
  } | null>(null);
  const [palette, setPalette] = useState(false);
  const [paletteQuery, setPaletteQuery] = useState("");
  const [deleteTarget, setDeleteTarget] = useState<Connection | null>(null);
  const [snippetTitle, setSnippetTitle] = useState<string | null>(null);
  const visibleTabs = tabs.filter((t) =>
    allConnections.some((c) => c.id === t.connectionId),
  );
  const active = visibleTabs.find((t) => t.id === activeId);
  const connection = allConnections.find(
    (c) => c.id === (active?.connectionId || selected),
  );
  const selectedConnection = allConnections.find((c) => c.id === selected);
  const notify = (message: string) => setToast(message);
  useEffect(() => {
    if (desktop)
      api
        .ListConnections()
        .then((cs) => {
          setConnections(cs);
          const restored = recovery.workspace(cs.map((c) => c.id));
          setTabs(restored.tabs);
          setMiddlewareActiveId(restored.activeId);
          setInfraActiveId(restored.activeId);
          setMiddlewareSelected(restored.selected);
        })
        .catch((e) => notify(errorText(e)));
  }, []);
  useEffect(() => {
    if (!desktop) return;
    const refresh = () => {
      api
        .ListConnections()
        .then((cs) => {
          setConnections(cs);
        })
        .catch((e) => notify(errorText(e)));
    };
    window.addEventListener("focus", refresh);
    return () => window.removeEventListener("focus", refresh);
  }, []);
  useEffect(() => {
    if (!toast) return;
    const timeout = setTimeout(() => setToast(""), 5000);
    return () => clearTimeout(timeout);
  }, [toast]);
  useEffect(() => {
    setPage(0);
    setTableFilter("");
    setSort(null);
  }, [activeId]);
  const browse = useCallback(
    async (c: Connection, override?: string, append = false) => {
      const scope = override ?? scopeRef.current[c.id] ?? c.database;
      const appliedPattern = append
        ? appliedPatternsRef.current[c.id] || "*"
        : patternsRef.current[c.id] || "*";
      if (!append) appliedPatternsRef.current[c.id] = appliedPattern;
      const version = (browseVersions.current[c.id] || 0) + 1;
      browseVersions.current[c.id] = version;
      setLoadingId(c.id);
      setBrowseErrors((es) => ({ ...es, [c.id]: "" }));
      setConnectionChecks((cs) => ({ ...cs, [c.id]: { state: "checking" } }));
      if (!append) {
        setResources((rs) => ({ ...rs, [c.id]: [] }));
        setResourcePages((ps) => ({
          ...ps,
          [c.id]: {
            resources: [],
            nextCursor: "",
            hasMore: false,
            message: "",
          },
        }));
      }
      try {
        let result: ResourcePage;
        result = await api.BrowseResources(c.id, {
          database: scope,
          cursor: append ? pagesRef.current[c.id]?.nextCursor || "0" : "0",
          filter: appliedPattern,
        });
        if (browseVersions.current[c.id] !== version) return;
        setResources((rs) => {
          const merged = append
            ? [...(rs[c.id] || []), ...result.resources]
            : result.resources;
          return {
            ...rs,
            [c.id]: [...new Map(merged.map((r) => [r.name, r])).values()],
          };
        });
        setResourcePages((ps) => ({ ...ps, [c.id]: result }));
        setConnectionChecks((cs) => ({
          ...cs,
          [c.id]: {
            state: "ready",
            at: new Date().toLocaleTimeString("en-US"),
          },
        }));
      } catch (e) {
        if (browseVersions.current[c.id] !== version) return;
        setBrowseErrors((es) => ({ ...es, [c.id]: errorText(e) }));
        setConnectionChecks((cs) => ({ ...cs, [c.id]: { state: "error" } }));
      } finally {
        if (browseVersions.current[c.id] === version)
          setLoadingId((current) => (current === c.id ? "" : current));
      }
    },
    [],
  );
  const databaseVersions = useRef<Record<string, number>>({});
  const loadDatabases = useCallback(async (c: Connection) => {
    const version = (databaseVersions.current[c.id] || 0) + 1;
    databaseVersions.current[c.id] = version;
    setDatabaseErrors((es) => ({ ...es, [c.id]: "" }));
    try {
      const names = await api.ListDatabases(c.id);
      if (databaseVersions.current[c.id] !== version) return;
      setDatabaseLists((ls) => ({ ...ls, [c.id]: names }));
    } catch (e) {
      if (databaseVersions.current[c.id] !== version) return;
      setDatabaseErrors((es) => ({ ...es, [c.id]: errorText(e) }));
    }
  }, []);
  useEffect(() => {
    const c = allConnections.find((c) => c.id === selected);
    if (!c) return;
    if (!resources[c.id]) void browse(c);
    if (engineCapabilities(c.kind).canSelectDatabase && !databaseLists[c.id])
      void loadDatabases(c);
  }, [selected, mode, connections, browse, loadDatabases]);
  function selectScope(c: Connection, database: string) {
    setDatabaseScopes((ss) => ({ ...ss, [c.id]: database }));
    scopeRef.current = { ...scopeRef.current, [c.id]: database };
    void browse(c, database);
  }
  function activateTab(t: Tab) {
    setActiveId(t.id);
    setSelected(t.connectionId);
    const c = allConnections.find((c) => c.id === t.connectionId);
    if (
      c &&
      (t.context ?? c.context) === c.context &&
      t.database !== undefined &&
      t.database !== (scopeRef.current[c.id] ?? c.database)
    )
      selectScope(c, t.database);
  }
  function patchTab(id: string, update: Partial<Tab>) {
    setTabs((ts) => ts.map((t) => (t.id === id ? { ...t, ...update } : t)));
  }
  function openQuery(
    c: Connection,
    r?: Resource,
    scopeOverride?: string,
    contextOverride?: string,
  ) {
    const context = contextOverride ?? c.context;
    const database = scopeOverride ?? scopeRef.current[c.id] ?? c.database;
    if (
      context === c.context &&
      database !== (scopeRef.current[c.id] ?? c.database)
    )
      selectScope(c, database);
    const existing =
      r &&
      tabs.find(
        (t) =>
          t.connectionId === c.id &&
          (t.context ?? c.context) === context &&
          (t.database ?? c.database) === database &&
          t.title === r.name,
      );
    if (existing) {
      activateTab(existing);
      setView("workbench");
      return;
    }
    const tab = {
      id: crypto.randomUUID(),
      connectionId: c.id,
      title: r?.name || `Query ${tabs.length + 1}`,
      database,
      context,
      resource: r,
      query: r?.query || defaultQuery(c.kind),
    };
    setTabs((ts) => [...ts, tab]);
    setActiveId(tab.id);
    setSelected(c.id);
    setView("workbench");
  }
  function closeTab(id: string) {
    const request = runningRef.current[id];
    if (request) void cancel(id);
    const next = tabs.filter((t) => t.id !== id);
    setTabs(next);
    if (activeId === id) setActiveId(next[next.length - 1]?.id || "");
  }
  function inspectRow(row: unknown[]) {
    if (!active?.result || !connection) return;
    const result = active.result;
    if (
      (connection.kind === "k8s" || connection.kind === "k3s") &&
      result.resourceType
    ) {
      const obj = row[result.columns.indexOf("resource")] as
        | {
            metadata?: {
              uid?: string;
            };
          }
        | undefined;
      setClusterInspector({
        connection: {
          ...connection,
          context: active.context ?? connection.context,
        },
        resource: {
          context: active.context ?? connection.context,
          namespace: String(row[result.columns.indexOf("namespace")] ?? ""),
          name: String(row[result.columns.indexOf("name")] ?? ""),
          resource: result.resourceType,
          uid: obj?.metadata?.uid || "",
        },
      });
    } else setInspect({ columns: result.columns, row });
  }
  async function runQuery(queryOverride?: string) {
    if (!active || !connection || runningRef.current[active.id]) return;
    const tab = { ...active, query: queryOverride ?? active.query };
    const c = connection;
    const request = crypto.randomUUID();
    runningRef.current = { ...runningRef.current, [tab.id]: request };
    setRunning({ ...runningRef.current });
    patchTab(tab.id, { error: undefined, query: tab.query });
    setTableFilter("");
    setPage(0);
    try {
      const result =
        c.kind === "k8s" || c.kind === "k3s"
          ? await api.ExecuteInContext(
              c.id,
              tab.database ?? c.database,
              tab.context ?? c.context,
              tab.query,
              request,
            )
          : await api.ExecuteScoped(
              c.id,
              tab.database ?? c.database,
              tab.query,
              request,
            );
      if (cancelled.current.has(request)) return;
      patchTab(tab.id, { result, resultQuery: tab.query, error: undefined });
      setConnectionChecks((cs) => ({
        ...cs,
        [c.id]: { state: "ready", at: new Date().toLocaleTimeString("en-US") },
      }));
      setHistory((h) =>
        [
          {
            id: request,
            connectionId: c.id,
            database: tab.database ?? c.database,
            context: tab.context ?? c.context,
            name: c.name,
            query: tab.query,
            at: new Date().toISOString(),
            rows: result.rows.length,
            duration: result.duration,
          },
          ...h,
        ].slice(0, 100),
      );
    } catch (e) {
      if (!cancelled.current.has(request)) {
        const error = errorText(e);
        patchTab(tab.id, { error, result: undefined });
        setConnectionChecks((cs) => ({ ...cs, [c.id]: { state: "error" } }));
        setHistory((h) =>
          [
            {
              id: request,
              connectionId: c.id,
              database: tab.database ?? c.database,
              context: tab.context ?? c.context,
              name: c.name,
              query: tab.query,
              at: new Date().toISOString(),
              rows: 0,
              duration: 0,
              error,
            },
            ...h,
          ].slice(0, 100),
        );
      }
    } finally {
      delete runningRef.current[tab.id];
      setRunning({ ...runningRef.current });
      cancelled.current.delete(request);
    }
  }
  async function cancel(tabID: string) {
    const request = runningRef.current[tabID];
    if (!request) return;
    cancelled.current.add(request);
    try {
      await api.CancelQuery(request);
      patchTab(tabID, { error: "Query cancelled." });
    } catch (e) {
      notify(errorText(e));
    }
  }
  const runRef = useRef(runQuery);
  runRef.current = runQuery;
  useEffect(() => {
    function key(e: KeyboardEvent) {
      if (
        !window.location.hash.startsWith("#workbench") &&
        !window.location.hash.startsWith("#infra/resources")
      )
        return;
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setPalette((p) => !p);
      }
      if (
        (e.metaKey || e.ctrlKey) &&
        e.key === "Enter" &&
        !(e.target as HTMLElement).closest(".cm-editor")
      ) {
        e.preventDefault();
        void runRef.current();
      }
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "n") {
        e.preventDefault();
        setConnectionDialog("new");
      }
    }
    window.addEventListener("keydown", key);
    return () => window.removeEventListener("keydown", key);
  }, []);
  async function exportResults() {
    if (!active?.result) return;
    const csv = toCSV(active.result);
    try {
      if (desktop) {
        if (await api.ExportCSV(csv)) notify("Results exported to CSV");
      } else {
        const url = URL.createObjectURL(
          new Blob([csv], { type: "text/csv;charset=utf-8;" }),
        );
        const link = document.createElement("a");
        link.href = url;
        link.download = "devopspad-results.csv";
        link.click();
        URL.revokeObjectURL(url);
        notify("Results exported to CSV");
      }
    } catch (e) {
      notify(errorText(e));
    }
  }
  const rows = useMemo(() => {
    let items = (active?.result?.rows || []).filter((row) =>
      row.some((value) =>
        display(value).toLowerCase().includes(tableFilter.toLowerCase()),
      ),
    );
    if (sort)
      items = [...items].sort(
        (a, b) =>
          display(a[sort.column]).localeCompare(
            display(b[sort.column]),
            undefined,
            { numeric: true },
          ) * sort.direction,
      );
    return items;
  }, [active?.result, tableFilter, sort]);
  const visibleConnections = allConnections.filter((c) =>
    `${c.name} ${engines[c.kind].name} ${c.host}`
      .toLowerCase()
      .includes(search.toLowerCase()),
  );
  const paletteItems = allConnections.filter((c) =>
    `${c.name} ${engines[c.kind].name}`
      .toLowerCase()
      .includes(paletteQuery.toLowerCase()),
  );
  return (
    <div
      className={`app-shell ${aiOpen ? "with-assistant" : "without-assistant"}`}
    >
      <nav className="app-rail" aria-label="Main navigation">
        <WorkspaceNavigation active={mode} />
        {(
          [
            { id: "workbench", icon: Terminal, title: "Workbench" },
            { id: "connections", icon: Boxes, title: "Connections" },
            { id: "saved", icon: Bookmark, title: "Saved queries" },
            { id: "history", icon: History, title: "Query history" },
          ] as const
        ).map((item) => (
          <button
            key={item.id}
            className={`rail-button ${view === item.id ? "active" : ""}`}
            aria-label={`${item.title}${item.id === "connections" ? ` ${allConnections.length}` : ""}`}
            title={item.title}
            onClick={() => setView(item.id)}
          >
            <item.icon size={21} strokeWidth={1.6} />
          </button>
        ))}
        <button
          className="rail-button"
          title="New connection"
          aria-label="New connection"
          onClick={() => setConnectionDialog("new")}
        >
          <Plus size={22} strokeWidth={1.6} />
        </button>
        <div className="rail-spacer" />
        <button
          className={`rail-button ${aiOpen ? "active" : ""}`}
          title="Toggle AI panel"
          aria-label="Toggle AI panel"
          aria-pressed={aiOpen}
          onClick={() => setAiOpen((open) => !open)}
        >
          <PanelRight size={20} strokeWidth={1.6} />
        </button>
        <button
          className="rail-button"
          title="Help and keyboard shortcuts"
          aria-label="Help and keyboard shortcuts"
          onClick={() => setHelp(true)}
        >
          <CircleHelp size={20} strokeWidth={1.6} />
        </button>
        <button
          className="rail-button"
          title="Settings"
          aria-label="Settings"
          onClick={() => setSettings(true)}
        >
          <Settings2 size={21} strokeWidth={1.6} />
        </button>
      </nav>
      <aside className="sidebar">
        <div className="explorer-heading">
          <span>{mode === "infra" ? "Infra · 集群与节点" : "中间件"}</span>
          <button
            className="icon-button"
            aria-label="Search workspace"
            title="Search workspace"
            onClick={() => {
              setPaletteQuery("");
              setPalette(true);
            }}
          >
            <Search size={16} />
          </button>
        </div>
        <div className="workspace-switch">
          <Database className="workspace-symbol" size={18} />
          <span>
            <strong>本地工作区</strong>
            <small>你的连接与查询</small>
          </span>
        </div>
        <div className="section-label">
          <span>CONNECTIONS</span>
          <div>
            <button
              className="icon-button"
              aria-label="Refresh resources"
              onClick={() =>
                selectedConnection && void browse(selectedConnection)
              }
            >
              <RefreshCw size={13} className={loadingId ? "spin" : ""} />
            </button>
            <button
              className="icon-button"
              aria-label="New connection"
              onClick={() => setConnectionDialog("new")}
            >
              <Plus size={15} />
            </button>
          </div>
        </div>
        {mode === "infra" && (
          <label className="infra-environment-filter">
            环境
            <select
              aria-label="Infra 环境"
              value={infraEnvironment}
              onChange={(e) => {
                setInfraEnvironment(e.target.value);
                setInfraActiveId("");
              }}
            >
              <option value="">全部环境</option>
              {[
                ...new Set([
                  ...groupConnections.map((c) => c.environment || "未分组"),
                  ...(infraEnvironment ? [infraEnvironment] : []),
                ]),
              ]
                .sort()
                .map((name) => (
                  <option key={name}>{name}</option>
                ))}
            </select>
          </label>
        )}
        <div className="sidebar-search">
          <Search size={14} />
          <input
            placeholder="Filter connections…"
            aria-label="Filter connections"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </div>
        <div className="connection-tree">
          {mode === "infra" && !visibleConnections.length && (
            <p className="tree-message">
              当前环境没有集群连接，可切换环境或新建 K3s / Kubernetes 连接。
            </p>
          )}
          {visibleConnections.map((c) => (
            <div className="connection-group" key={c.id}>
              <button
                className={`tree-connection ${selected === c.id ? "selected" : ""}`}
                onClick={() => {
                  if (selected === c.id) {
                    if (c.kind === "k8s" || c.kind === "k3s") {
                      setView("workbench");
                      setActiveId("");
                      return;
                    }
                    setSelected("");
                    return;
                  }
                  setSelected(c.id);
                  if (c.kind === "k8s" || c.kind === "k3s") {
                    setView("workbench");
                    setActiveId("");
                  }
                }}
              >
                <ChevronRight
                  className={selected === c.id ? "expanded" : ""}
                  size={13}
                />
                <EngineIcon kind={c.kind} small />
                <span>{c.name}</span>
                <span
                  className={`connection-dot ${connectionChecks[c.id]?.state === "ready" ? "ready" : connectionChecks[c.id]?.state === "error" ? "failed" : ""}`}
                  title={
                    connectionChecks[c.id]?.state === "ready"
                      ? `Last successful request at ${connectionChecks[c.id]?.at}`
                      : connectionChecks[c.id]?.state === "error"
                        ? "Last request failed"
                        : "Not checked"
                  }
                />
              </button>
              {selected === c.id && (
                <div className="tree-resources">
                  <div className="database-label">
                    <ChevronDown size={12} />
                    <Database size={13} />
                    {engineCapabilities(c.kind).canSelectDatabase ? (
                      <select
                        aria-label={`${engineCapabilities(c.kind).scopeLabel} for ${c.name}`}
                        value={databaseScopes[c.id] ?? c.database}
                        onChange={(e) => selectScope(c, e.target.value)}
                      >
                        {[
                          ...new Set([
                            databaseScopes[c.id] ?? c.database,
                            ...(databaseLists[c.id] || []),
                          ]),
                        ].map((name) => (
                          <option key={name} value={name}>
                            {name === "*"
                              ? "All namespaces"
                              : name ||
                                `Select ${engineCapabilities(c.kind).scopeLabel.toLowerCase()}`}
                          </option>
                        ))}
                      </select>
                    ) : (
                      <span>{c.database || engines[c.kind].name}</span>
                    )}
                    <span className="resource-count">
                      {resources[c.id]?.length ?? "—"}
                    </span>
                  </div>
                  {databaseErrors[c.id] && (
                    <div className="tree-error">
                      {databaseErrors[c.id]}
                      <button onClick={() => void loadDatabases(c)}>
                        Retry{" "}
                        {engineCapabilities(c.kind).scopeLabel.toLowerCase()}{" "}
                        list
                      </button>
                    </div>
                  )}
                  {c.kind === "redis" && (
                    <form
                      className="key-pattern"
                      onSubmit={(e) => {
                        e.preventDefault();
                        void browse(c);
                      }}
                    >
                      <input
                        aria-label={`Key pattern for ${c.name}`}
                        placeholder="Key pattern: *"
                        value={keyPatterns[c.id] || ""}
                        onChange={(e) =>
                          setKeyPatterns((ps) => ({
                            ...ps,
                            [c.id]: e.target.value,
                          }))
                        }
                      />
                      <button
                        aria-label="Apply key pattern"
                        disabled={loadingId === c.id}
                      >
                        <Search size={12} />
                      </button>
                    </form>
                  )}
                  {loadingId === c.id && (
                    <p className="tree-message">
                      <LoaderCircle className="spin" size={13} />
                      Loading resources…
                    </p>
                  )}
                  {browseErrors[c.id] && (
                    <div className="tree-error">
                      {browseErrors[c.id]}
                      <button onClick={() => void browse(c)}>Try again</button>
                    </div>
                  )}
                  {resources[c.id]?.map((r) => (
                    <div className="resource-entry" key={r.name}>
                      <button
                        className={`resource ${active?.connectionId === c.id && (active.context ?? c.context) === c.context && (active.database ?? c.database) === (databaseScopes[c.id] ?? c.database) && active.title === r.name ? "active" : ""}`}
                        onClick={() => openQuery(c, r)}
                      >
                        {r.kind === "table" ? (
                          <Table2 size={14} />
                        ) : r.kind === "key" ? (
                          <Braces size={14} />
                        ) : (
                          <Boxes size={14} />
                        )}
                        <span>{r.name}</span>
                        {active?.connectionId === c.id &&
                          (active.database ?? c.database) ===
                            (databaseScopes[c.id] ?? c.database) &&
                          active.title === r.name && (
                            <span className="resource-selected" />
                          )}
                      </button>
                      {engineCapabilities(c.kind).canInspect && (
                        <button
                          className="resource-inspect"
                          aria-label={`Inspect ${r.name}`}
                          onClick={() =>
                            setObjectInspector({
                              connection: c,
                              database: databaseScopes[c.id] ?? c.database,
                              resource: r,
                            })
                          }
                        >
                          <CircleHelp size={12} />
                        </button>
                      )}
                    </div>
                  ))}
                  {resources[c.id]?.length === 0 && (
                    <p className="tree-message">No resources found.</p>
                  )}
                  {resourcePages[c.id]?.hasMore && (
                    <button
                      className="load-more-keys"
                      disabled={loadingId === c.id}
                      onClick={() => void browse(c, undefined, true)}
                    >
                      <ChevronDown size={13} />
                      Load more keys
                    </button>
                  )}
                  <button
                    className="new-query-link"
                    onClick={() => openQuery(c)}
                  >
                    <Plus size={13} /> New query
                  </button>
                </div>
              )}
            </div>
          ))}
          {!visibleConnections.length && (
            <div className="sidebar-empty">
              {search
                ? "No matching connections."
                : "Your next connection starts here."}
              <button onClick={() => setConnectionDialog("new")}>
                Add a connection <ArrowRight size={12} />
              </button>
            </div>
          )}
        </div>
        <div className="sidebar-bottom">
          <button
            className="new-connection-button"
            onClick={() => setConnectionDialog("new")}
          >
            <Plus size={16} /> New connection <kbd>⌘ N</kbd>
          </button>
          <div className="workspace-location">
            <ShieldCheck size={13} />
            <span>Stored on this device</span>
          </div>
        </div>
      </aside>
      <div className="main-shell">
        <header className="topbar">
          {view === "workbench" && tabs.length > 0 && connection ? (
            <div className="query-tabs">
              <div className="tabs-scroll">
                {visibleTabs.map((t) => (
                  <div
                    role="tab"
                    aria-selected={t.id === activeId}
                    tabIndex={0}
                    key={t.id}
                    className={`query-tab ${t.id === activeId ? "active" : ""}`}
                    onClick={() => {
                      activateTab(t);
                    }}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") {
                        activateTab(t);
                      }
                    }}
                  >
                    <FileCode2 size={14} />
                    <span>{t.title}</span>
                    {running[t.id] && (
                      <LoaderCircle size={12} className="spin" />
                    )}
                    <button
                      aria-label={`Close ${t.title}`}
                      onClick={(e) => {
                        e.stopPropagation();
                        closeTab(t.id);
                      }}
                    >
                      <X size={12} />
                    </button>
                  </div>
                ))}
              </div>
              <button
                className="icon-button tab-plus"
                aria-label="New query tab"
                onClick={() => openQuery(connection)}
              >
                <Plus size={16} />
              </button>
              <span className="editor-language">
                {connection.kind === "mysql" || connection.kind === "clickhouse"
                  ? "SQL"
                  : engines[connection.kind].name}
              </span>
            </div>
          ) : (
            <div className="breadcrumbs">
              <Layers3 size={16} />
              <span>Workspace</span>
              <ChevronRight size={13} />
              <strong>
                {view === "workbench"
                  ? "Workbench"
                  : view === "connections"
                    ? "Connections"
                    : view === "history"
                      ? "Query history"
                      : "Saved queries"}
              </strong>
            </div>
          )}
          <div className="topbar-actions">
            <button
              className="command-button"
              aria-label="Jump to anything"
              title="Jump to anything (⌘K)"
              onClick={() => {
                setPaletteQuery("");
                setPalette(true);
              }}
            >
              <Search size={14} />
              <span>Jump to anything</span>
              <kbd>⌘ K</kbd>
            </button>
            <span className="topbar-divider" />
            <button
              className={`ai-toggle ${aiOpen ? "selected" : ""}`}
              onClick={() => setAiOpen(!aiOpen)}
            >
              <Sparkles size={15} /> AI assistant
            </button>
          </div>
        </header>
        <div className="content-layout">
          <main className="main-content">
            {view === "workbench" ? (
              <>
                {selectedConnection &&
                (selectedConnection.kind === "k8s" ||
                  selectedConnection.kind === "k3s") &&
                !active ? (
                  <InfraCluster
                    onNode={(resource) =>
                      setClusterInspector({
                        connection: selectedConnection,
                        resource,
                      })
                    }
                    key={`${selectedConnection.id}:${selectedConnection.context}`}
                    connection={selectedConnection}
                    namespace={
                      databaseScopes[selectedConnection.id] ??
                      selectedConnection.database ??
                      "default"
                    }
                    namespaces={
                      databaseLists[selectedConnection.id] || [
                        databaseScopes[selectedConnection.id] ??
                          selectedConnection.database ??
                          "default",
                      ]
                    }
                    onNamespace={(namespace) =>
                      selectScope(selectedConnection, namespace)
                    }
                    onOpen={(item) =>
                      setClusterInspector({
                        connection: selectedConnection,
                        resource: {
                          context: selectedConnection.context,
                          namespace: item.namespace,
                          name: item.name,
                          resource: item.resource,
                          uid: item.uid,
                        },
                      })
                    }
                  />
                ) : active && connection ? (
                  <>
                    <div className="workspace-heading">
                      <div>
                        <div className="connection-heading">
                          <EngineIcon kind={connection.kind} small />
                          <h1>{connection.name}</h1>
                          <span
                            className={`environment ${connection.environment.toLowerCase()}`}
                          >
                            {connection.environment}
                          </span>
                        </div>
                        <p>
                          <span className="status-dot" />
                          {connectionChecks[connection.id]?.state === "ready"
                            ? `Last response at ${connectionChecks[connection.id]?.at}`
                            : connectionChecks[connection.id]?.state === "error"
                              ? "Last request failed"
                              : "Not yet verified"}
                          {connection.ssh?.enabled && (
                            <span
                              className="ssh-route"
                              title={`SSH via ${connection.ssh.host}`}
                            >
                              {" "}
                              · SSH tunnel
                            </span>
                          )}
                          <span className="separator">/</span>
                          {connection.host ||
                            connection.context ||
                            "Kubeconfig"}
                          <span className="separator">/</span>
                          {(connection.kind === "k8s" ||
                            connection.kind === "k3s") &&
                            `${active.context || connection.context || "Current context"} / `}
                          {active.database ?? connection.database ?? "default"}
                        </p>
                      </div>
                      <button
                        className="button subtle"
                        onClick={() => openQuery(connection)}
                      >
                        <Plus size={15} /> New query
                      </button>
                    </div>
                    <div className="query-toolbar">
                      <div className="toolbar-left">
                        <Database size={14} />
                        <span>
                          {(connection.kind === "k8s" ||
                            connection.kind === "k3s") &&
                            `${active.context || connection.context || "Current context"} / `}
                          {active.database ?? connection.database ?? "default"}
                        </span>
                        <span className="toolbar-separator" />
                        <ShieldCheck size={14} />
                        <span className="read-only-label">
                          Read-only session
                        </span>
                        <button
                          className="icon-button"
                          aria-label="Connection capabilities"
                          onClick={() => setCapabilityKind(connection.kind)}
                        >
                          <CircleHelp size={13} />
                        </button>
                      </div>
                      <div>
                        {active.resource &&
                          engineCapabilities(connection.kind).canInspect &&
                          active.resource.kind !== "query" && (
                            <button
                              className="button-icon-label"
                              onClick={() =>
                                setObjectInspector({
                                  connection,
                                  database:
                                    active.database ?? connection.database,
                                  resource: active.resource!,
                                })
                              }
                            >
                              <Table2 size={14} />
                              {connection.kind === "redis"
                                ? "Key details"
                                : "Structure"}
                            </button>
                          )}
                        <button
                          className="button-icon-label"
                          onClick={() => setSnippetTitle(active.title)}
                        >
                          <Bookmark size={14} /> Save
                        </button>
                        <button
                          className="button run-button"
                          onClick={() =>
                            running[active.id]
                              ? void cancel(active.id)
                              : void runQuery()
                          }
                        >
                          {running[active.id] ? (
                            <Square size={11} fill="currentColor" />
                          ) : (
                            <Play size={12} fill="currentColor" />
                          )}
                          {running[active.id] ? "Cancel" : "Run query"}
                          {!running[active.id] && <kbd>⌘ ↵</kbd>}
                        </button>
                      </div>
                    </div>
                    <Editor
                      key={active.id}
                      value={active.query}
                      kind={connection.kind}
                      onChange={(query) => patchTab(active.id, { query })}
                      onRun={() => void runQuery()}
                    />
                    <div className="editor-bottom">
                      <span>
                        <span className="status-dot" />
                        {"Queries time out after 20 seconds"}
                      </span>
                      <span>
                        UTF-8<span className="separator">·</span>
                        {active.query.split("\n").length} lines
                      </span>
                    </div>
                    <section className="results">
                      <div className="results-heading">
                        <div className="result-label">
                          <Table2 size={15} />
                          <strong>Results</strong>
                          <span className="count-badge">
                            {active.result?.rows.length ?? 0}
                          </span>
                          {active.result && (
                            <span className="result-time">
                              <Check size={13} />
                              {active.result.duration} ms
                            </span>
                          )}
                        </div>
                        <div className="results-actions">
                          <button
                            className="icon-button"
                            title="Refresh query"
                            aria-label="Refresh query"
                            disabled={!!running[active.id]}
                            onClick={() => void runQuery()}
                          >
                            <RefreshCw size={14} />
                          </button>
                          <button
                            className="button-icon-label"
                            disabled={!active.result}
                            onClick={() => void exportResults()}
                          >
                            <ArrowDownToLine size={14} /> Export CSV
                          </button>
                        </div>
                      </div>
                      <div className="result-filter">
                        <Search size={14} />
                        <input
                          placeholder="Filter results…"
                          aria-label="Filter results"
                          value={tableFilter}
                          onChange={(e) => {
                            setTableFilter(e.target.value);
                            setPage(0);
                          }}
                        />
                        <span>Double-click a row to inspect</span>
                      </div>
                      {active.error ? (
                        <div className="query-error" role="alert">
                          <Terminal size={25} />
                          <h3>That query didn’t go through.</h3>
                          <pre>{active.error}</pre>
                          <p>
                            Update your query or connection settings and try
                            again.
                          </p>
                        </div>
                      ) : !active.result ? (
                        <div className="empty-state compact">
                          <Play size={26} />
                          <h3>Ready when you are.</h3>
                          <p>Run your query to see the results here.</p>
                          <button
                            className="button primary"
                            onClick={() => void runQuery()}
                          >
                            Run query
                          </button>
                        </div>
                      ) : (
                        <div
                          className={`table-scroll ${running[active.id] ? "is-loading" : ""}`}
                        >
                          <table>
                            <thead>
                              <tr>
                                <th className="row-number">#</th>
                                {active.result.columns.map((column, i) => (
                                  <th key={`${column}-${i}`}>
                                    <button
                                      onClick={() =>
                                        setSort({
                                          column: i,
                                          direction:
                                            sort?.column === i
                                              ? -sort.direction
                                              : 1,
                                        })
                                      }
                                    >
                                      <span className="column-type">
                                        {column === "id" ||
                                        column === "total" ? (
                                          "#"
                                        ) : column.endsWith("_at") ? (
                                          <Clock3 size={11} />
                                        ) : (
                                          "A"
                                        )}
                                      </span>
                                      {column}
                                      <ArrowUpDown size={11} />
                                    </button>
                                  </th>
                                ))}
                              </tr>
                            </thead>
                            <tbody>
                              {rows
                                .slice(page * 50, (page + 1) * 50)
                                .map((row, ri) => (
                                  <tr
                                    key={ri}
                                    onDoubleClick={() => inspectRow(row)}
                                  >
                                    <td className="row-number">
                                      {page * 50 + ri + 1}
                                    </td>
                                    {row.map((cell, ci) => (
                                      <td
                                        key={ci}
                                        className={`${ci === 0 ? "first-column" : ""} ${active.result!.columns[ci] === "email" ? "muted-cell" : ""}`}
                                        title={display(cell)}
                                      >
                                        {[
                                          "Completed",
                                          "Processing",
                                          "Pending",
                                          "Running",
                                          "Active",
                                        ].includes(String(cell)) ? (
                                          <span
                                            className={`cell-status ${String(cell).toLowerCase()}`}
                                          >
                                            <span />
                                            {String(cell)}
                                          </span>
                                        ) : cell == null ? (
                                          <span className="null-value">
                                            NULL
                                          </span>
                                        ) : active.result!.columns[ci] ===
                                          "total" ? (
                                          <span className="amount">
                                            ${Number(cell).toFixed(2)}
                                          </span>
                                        ) : (
                                          display(cell)
                                        )}
                                        {ci === 0 &&
                                          active.result?.resourceType && (
                                            <button
                                              className="resource-detail-button"
                                              aria-label={`Open details for ${String(cell)}`}
                                              onClick={() => inspectRow(row)}
                                            >
                                              <ArrowRight size={13} />
                                            </button>
                                          )}
                                      </td>
                                    ))}
                                  </tr>
                                ))}
                            </tbody>
                          </table>
                          {rows.length === 0 && (
                            <div className="table-empty">
                              {tableFilter
                                ? "No rows match your filter."
                                : "Query completed. No rows returned."}
                            </div>
                          )}
                        </div>
                      )}
                      {connection?.kind === "mongo" &&
                        active.result &&
                        !active.error && (
                          <div className="batch-navigation">
                            <span title={active.result.message}>
                              Batch offset {active.result.offset || 0} ·{" "}
                              {active.result.rows.length}{" "}
                              {active.result.rows.length === 1
                                ? "document"
                                : "documents"}
                            </span>
                            <span className="field-help">
                              {active.query !== active.resultQuery
                                ? "Query changed. Run again to browse batches."
                                : "Concurrent writes can change batches."}
                            </span>
                            <button
                              className="button"
                              disabled={
                                !!running[active.id] ||
                                active.query !== active.resultQuery ||
                                !active.result.previousQuery
                              }
                              onClick={() =>
                                void runQuery(active.result?.previousQuery)
                              }
                            >
                              Previous batch
                            </button>
                            <button
                              className="button"
                              disabled={
                                !!running[active.id] ||
                                active.query !== active.resultQuery ||
                                !active.result.nextQuery
                              }
                              onClick={() =>
                                void runQuery(active.result?.nextQuery)
                              }
                            >
                              Next batch
                            </button>
                          </div>
                        )}
                      <div className="result-footer">
                        <span>
                          {rows.length
                            ? `${page * 50 + 1}–${Math.min((page + 1) * 50, rows.length)} of ${rows.length} rows`
                            : "0 rows"}
                          {active.result?.truncated && (
                            <span className="limit-note">
                              {" "}
                              ·{" "}
                              {active.result.nextQuery
                                ? "More documents available"
                                : active.result.message ||
                                  "Result limit reached"}
                            </span>
                          )}
                        </span>
                        <span className="result-source">
                          {"Up to 500 rows"}
                          <span className="separator">·</span>
                          <ShieldCheck size={12} />
                          Read-only
                        </span>
                        <div>
                          <button
                            className="icon-button"
                            aria-label="Previous page"
                            disabled={page === 0}
                            onClick={() => setPage((p) => p - 1)}
                          >
                            <ChevronLeft size={14} />
                          </button>
                          <span>{page + 1}</span>
                          <button
                            className="icon-button"
                            aria-label="Next page"
                            disabled={(page + 1) * 50 >= rows.length}
                            onClick={() => setPage((p) => p + 1)}
                          >
                            <ChevronRight size={14} />
                          </button>
                        </div>
                      </div>
                    </section>
                  </>
                ) : (
                  <div className="empty-state">
                    <div className="empty-icon">
                      <Database size={35} />
                    </div>
                    <span className="eyebrow">
                      YOUR INFRASTRUCTURE, CONNECTED
                    </span>
                    <h1>还没有打开的连接</h1>
                    <p>
                      {desktop
                        ? "添加连接后即可查询你的中间件或集群。"
                        : "请打开 DevOpsPad 桌面应用来连接你的中间件和集群。"}
                    </p>
                    <button
                      className="button primary"
                      onClick={() => setConnectionDialog("new")}
                    >
                      <Plus size={16} /> New connection
                    </button>
                    {selectedConnection && (
                      <button
                        className="button"
                        onClick={() => openQuery(selectedConnection)}
                      >
                        Open query for {selectedConnection.name}
                      </button>
                    )}
                  </div>
                )}
              </>
            ) : view === "connections" ? (
              <div className="page-view">
                <div className="page-heading">
                  <div>
                    <span className="eyebrow">ALL SYSTEMS, ONE PLACE</span>
                    <h1>Your connections</h1>
                    <p>A clear view of the infrastructure behind your work.</p>
                  </div>
                  <button
                    className="button primary"
                    onClick={() => setConnectionDialog("new")}
                  >
                    <Plus size={15} />
                    New connection
                  </button>
                </div>

                <div className="connection-cards">
                  {allConnections.map((c) => (
                    <article className="connection-card" key={c.id}>
                      <div>
                        <EngineIcon kind={c.kind} />
                        <span
                          className={`environment ${c.environment.toLowerCase()}`}
                        >
                          {c.environment}
                        </span>
                      </div>
                      <h3>{c.name}</h3>
                      <p>
                        {engines[c.kind].name} <span>·</span>{" "}
                        {c.database || "default"}
                      </p>
                      <code>{c.host || c.context || c.kubeconfig}</code>
                      <button
                        className="capability-link"
                        onClick={() => setCapabilityKind(c.kind)}
                      >
                        <CircleHelp size={12} />
                        {engineCapabilities(c.kind).resourceLabel} ·{" "}
                        {engineCapabilities(c.kind).language}
                      </button>
                      <footer>
                        <button
                          className="text-button"
                          onClick={() => {
                            setSelected(c.id);
                            setView("workbench");
                            if (c.kind === "k8s" || c.kind === "k3s")
                              setActiveId("");
                            else openQuery(c);
                          }}
                        >
                          Open workspace <ArrowUpDown size={13} />
                        </button>
                        {
                          <div>
                            <button
                              className="icon-button"
                              aria-label={`Edit ${c.name}`}
                              onClick={() => setConnectionDialog(c)}
                            >
                              <Settings2 size={15} />
                            </button>
                            <button
                              className="icon-button"
                              aria-label={`Delete ${c.name}`}
                              onClick={() => setDeleteTarget(c)}
                            >
                              <Trash2 size={15} />
                            </button>
                          </div>
                        }
                      </footer>
                    </article>
                  ))}
                  <button
                    className="add-connection-card"
                    onClick={() => setConnectionDialog("new")}
                  >
                    <div>
                      <Plus size={21} />
                    </div>
                    <strong>Make a new connection</strong>
                    <span>Databases, caches, streams & clusters</span>
                  </button>
                </div>
              </div>
            ) : view === "history" ? (
              <div className="page-view">
                <div className="page-heading">
                  <div>
                    <span className="eyebrow">PICK UP WHERE YOU LEFT OFF</span>
                    <h1>Query history</h1>
                    <p>Your last 100 queries in this session.</p>
                  </div>
                  <button
                    className="button"
                    disabled={!history.length}
                    onClick={() => setHistory([])}
                  >
                    <Trash2 size={14} />
                    Clear history
                  </button>
                </div>
                {history.length ? (
                  <div className="history-list">
                    {history.map((h) => (
                      <button
                        key={h.id}
                        onClick={() => {
                          const c = allConnections.find(
                            (c) => c.id === h.connectionId,
                          );
                          if (!c) {
                            notify("Switch workspaces to reopen this query.");
                            return;
                          }
                          openQuery(
                            c,
                            {
                              name: `History ${new Date(h.at).toLocaleTimeString("en-US")}`,
                              query: h.query,
                              kind: "query",
                            },
                            h.database,
                            h.context,
                          );
                        }}
                      >
                        <div>
                          <span
                            className={
                              h.error ? "history-error" : "history-check"
                            }
                          >
                            {h.error ? <X size={15} /> : <Check size={15} />}
                          </span>
                          <code>{h.query}</code>
                          <ArrowUpDown size={14} />
                        </div>
                        <footer>
                          <span>{h.name}</span>
                          <span>
                            {h.error
                              ? "Failed"
                              : `${h.rows} rows · ${h.duration} ms`}
                          </span>
                          <time>
                            {new Date(h.at).toLocaleTimeString("en-US")}
                          </time>
                        </footer>
                      </button>
                    ))}
                  </div>
                ) : (
                  <div className="empty-state compact">
                    <History size={30} />
                    <h3>A fresh start.</h3>
                    <p>Run a query and its history will appear here.</p>
                  </div>
                )}
              </div>
            ) : (
              <div className="page-view">
                <div className="page-heading">
                  <div>
                    <span className="eyebrow">THE QUERIES WORTH KEEPING</span>
                    <h1>Saved queries</h1>
                    <p>Your personal collection, stored on this device.</p>
                  </div>
                </div>
                {snippets.length ? (
                  <div className="snippet-list">
                    {snippets.map((s) => (
                      <article key={s.id}>
                        <div>
                          <FileCode2 size={18} />
                          <h3>{s.title}</h3>
                          <span className="tiny-badge">{s.kind}</span>
                          <button
                            className="icon-button"
                            aria-label={`Delete saved query ${s.title}`}
                            onClick={() => {
                              const next = snippets.filter(
                                (x) => x.id !== s.id,
                              );
                              try {
                                localStorage.setItem(
                                  "devopspad-snippets",
                                  JSON.stringify(next),
                                );
                                setSnippets(next);
                              } catch (e) {
                                notify(errorText(e));
                              }
                            }}
                          >
                            <Trash2 size={15} />
                          </button>
                        </div>
                        <pre>{s.query}</pre>
                        <button
                          className="text-button"
                          onClick={() => {
                            const c =
                              allConnections.find(
                                (c) => c.id === selected && c.kind === s.kind,
                              ) ||
                              allConnections.find((c) => c.kind === s.kind);
                            if (c)
                              openQuery(c, {
                                name: s.title,
                                query: s.query,
                                kind: "query",
                              });
                            else
                              notify(
                                `Add a ${s.kind} connection to use this query.`,
                              );
                          }}
                        >
                          Open in editor <ArrowRight size={14} />
                        </button>
                      </article>
                    ))}
                  </div>
                ) : (
                  <div className="empty-state compact">
                    <Bookmark size={30} />
                    <h3>Good queries deserve a home.</h3>
                    <p>
                      Click Save above the editor to keep a query for later.
                    </p>
                  </div>
                )}
              </div>
            )}
          </main>
        </div>
        <footer className="statusbar">
          <div>
            <span className="status-dot" />
            <span>{"Local workspace"}</span>
            <span className="separator">·</span>
            <span>{allConnections.length} connections</span>
          </div>
          <div>
            <span>{desktop ? "Wails desktop" : "Browser preview"}</span>
            <span className="separator">·</span>
            <span>v0.1.0</span>
          </div>
        </footer>
      </div>
      {aiOpen && (
        <Assistant
          key={`${connection?.id || selected}`}
          kind={connection?.kind || selectedConnection?.kind || "mysql"}
          connectionName={
            connection?.name ||
            selectedConnection?.name ||
            "No connection selected"
          }
          activeTitle={active?.title}
          onFocusQuery={() => setView("workbench")}
          names={((active?.context ?? connection?.context) ===
            connection?.context &&
          (active?.database ?? connection?.database) ===
            (databaseScopes[connection?.id || selected] ?? connection?.database)
            ? resources[connection?.id || selected] || []
            : []
          ).map((r) => r.name)}
          onClose={() => setAiOpen(false)}
          onSettings={() => setSettings(true)}
          onInsert={(query) => {
            if (active) {
              patchTab(active.id, { query });
              setView("workbench");
              notify("Suggestion inserted. Review it before running.");
            } else if (selectedConnection)
              openQuery(selectedConnection, {
                name: "AI suggestion",
                kind: "query",
                query,
              });
            else notify("Open a connection first.");
          }}
        />
      )}
      {capabilityKind && (
        <ConnectionCapabilities
          kind={capabilityKind}
          onClose={() => setCapabilityKind(null)}
        />
      )}
      {clusterInspector && (
        <ClusterInspector
          {...clusterInspector}
          onClose={() => setClusterInspector(null)}
        />
      )}
      {objectInspector && (
        <ResourceInspector
          {...objectInspector}
          onClose={() => setObjectInspector(null)}
          onQuery={() => {
            openQuery(
              objectInspector.connection,
              objectInspector.resource,
              objectInspector.database,
            );
            setObjectInspector(null);
          }}
        />
      )}
      {connectionDialog && (
        <ConnectionDialog
          initial={connectionDialog === "new" ? undefined : connectionDialog}
          defaultKind={mode === "infra" ? "k3s" : "mysql"}
          onClose={() => setConnectionDialog(null)}
          onSaved={(c) => {
            setConnections((cs) => [...cs.filter((x) => x.id !== c.id), c]);
            setConnectionDialog(null);
            browseVersions.current[c.id] =
              (browseVersions.current[c.id] || 0) + 1;
            databaseVersions.current[c.id] =
              (databaseVersions.current[c.id] || 0) + 1;
            setDatabaseLists((ls) => {
              const next = { ...ls };
              delete next[c.id];
              return next;
            });
            setDatabaseScopes((ss) => {
              const next = { ...ss };
              delete next[c.id];
              scopeRef.current = next;
              return next;
            });
            setConnectionChecks((cs) => {
              const next = { ...cs };
              delete next[c.id];
              return next;
            });
            setResources((rs) => {
              const next = { ...rs };
              delete next[c.id];
              return next;
            });
            if (c.kind === "k8s" || c.kind === "k3s") {
              setInfraSelected(c.id);
              setInfraEnvironment("");
              setInfraActiveId("");
              window.location.hash = "infra/resources";
            } else {
              setMiddlewareSelected(c.id);
              window.location.hash = "workbench";
            }
            setView("connections");
            notify("Connection saved securely");
          }}
        />
      )}
      {settings && (
        <SettingsDialog
          recovery={recovery}
          onClose={() => setSettings(false)}
        />
      )}
      {recovery.error && (
        <div className="recovery-warning" role="alert">
          <span>{recovery.error}</span>
          <button className="button" onClick={() => setSettings(true)}>
            Recovery settings
          </button>
        </div>
      )}
      {toast && (
        <div className="toast" role="status">
          <Check size={16} />
          <span>{toast}</span>
          <button
            className="icon-button"
            aria-label="Dismiss notification"
            onClick={() => setToast("")}
          >
            <X size={14} />
          </button>
        </div>
      )}
      {inspect && (
        <Modal
          title="Row inspector"
          subtitle="A closer look at your result."
          onClose={() => setInspect(null)}
          wide
        >
          <div className="inspector">
            {inspect.columns.map((c, i) => (
              <div key={i}>
                <span>{c}</span>
                <pre>
                  {typeof inspect.row[i] === "object"
                    ? JSON.stringify(inspect.row[i], null, 2)
                    : display(inspect.row[i])}
                </pre>
              </div>
            ))}
          </div>
          <div className="modal-footer">
            <button
              className="button"
              onClick={async () => {
                try {
                  await navigator.clipboard.writeText(
                    JSON.stringify(
                      Object.fromEntries(
                        inspect.columns.map((c, i) => [c, inspect.row[i]]),
                      ),
                      null,
                      2,
                    ),
                  );
                  notify("Row copied as JSON");
                } catch (e) {
                  notify(errorText(e));
                }
              }}
            >
              <Copy size={14} />
              Copy JSON
            </button>
          </div>
        </Modal>
      )}
      {palette && (
        <Modal title="Jump to anything" onClose={() => setPalette(false)}>
          <div className="palette-search">
            <Search size={18} />
            <input
              autoFocus
              value={paletteQuery}
              onChange={(e) => setPaletteQuery(e.target.value)}
              placeholder="Find a connection…"
              aria-label="Search workspace"
            />
          </div>
          <div className="palette-results">
            {paletteItems.map((c) => (
              <button
                key={c.id}
                onClick={() => {
                  openQuery(c);
                  setPalette(false);
                }}
              >
                <EngineIcon kind={c.kind} small />
                <span>{c.name}</span>
                <small>{engines[c.kind].name}</small>
                <ArrowRight size={14} />
              </button>
            ))}
            {!paletteItems.length && <p>No matching connections.</p>}
            <button
              onClick={() => {
                setPalette(false);
                setConnectionDialog("new");
              }}
            >
              <Plus size={18} />
              <span>Create a new connection</span>
              <kbd>⌘ N</kbd>
            </button>
          </div>
        </Modal>
      )}
      {deleteTarget && (
        <Modal
          title="Delete connection?"
          subtitle={`Remove “${deleteTarget.name}” from your workspace. Remote data is unaffected.`}
          onClose={() => setDeleteTarget(null)}
        >
          <div className="modal-footer">
            <button className="button" onClick={() => setDeleteTarget(null)}>
              Cancel
            </button>
            <button
              className="button danger"
              onClick={async () => {
                try {
                  await api.DeleteConnection(deleteTarget.id);
                  setConnections((cs) =>
                    cs.filter((c) => c.id !== deleteTarget.id),
                  );
                  const next = tabs.filter(
                    (t) => t.connectionId !== deleteTarget.id,
                  );
                  setTabs(next);
                  if (active?.connectionId === deleteTarget.id)
                    setActiveId(next[0]?.id || "");
                  setDeleteTarget(null);
                  notify("Connection deleted");
                } catch (e) {
                  notify(errorText(e));
                }
              }}
            >
              Delete connection
            </button>
          </div>
        </Modal>
      )}
      {snippetTitle !== null && active && (
        <Modal
          title="Save query"
          subtitle="Keep a reusable query in your personal collection."
          onClose={() => setSnippetTitle(null)}
        >
          <form
            onSubmit={(e) => {
              e.preventDefault();
              const next = [
                ...snippets,
                {
                  id: crypto.randomUUID(),
                  title: snippetTitle.trim(),
                  query: active.query,
                  kind: connection?.kind || "mysql",
                },
              ];
              try {
                localStorage.setItem("devopspad-snippets", JSON.stringify(next));
                setSnippets(next);
                setSnippetTitle(null);
                notify("Query saved");
              } catch (e) {
                notify(errorText(e));
              }
            }}
          >
            <div className="form-body">
              <label>
                Query name
                <input
                  autoFocus
                  required
                  value={snippetTitle}
                  onChange={(e) => setSnippetTitle(e.target.value)}
                />
              </label>
            </div>
            <div className="modal-footer">
              <button
                className="button primary"
                disabled={!snippetTitle.trim()}
              >
                Save query
              </button>
            </div>
          </form>
        </Modal>
      )}
      {help && (
        <Modal
          title="A workspace that stays out of your way."
          subtitle="DevOpsPad · early preview"
          onClose={() => setHelp(false)}
        >
          <div className="form-body help-content">
            <h3>Keyboard shortcuts</h3>
            <p>
              Run query <kbd>⌘ / Ctrl + Enter</kbd>
            </p>
            <p>
              Jump to a connection <kbd>⌘ / Ctrl + K</kbd>
            </p>
            <p>
              New connection <kbd>⌘ / Ctrl + N</kbd>
            </p>
            <h3>Explore with confidence</h3>
            <p>Connections and queries use the Wails desktop app.</p>
            <p>
              This release focuses on reading and exploring. SQL runs in a
              read-only session. Redis uses a command allowlist; MongoDB
              supports find queries, Kafka supports metadata, and clusters
              support resource lists.
            </p>
            <p>
              For Redis, use a JSON array of arguments when a key contains
              spaces. Kubernetes queries use <code>get pods namespace</code>.
            </p>
          </div>
        </Modal>
      )}
    </div>
  );
}
