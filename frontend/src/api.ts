import type { OpsBridge } from "./ops-api";
import type {
  Connection,
  ClusterRef,
  ClusterDetails,
  PodLogRequest,
  PodLogs,
  KubeconfigInfo,
  Resource,
  QueryResult,
  Workload,
  AISettings,
  ResourcePage,
  ResourceDetails,
  BrowseRequest,
} from "./types";
type Bridge = OpsBridge & {
  SaveConfigMap(id: string, request: { ref: ClusterRef; resourceVersion: string; data: Record<string, string> }): Promise<string>;
  InspectClusterResource(id: string, ref: ClusterRef): Promise<ClusterDetails>;
  ClusterEvents(id: string, ref: ClusterRef): Promise<QueryResult>;
  ReadPodLogs(id: string, request: PodLogRequest): Promise<PodLogs>;
  ListConnections(): Promise<Connection[]>;
  SaveConnection(c: Connection): Promise<Connection>;
  DeleteConnection(id: string): Promise<void>;
  TestConnection(c: Connection): Promise<number>;
  Browse(id: string): Promise<Resource[]>;
  BrowseResources(id: string, request: BrowseRequest): Promise<ResourcePage>;
  ListDatabases(id: string): Promise<string[]>;
  ListWorkloads(id: string, namespace: string): Promise<Workload[]>;
  InspectResource(
    id: string,
    database: string,
    name: string,
  ): Promise<ResourceDetails>;
  ExecuteScoped(
    id: string,
    database: string,
    query: string,
    requestID: string,
  ): Promise<QueryResult>;
  Execute(id: string, query: string, requestID: string): Promise<QueryResult>;
  CancelQuery(id: string): Promise<void>;
  GetAISettings(): Promise<AISettings>;
  SaveAISettings(c: AISettings): Promise<void>;
  AskAI(r: { prompt: string; kind: string; schema: string[] }): Promise<string>;
  SelectKubeconfig(): Promise<string>;
  SelectSSHFile(kind: "privateKey" | "knownHosts"): Promise<string>;
  ReadKubeconfig(path: string): Promise<KubeconfigInfo>;
  ExecuteInContext(
    id: string,
    database: string,
    context: string,
    query: string,
    requestID: string,
  ): Promise<QueryResult>;
  ExportCSV(content: string): Promise<boolean>;
};
declare global {
  interface Window {
    go?: { main?: { App?: Bridge } };
  }
}
export const desktop = Boolean(window.go?.main?.App);
export const api = new Proxy({} as Bridge, {
  get:
    (_, key: keyof Bridge) =>
    (...args: unknown[]) => {
      const bridge = window.go?.main?.App;
      if (!bridge)
        return Promise.reject(
          new Error(
            "Open the Wails desktop app to use live connections and AI settings.",
          ),
        );
      if (typeof bridge[key] !== "function")
        return Promise.reject(new Error(`Desktop operation ${String(key)} is unavailable. Rebuild or update the desktop app.`));
      return Promise.resolve().then(() => (bridge[key] as (...args: unknown[]) => unknown)(...args));
    },
});
export function errorText(e: unknown) {
  return e instanceof Error ? e.message : String(e);
}
export function display(value: unknown): string {
  return value == null
    ? "NULL"
    : typeof value === "object"
      ? JSON.stringify(value)
      : String(value);
}
export function toCSV(result: QueryResult) {
  const escape = (v: unknown) => {
    let text = v == null ? "" : display(v);
    if (/^[=+\-@\t\r]/.test(text)) text = `'${text}`;
    return `"${text.replaceAll('"', '""')}"`;
  };
  return [result.columns, ...result.rows]
    .map((row) => row.map(escape).join(","))
    .join("\r\n");
}
