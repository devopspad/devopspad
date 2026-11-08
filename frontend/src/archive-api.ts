import { api } from "./api";

export type ArchiveIssue = {
  file: string;
  line?: number;
  path?: string;
  code: string;
  message: string;
};
export type ArchiveResource = {
  ref: string;
  kind: string;
  name: string;
  title: string;
  file: string;
  labels?: Record<string, string>;
  header: string[];
  row: string[];
  usedBy: string[];
  product?: string;
};
export type ArchiveMatrixRow = {
  product: string;
  title?: string;
  stage: string;
  services: string[];
  domains: string[];
  environments: string[];
  hosts: string[];
};
export type ArchiveRenewal = {
  due: string;
  days: number;
  what: string;
  ref: string;
  autoRenew?: boolean;
  cost?: string;
  usedBy: string[];
};
export type ArchiveDrift = {
  ref: string;
  severity: "error" | "warn" | "info";
  field?: string;
  archive?: string;
  observed?: string;
  message: string;
};
export type ArchiveGitStatus = {
  dir: string;
  branch: string;
  upstream?: string;
  ahead: number;
  behind: number;
  fetched: boolean;
  changes: string[];
};
export type ArchiveSummary = {
  dir: string;
  name: string;
  errors: ArchiveIssue[];
  warnings: ArchiveIssue[];
  resources: ArchiveResource[];
  matrix: ArchiveMatrixRow[];
  renewals: ArchiveRenewal[];
  drift: ArchiveDrift[];
  git?: ArchiveGitStatus;
  gitError?: string;
};
export type ArchiveRefUse = { from: string; field: string; to: string; part?: string; file: string };
export type ArchiveRecord = {
  id: string;
  action: string;
  status: string;
  message?: string;
  startedAt: string;
  actor: { type: string; name: string };
  targets: string[];
};
export type ArchiveDeployment = {
  environment: string;
  runtime: string;
  cluster?: string;
  namespace?: string;
  selector?: string;
  workload?: { kind: string; name: string };
  hosts?: string[];
  unit?: string;
  provider?: string;
};
export type ArchiveDetail = {
  description: {
    ref: string;
    kind: string;
    file: string;
    deployments?: ArchiveDeployment[];
    releaseNotes?: string;
    references: ArchiveRefUse[];
    referencedBy: ArchiveRefUse[];
    usedByProducts: string[];
    secrets: { field: string; ref: string }[];
    notes: string[];
    recentRecords: ArchiveRecord[];
  };
  text: string;
  revision: string;
  observation?: { observedAt: string } & Record<string, unknown>;
};
export type ArchiveSyncItem = { ref: string; name: string; action: string; reason?: string };
export type ArchiveScanResult = { observations: unknown[]; unregisteredZones: string[]; notes: string[] };

export type ArchiveBridge = {
  GetArchive(): Promise<ArchiveSummary>;
  SelectArchive(create: boolean): Promise<ArchiveSummary>;
  DescribeArchiveResource(ref: string): Promise<ArchiveDetail>;
  SaveArchiveFile(file: string, text: string, revision: string): Promise<{ revision: string; issues: ArchiveIssue[] }>;
  NewArchiveResource(kind: string, name: string, product: string): Promise<string>;
  ArchiveGit(action: string, message: string): Promise<string>;
  ScanArchive(kind: string): Promise<ArchiveScanResult>;
  SyncArchiveConnections(dryRun: boolean): Promise<ArchiveSyncItem[]>;
};

// The shared proxy forwards any method name to the Wails bridge.
export const archiveApi = api as unknown as ArchiveBridge;
