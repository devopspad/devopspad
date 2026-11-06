export type Kind =
  "mysql" | "redis" | "mongo" | "kafka" | "clickhouse" | "k8s" | "k3s";
export type SSHConfig = {
  enabled: boolean;
  host: string;
  port: number;
  username: string;
  authMethod: "password" | "key" | "agent";
  password?: string;
  privateKeyPath: string;
  passphrase?: string;
  knownHostsPath: string;
  hostKeyFingerprint: string;
  hasPassword: boolean;
  hasPassphrase: boolean;
};
export type Connection = {
  ssh?: SSHConfig;
  id: string;
  name: string;
  kind: Kind;
  host: string;
  port: number;
  username: string;
  password?: string;
  authSource?: string;
  database: string;
  environment: string;
  tls: boolean;
  kubeconfig: string;
  context: string;
  hasPassword: boolean;
};
export type Resource = { name: string; kind: string; query: string };
export type Workload = {
  kind: string;
  namespace: string;
  name: string;
  uid: string;
  resource: string;
  desired: number;
  ready: number;
  image: string;
  extraImages: number;
  updatedAt: string;
  selector: string;
};
export type QueryResult = {
  resourceType?: string;
  columns: string[];
  rows: unknown[][];
  duration: number;
  truncated: boolean;
  message: string;
  nextQuery?: string;
  previousQuery?: string;
  offset?: number;
};
export type AISettings = {
  endpoint: string;
  model: string;
  apiKey?: string;
  hasKey: boolean;
};
export type Tab = {
  id: string;
  connectionId: string;
  title: string;
  database?: string;
  context?: string;
  resource?: Resource;
  query: string;
  result?: QueryResult;
  resultQuery?: string;
  error?: string;
};
export type HistoryItem = {
  id: string;
  connectionId: string;
  database?: string;
  context?: string;
  name: string;
  query: string;
  at: string;
  rows: number;
  duration: number;
  error?: string;
};
export const engines: Record<
  Kind,
  {
    name: string;
    color: string;
    short: string;
    port: number;
    description: string;
  }
> = {
  mysql: {
    name: "MySQL",
    color: "#287bb7",
    short: "My",
    port: 3306,
    description: "Relational database",
  },
  redis: {
    name: "Redis",
    color: "#d95b54",
    short: "Re",
    port: 6379,
    description: "In-memory data store",
  },
  mongo: {
    name: "MongoDB",
    color: "#409870",
    short: "Mo",
    port: 27017,
    description: "Document database",
  },
  kafka: {
    name: "Kafka",
    color: "#8b68c4",
    short: "Ka",
    port: 9092,
    description: "Event streaming",
  },
  clickhouse: {
    name: "ClickHouse",
    color: "#ba941a",
    short: "Ch",
    port: 8123,
    description: "Analytics database",
  },
  k8s: {
    name: "Kubernetes",
    color: "#527de1",
    short: "K8",
    port: 6443,
    description: "Container orchestration",
  },
  k3s: {
    name: "K3s",
    color: "#de883e",
    short: "K3",
    port: 6443,
    description: "Lightweight Kubernetes",
  },
};
export function defaultQuery(kind: Kind): string {
  return {
    mysql: "SHOW TABLES;",
    clickhouse: "SHOW TABLES;",
    redis: "SCAN 0 COUNT 100",
    mongo: '{\n  "collection": "orders",\n  "filter": {},\n  "limit": 100\n}',
    kafka: "topics",
    k8s: "get pods",
    k3s: "get pods",
  }[kind];
}

export type ResourcePage = {
  resources: Resource[];
  nextCursor: string;
  hasMore: boolean;
  message: string;
};
export type ResourceDetails = {
  name: string;
  database: string;
  kind: string;
  properties: { name: string; value: string }[];
  columns: QueryResult;
  indexes: QueryResult;
  preview: QueryResult;
  definition: string;
};
export type BrowseRequest = {
  database: string;
  cursor: string;
  filter: string;
};

export type KubeconfigInfo = {
  currentContext: string;
  contexts: {
    name: string;
    cluster: string;
    namespace: string;
    execAuth: boolean;
  }[];
};

export type ClusterRef = {
  context: string;
  namespace: string;
  resource: string;
  name: string;
  uid: string;
};
export type ContainerInfo = {
  name: string;
  kind: string;
  image: string;
  state: string;
  ready: boolean;
  restarts: number;
};
export type ClusterDetails = {
  ref: ClusterRef;
  properties: { name: string; value: string }[];
  labels: Record<string, string>;
  containers: ContainerInfo[];
  definition: string;
};
export type PodLogRequest = {
  ref: ClusterRef;
  container: string;
  tailLines: number;
  previous: boolean;
};
export type PodLogs = { text: string; truncated: boolean };
