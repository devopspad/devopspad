import type { ContainerInfo, Workload } from "./types";
export type OpsRun = {
  apiVersion: string;
  id: string;
  kind: "build" | "release";
  application: string;
  version: string;
  actor: { kind: string; name: string; sessionId: string };
  source: {
    repository: string;
    commit: string;
    branch: string;
    dirty: boolean;
  };
  recipe: string;
  target?: {
    environment: string;
    workload: string;
    container: string;
    buildId: string;
  };
  connection?: string;
  namespace?: string;
  status: string;
  image?: string;
  startedAt: string;
  finishedAt?: string;
  message: string;
  events: {
    at: string;
    event: {
      id: string;
      type: string;
      step?: {
        name: string;
        status: string;
        message: string;
        logPath?: string;
      };
      finish?: { status: string; message: string };
    };
  }[];
};
export type OpsEnvironment = {
  name: string;
  connection: string;
  namespace: string;
};
export type OpsApplication = { name: string; selector: string; nginxPath?: string };
export type OpsNginxConfig = { path: string; text: string; revision: string; message?: string };
export type OpsProject = {
  apiVersion: string;
  name: string;
  environments: OpsEnvironment[];
  applications: OpsApplication[];
};
export type OpsArtifact = {
  runId?: string;
  id: string;
  application: string;
  version: string;
  image: string;
  source: string;
  createdAt: string;
};
export type OpsRecord = {
  origin?: "external";
  runId?: string;
  action?: "release" | "rollback" | "scale" | "restart" | "nginx" | "config";
  beforeReplicas?: number;
  replicas?: number;
  id: string;
  application: string;
  environment: string;
  connection: string;
  namespace: string;
  workload: string;
  container: string;
  image: string;
  beforeImage: string;
  version: string;
  status: string;
  startedAt: string;
  finishedAt: string;
  message: string;
  steps: string[];
};
export type OpsSnapshot = {
  runs?: OpsRun[];
  root: string;
  yaml: string;
  revision: string;
  project: OpsProject;
  artifacts: OpsArtifact[];
  releases: OpsRecord[];
};
export type OpsPod = {
  name: string;
  namespace: string;
  uid: string;
  workload: string;
  ip: string;
  node: string;
  nodeIP: string;
  phase: string;
  ready: number;
  total: number;
  restarts: number;
  createdAt: string;
  ports: string[];
  containers: ContainerInfo[];
};
export type OpsCluster = {
  workloads: Workload[];
  pods: OpsPod[];
  fetchedAt: string;
  truncated: boolean;
  warnings: string[];
};
export type OpsMetric = {
  pod: string;
  namespace: string;
  cpu: number;
  memory: number;
  timestamp: string;
};
export type OpsPlan = {
  action?: "release" | "rollback" | "scale" | "restart";
  beforeReplicas?: number;
  replicas?: number;
  id: string;
  namespace: string;
  workload: string;
  container: string;
  beforeImage: string;
  image: string;
  expiresAt: string;
};
export type OpsBridge = {
  ReadOpsNginx(environment: string, application: string): Promise<OpsNginxConfig>;
  SaveOpsNginx(environment: string, application: string, update: { revision: string; text: string }): Promise<OpsNginxConfig>;
  GetOpsProject(): Promise<OpsSnapshot>;
  AttachOpsWorkload(request: {
    connectionId: string;
    namespace: string;
    workload: string;
    uid: string;
    resource: string;
    environment: string;
    application: string;
    revision: string;
  }): Promise<OpsSnapshot>;
  SaveOpsProject(text: string, revision: string): Promise<OpsSnapshot>;
  SelectOpsProject(): Promise<OpsSnapshot>;
  AddOpsArtifact(artifact: OpsArtifact): Promise<OpsSnapshot>;
  OpsGit(action: string): Promise<string>;
  ReadOpsCluster(
    id: string,
    namespace: string,
    selector: string,
  ): Promise<OpsCluster>;
  ReadOpsMetrics(
    id: string,
    namespace: string,
    selector: string,
  ): Promise<OpsMetric[]>;
  ReadOpsWorkloads(
    id: string,
    namespace: string,
    selector: string,
  ): Promise<OpsCluster>;
  PlanOpsRelease(
    environment: string,
    application: string,
    workload: string,
    container: string,
    artifact: string,
  ): Promise<OpsPlan>;
  ApplyOpsRelease(plan: string): Promise<OpsRecord>;
  PlanOpsWorkloadAction(
    environment: string,
    application: string,
    workload: string,
    action: string,
    replicas: number,
  ): Promise<OpsPlan>;
  PlanOpsRollback(
    environment: string,
    application: string,
    record: string,
  ): Promise<OpsPlan>;
};
