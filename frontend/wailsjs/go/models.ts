export namespace archive {
	
	export class Links {
	    run?: string;
	    artifact?: string;
	    commit?: string;
	
	    static createFrom(source: any = {}) {
	        return new Links(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.run = source["run"];
	        this.artifact = source["artifact"];
	        this.commit = source["commit"];
	    }
	}
	export class Step {
	    name: string;
	    status: string;
	    at?: string;
	    message?: string;
	
	    static createFrom(source: any = {}) {
	        return new Step(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.status = source["status"];
	        this.at = source["at"];
	        this.message = source["message"];
	    }
	}
	export class PlanSummary {
	    digest?: string;
	    summary?: string;
	
	    static createFrom(source: any = {}) {
	        return new PlanSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.digest = source["digest"];
	        this.summary = source["summary"];
	    }
	}
	export class Actor {
	    type: string;
	    name: string;
	    session?: string;
	    machine?: string;
	    at?: string;
	
	    static createFrom(source: any = {}) {
	        return new Actor(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.name = source["name"];
	        this.session = source["session"];
	        this.machine = source["machine"];
	        this.at = source["at"];
	    }
	}
	export class ActionRecord {
	    apiVersion: string;
	    kind: string;
	    id: string;
	    action: string;
	    targets: string[];
	    actor: Actor;
	    approver?: Actor;
	    archiveRevision?: string;
	    plan?: PlanSummary;
	    status: string;
	    message?: string;
	    steps: Step[];
	    startedAt: string;
	    finishedAt?: string;
	    links?: Links;
	
	    static createFrom(source: any = {}) {
	        return new ActionRecord(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.apiVersion = source["apiVersion"];
	        this.kind = source["kind"];
	        this.id = source["id"];
	        this.action = source["action"];
	        this.targets = source["targets"];
	        this.actor = this.convertValues(source["actor"], Actor);
	        this.approver = this.convertValues(source["approver"], Actor);
	        this.archiveRevision = source["archiveRevision"];
	        this.plan = this.convertValues(source["plan"], PlanSummary);
	        this.status = source["status"];
	        this.message = source["message"];
	        this.steps = this.convertValues(source["steps"], Step);
	        this.startedAt = source["startedAt"];
	        this.finishedAt = source["finishedAt"];
	        this.links = this.convertValues(source["links"], Links);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class ObservedWorkload {
	    kind: string;
	    namespace: string;
	    name: string;
	    labels?: Record<string, string>;
	    templateLabels?: Record<string, string>;
	    images: string[];
	    desired: number;
	    ready: number;
	
	    static createFrom(source: any = {}) {
	        return new ObservedWorkload(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.labels = source["labels"];
	        this.templateLabels = source["templateLabels"];
	        this.images = source["images"];
	        this.desired = source["desired"];
	        this.ready = source["ready"];
	    }
	}
	export class ObservedNode {
	    name: string;
	    internalIP?: string;
	    version?: string;
	    ready: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ObservedNode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.internalIP = source["internalIP"];
	        this.version = source["version"];
	        this.ready = source["ready"];
	    }
	}
	export class ClusterFacts {
	    nodes: ObservedNode[];
	    workloads: ObservedWorkload[];
	    truncated?: boolean;
	    errors: string[];
	
	    static createFrom(source: any = {}) {
	        return new ClusterFacts(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.nodes = this.convertValues(source["nodes"], ObservedNode);
	        this.workloads = this.convertValues(source["workloads"], ObservedWorkload);
	        this.truncated = source["truncated"];
	        this.errors = source["errors"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SecretUse {
	    from: string;
	    field: string;
	    ref: string;
	    file: string;
	
	    static createFrom(source: any = {}) {
	        return new SecretUse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.from = source["from"];
	        this.field = source["field"];
	        this.ref = source["ref"];
	        this.file = source["file"];
	    }
	}
	export class RefUse {
	    from: string;
	    field: string;
	    to: string;
	    part?: string;
	    file: string;
	
	    static createFrom(source: any = {}) {
	        return new RefUse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.from = source["from"];
	        this.field = source["field"];
	        this.to = source["to"];
	        this.part = source["part"];
	        this.file = source["file"];
	    }
	}
	export class Workload {
	    kind: string;
	    name: string;
	    container?: string;
	
	    static createFrom(source: any = {}) {
	        return new Workload(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.container = source["container"];
	    }
	}
	export class ResolvedDeployment {
	    environment: string;
	    runtime: string;
	    cluster?: string;
	    namespace?: string;
	    selector?: string;
	    workload?: Workload;
	    hosts?: string[];
	    unit?: string;
	    provider?: string;
	
	    static createFrom(source: any = {}) {
	        return new ResolvedDeployment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.environment = source["environment"];
	        this.runtime = source["runtime"];
	        this.cluster = source["cluster"];
	        this.namespace = source["namespace"];
	        this.selector = source["selector"];
	        this.workload = this.convertValues(source["workload"], Workload);
	        this.hosts = source["hosts"];
	        this.unit = source["unit"];
	        this.provider = source["provider"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Description {
	    ref: string;
	    kind: string;
	    file: string;
	    object: any;
	    deployments?: ResolvedDeployment[];
	    releaseNotes?: string;
	    references: RefUse[];
	    referencedBy: RefUse[];
	    usedByProducts: string[];
	    secrets: SecretUse[];
	    notes: string[];
	    recentRecords: ActionRecord[];
	
	    static createFrom(source: any = {}) {
	        return new Description(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ref = source["ref"];
	        this.kind = source["kind"];
	        this.file = source["file"];
	        this.object = source["object"];
	        this.deployments = this.convertValues(source["deployments"], ResolvedDeployment);
	        this.releaseNotes = source["releaseNotes"];
	        this.references = this.convertValues(source["references"], RefUse);
	        this.referencedBy = this.convertValues(source["referencedBy"], RefUse);
	        this.usedByProducts = source["usedByProducts"];
	        this.secrets = this.convertValues(source["secrets"], SecretUse);
	        this.notes = source["notes"];
	        this.recentRecords = this.convertValues(source["recentRecords"], ActionRecord);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ObservedCert {
	    host: string;
	    notAfter?: string;
	    issuer?: string;
	    valid: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ObservedCert(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.notAfter = source["notAfter"];
	        this.issuer = source["issuer"];
	        this.valid = source["valid"];
	        this.error = source["error"];
	    }
	}
	export class ObservedRecord {
	    name: string;
	    type: string;
	    content: string;
	    proxied?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ObservedRecord(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.content = source["content"];
	        this.proxied = source["proxied"];
	    }
	}
	export class WorkerRoute {
	    id: string;
	    pattern: string;
	    script?: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkerRoute(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.pattern = source["pattern"];
	        this.script = source["script"];
	    }
	}
	export class DomainFacts {
	    expiresAt?: string;
	    expirySource?: string;
	    autoRenew?: boolean;
	    registrar?: string;
	    dns?: string;
	    zoneStatus?: string;
	    nameServers?: string[];
	    workerRoutes?: WorkerRoute[];
	    records: ObservedRecord[];
	    certificates: ObservedCert[];
	    errors: string[];
	
	    static createFrom(source: any = {}) {
	        return new DomainFacts(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.expiresAt = source["expiresAt"];
	        this.expirySource = source["expirySource"];
	        this.autoRenew = source["autoRenew"];
	        this.registrar = source["registrar"];
	        this.dns = source["dns"];
	        this.zoneStatus = source["zoneStatus"];
	        this.nameServers = source["nameServers"];
	        this.workerRoutes = this.convertValues(source["workerRoutes"], WorkerRoute);
	        this.records = this.convertValues(source["records"], ObservedRecord);
	        this.certificates = this.convertValues(source["certificates"], ObservedCert);
	        this.errors = source["errors"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Drift {
	    ref: string;
	    severity: string;
	    field?: string;
	    archive?: string;
	    observed?: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new Drift(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ref = source["ref"];
	        this.severity = source["severity"];
	        this.field = source["field"];
	        this.archive = source["archive"];
	        this.observed = source["observed"];
	        this.message = source["message"];
	    }
	}
	export class ObservedFile {
	    path: string;
	    sha256: string;
	
	    static createFrom(source: any = {}) {
	        return new ObservedFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.sha256 = source["sha256"];
	    }
	}
	export class HostFacts {
	    os?: string;
	    kernel?: string;
	    cpu?: number;
	    memoryGiB?: number;
	    diskGiB?: number;
	    services: string[];
	    nginxConfs: ObservedFile[];
	    k3s?: string;
	    errors: string[];
	
	    static createFrom(source: any = {}) {
	        return new HostFacts(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.os = source["os"];
	        this.kernel = source["kernel"];
	        this.cpu = source["cpu"];
	        this.memoryGiB = source["memoryGiB"];
	        this.diskGiB = source["diskGiB"];
	        this.services = source["services"];
	        this.nginxConfs = this.convertValues(source["nginxConfs"], ObservedFile);
	        this.k3s = source["k3s"];
	        this.errors = source["errors"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Issue {
	    file: string;
	    line?: number;
	    path?: string;
	    code: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new Issue(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.file = source["file"];
	        this.line = source["line"];
	        this.path = source["path"];
	        this.code = source["code"];
	        this.message = source["message"];
	    }
	}
	
	export class MatrixRow {
	    product: string;
	    title?: string;
	    stage: string;
	    services: string[];
	    domains: string[];
	    environments: string[];
	    hosts: string[];
	
	    static createFrom(source: any = {}) {
	        return new MatrixRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.product = source["product"];
	        this.title = source["title"];
	        this.stage = source["stage"];
	        this.services = source["services"];
	        this.domains = source["domains"];
	        this.environments = source["environments"];
	        this.hosts = source["hosts"];
	    }
	}
	export class Observation {
	    apiVersion: string;
	    kind: string;
	    ref: string;
	    observedAt: string;
	    collector: string;
	    domain?: DomainFacts;
	    cluster?: ClusterFacts;
	    host?: HostFacts;
	
	    static createFrom(source: any = {}) {
	        return new Observation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.apiVersion = source["apiVersion"];
	        this.kind = source["kind"];
	        this.ref = source["ref"];
	        this.observedAt = source["observedAt"];
	        this.collector = source["collector"];
	        this.domain = this.convertValues(source["domain"], DomainFacts);
	        this.cluster = this.convertValues(source["cluster"], ClusterFacts);
	        this.host = this.convertValues(source["host"], HostFacts);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	
	
	
	
	export class RenewalRow {
	    due: string;
	    days: number;
	    what: string;
	    ref: string;
	    autoRenew?: boolean;
	    cost?: string;
	    usedBy: string[];
	
	    static createFrom(source: any = {}) {
	        return new RenewalRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.due = source["due"];
	        this.days = source["days"];
	        this.what = source["what"];
	        this.ref = source["ref"];
	        this.autoRenew = source["autoRenew"];
	        this.cost = source["cost"];
	        this.usedBy = source["usedBy"];
	    }
	}
	
	export class ScanResult {
	    observations: Observation[];
	    unregisteredZones: string[];
	    notes: string[];
	
	    static createFrom(source: any = {}) {
	        return new ScanResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.observations = this.convertValues(source["observations"], Observation);
	        this.unregisteredZones = source["unregisteredZones"];
	        this.notes = source["notes"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class Status {
	    dir: string;
	    branch: string;
	    upstream?: string;
	    ahead: number;
	    behind: number;
	    fetched: boolean;
	    changes: string[];
	    errors: number;
	    warnings: number;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dir = source["dir"];
	        this.branch = source["branch"];
	        this.upstream = source["upstream"];
	        this.ahead = source["ahead"];
	        this.behind = source["behind"];
	        this.fetched = source["fetched"];
	        this.changes = source["changes"];
	        this.errors = source["errors"];
	        this.warnings = source["warnings"];
	    }
	}
	
	export class SyncItem {
	    ref: string;
	    name: string;
	    action: string;
	    reason?: string;
	
	    static createFrom(source: any = {}) {
	        return new SyncItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ref = source["ref"];
	        this.name = source["name"];
	        this.action = source["action"];
	        this.reason = source["reason"];
	    }
	}
	

}

export namespace main {
	
	export class ArchiveDetail {
	    description?: archive.Description;
	    text: string;
	    revision: string;
	    observation?: archive.Observation;
	
	    static createFrom(source: any = {}) {
	        return new ArchiveDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.description = this.convertValues(source["description"], archive.Description);
	        this.text = source["text"];
	        this.revision = source["revision"];
	        this.observation = this.convertValues(source["observation"], archive.Observation);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ArchiveResource {
	    ref: string;
	    kind: string;
	    name: string;
	    title: string;
	    file: string;
	    labels: Record<string, string>;
	    header: string[];
	    row: string[];
	    usedBy: string[];
	    product?: string;
	
	    static createFrom(source: any = {}) {
	        return new ArchiveResource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ref = source["ref"];
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.title = source["title"];
	        this.file = source["file"];
	        this.labels = source["labels"];
	        this.header = source["header"];
	        this.row = source["row"];
	        this.usedBy = source["usedBy"];
	        this.product = source["product"];
	    }
	}
	export class ArchiveSaveResult {
	    revision: string;
	    issues: archive.Issue[];
	
	    static createFrom(source: any = {}) {
	        return new ArchiveSaveResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.revision = source["revision"];
	        this.issues = this.convertValues(source["issues"], archive.Issue);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ArchiveSummary {
	    dir: string;
	    name: string;
	    errors: archive.Issue[];
	    warnings: archive.Issue[];
	    resources: ArchiveResource[];
	    matrix: archive.MatrixRow[];
	    renewals: archive.RenewalRow[];
	    drift: archive.Drift[];
	    git?: archive.Status;
	    gitError?: string;
	
	    static createFrom(source: any = {}) {
	        return new ArchiveSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dir = source["dir"];
	        this.name = source["name"];
	        this.errors = this.convertValues(source["errors"], archive.Issue);
	        this.warnings = this.convertValues(source["warnings"], archive.Issue);
	        this.resources = this.convertValues(source["resources"], ArchiveResource);
	        this.matrix = this.convertValues(source["matrix"], archive.MatrixRow);
	        this.renewals = this.convertValues(source["renewals"], archive.RenewalRow);
	        this.drift = this.convertValues(source["drift"], archive.Drift);
	        this.git = this.convertValues(source["git"], archive.Status);
	        this.gitError = source["gitError"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class OpsAttachRequest {
	    connectionId: string;
	    namespace: string;
	    workload: string;
	    uid: string;
	    resource: string;
	    environment: string;
	    application: string;
	    revision: string;
	
	    static createFrom(source: any = {}) {
	        return new OpsAttachRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connectionId = source["connectionId"];
	        this.namespace = source["namespace"];
	        this.workload = source["workload"];
	        this.uid = source["uid"];
	        this.resource = source["resource"];
	        this.environment = source["environment"];
	        this.application = source["application"];
	        this.revision = source["revision"];
	    }
	}
	export class OpsSecretRequest {
	    name: string;
	    type: string;
	    data: Record<string, string>;
	    server: string;
	    username: string;
	    password: string;
	
	    static createFrom(source: any = {}) {
	        return new OpsSecretRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.data = source["data"];
	        this.server = source["server"];
	        this.username = source["username"];
	        this.password = source["password"];
	    }
	}
	export class UpdateInfo {
	    current: string;
	    latest: string;
	    available: boolean;
	    url: string;
	    downloadUrl: string;
	    notes: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.current = source["current"];
	        this.latest = source["latest"];
	        this.available = source["available"];
	        this.url = source["url"];
	        this.downloadUrl = source["downloadUrl"];
	        this.notes = source["notes"];
	    }
	}

}

export namespace ops {
	
	export class Application {
	    name: string;
	    selector: string;
	    nginxPath?: string;
	
	    static createFrom(source: any = {}) {
	        return new Application(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.selector = source["selector"];
	        this.nginxPath = source["nginxPath"];
	    }
	}
	export class Artifact {
	    runId?: string;
	    id: string;
	    application: string;
	    version: string;
	    image: string;
	    source: string;
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Artifact(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.runId = source["runId"];
	        this.id = source["id"];
	        this.application = source["application"];
	        this.version = source["version"];
	        this.image = source["image"];
	        this.source = source["source"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class Environment {
	    name: string;
	    connection: string;
	    namespace: string;
	
	    static createFrom(source: any = {}) {
	        return new Environment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.connection = source["connection"];
	        this.namespace = source["namespace"];
	    }
	}
	export class Project {
	    apiVersion: string;
	    name: string;
	    environments: Environment[];
	    applications: Application[];
	
	    static createFrom(source: any = {}) {
	        return new Project(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.apiVersion = source["apiVersion"];
	        this.name = source["name"];
	        this.environments = this.convertValues(source["environments"], Environment);
	        this.applications = this.convertValues(source["applications"], Application);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RecordTarget {
	    application: string;
	    workload: string;
	    container: string;
	    beforeImage: string;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new RecordTarget(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.application = source["application"];
	        this.workload = source["workload"];
	        this.container = source["container"];
	        this.beforeImage = source["beforeImage"];
	        this.status = source["status"];
	    }
	}
	export class Record {
	    runId?: string;
	    action?: string;
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
	    targets?: RecordTarget[];
	    configMap?: string;
	    key?: string;
	
	    static createFrom(source: any = {}) {
	        return new Record(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.runId = source["runId"];
	        this.action = source["action"];
	        this.beforeReplicas = source["beforeReplicas"];
	        this.replicas = source["replicas"];
	        this.id = source["id"];
	        this.application = source["application"];
	        this.environment = source["environment"];
	        this.connection = source["connection"];
	        this.namespace = source["namespace"];
	        this.workload = source["workload"];
	        this.container = source["container"];
	        this.image = source["image"];
	        this.beforeImage = source["beforeImage"];
	        this.version = source["version"];
	        this.status = source["status"];
	        this.startedAt = source["startedAt"];
	        this.finishedAt = source["finishedAt"];
	        this.message = source["message"];
	        this.steps = source["steps"];
	        this.targets = this.convertValues(source["targets"], RecordTarget);
	        this.configMap = source["configMap"];
	        this.key = source["key"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class RunFinish {
	    status: string;
	    image?: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new RunFinish(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.image = source["image"];
	        this.message = source["message"];
	    }
	}
	export class RunStep {
	    name: string;
	    status: string;
	    message: string;
	    logPath?: string;
	
	    static createFrom(source: any = {}) {
	        return new RunStep(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.status = source["status"];
	        this.message = source["message"];
	        this.logPath = source["logPath"];
	    }
	}
	export class RunStart {
	    kind: string;
	    application: string;
	    version: string;
	    actor: RunActor;
	    source: RunSource;
	    recipe: string;
	    target?: RunTarget;
	
	    static createFrom(source: any = {}) {
	        return new RunStart(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.application = source["application"];
	        this.version = source["version"];
	        this.actor = this.convertValues(source["actor"], RunActor);
	        this.source = this.convertValues(source["source"], RunSource);
	        this.recipe = source["recipe"];
	        this.target = this.convertValues(source["target"], RunTarget);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RunEvent {
	    apiVersion: string;
	    id: string;
	    runId: string;
	    type: string;
	    start?: RunStart;
	    step?: RunStep;
	    finish?: RunFinish;
	
	    static createFrom(source: any = {}) {
	        return new RunEvent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.apiVersion = source["apiVersion"];
	        this.id = source["id"];
	        this.runId = source["runId"];
	        this.type = source["type"];
	        this.start = this.convertValues(source["start"], RunStart);
	        this.step = this.convertValues(source["step"], RunStep);
	        this.finish = this.convertValues(source["finish"], RunFinish);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SavedRunEvent {
	    event: RunEvent;
	    at: string;
	
	    static createFrom(source: any = {}) {
	        return new SavedRunEvent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.event = this.convertValues(source["event"], RunEvent);
	        this.at = source["at"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RunTarget {
	    environment: string;
	    workload: string;
	    container: string;
	    buildId: string;
	
	    static createFrom(source: any = {}) {
	        return new RunTarget(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.environment = source["environment"];
	        this.workload = source["workload"];
	        this.container = source["container"];
	        this.buildId = source["buildId"];
	    }
	}
	export class RunSource {
	    repository: string;
	    commit: string;
	    branch: string;
	    dirty: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RunSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.repository = source["repository"];
	        this.commit = source["commit"];
	        this.branch = source["branch"];
	        this.dirty = source["dirty"];
	    }
	}
	export class RunActor {
	    kind: string;
	    name: string;
	    sessionId: string;
	
	    static createFrom(source: any = {}) {
	        return new RunActor(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.sessionId = source["sessionId"];
	    }
	}
	export class Run {
	    apiVersion: string;
	    id: string;
	    kind: string;
	    application: string;
	    version: string;
	    actor: RunActor;
	    source: RunSource;
	    recipe: string;
	    target?: RunTarget;
	    connection?: string;
	    namespace?: string;
	    status: string;
	    image?: string;
	    startedAt: string;
	    finishedAt?: string;
	    message: string;
	    events: SavedRunEvent[];
	
	    static createFrom(source: any = {}) {
	        return new Run(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.apiVersion = source["apiVersion"];
	        this.id = source["id"];
	        this.kind = source["kind"];
	        this.application = source["application"];
	        this.version = source["version"];
	        this.actor = this.convertValues(source["actor"], RunActor);
	        this.source = this.convertValues(source["source"], RunSource);
	        this.recipe = source["recipe"];
	        this.target = this.convertValues(source["target"], RunTarget);
	        this.connection = source["connection"];
	        this.namespace = source["namespace"];
	        this.status = source["status"];
	        this.image = source["image"];
	        this.startedAt = source["startedAt"];
	        this.finishedAt = source["finishedAt"];
	        this.message = source["message"];
	        this.events = this.convertValues(source["events"], SavedRunEvent);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	
	
	
	
	
	export class Snapshot {
	    runs: Run[];
	    root: string;
	    yaml: string;
	    revision: string;
	    project: Project;
	    artifacts: Artifact[];
	    releases: Record[];
	
	    static createFrom(source: any = {}) {
	        return new Snapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.runs = this.convertValues(source["runs"], Run);
	        this.root = source["root"];
	        this.yaml = source["yaml"];
	        this.revision = source["revision"];
	        this.project = this.convertValues(source["project"], Project);
	        this.artifacts = this.convertValues(source["artifacts"], Artifact);
	        this.releases = this.convertValues(source["releases"], Record);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace release {
	
	export class DesktopPlan {
	    id: string;
	    action: string;
	    beforeReplicas: number;
	    replicas: number;
	    namespace: string;
	    workload: string;
	    container: string;
	    beforeImage: string;
	    image: string;
	    expiresAt: string;
	    uid: string;
	    resourceVersion: string;
	    changes?: string[];
	    warnings?: string[];
	
	    static createFrom(source: any = {}) {
	        return new DesktopPlan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.action = source["action"];
	        this.beforeReplicas = source["beforeReplicas"];
	        this.replicas = source["replicas"];
	        this.namespace = source["namespace"];
	        this.workload = source["workload"];
	        this.container = source["container"];
	        this.beforeImage = source["beforeImage"];
	        this.image = source["image"];
	        this.expiresAt = source["expiresAt"];
	        this.uid = source["uid"];
	        this.resourceVersion = source["resourceVersion"];
	        this.changes = source["changes"];
	        this.warnings = source["warnings"];
	    }
	}
	export class EnvSecretRef {
	    name: string;
	    secret: string;
	    key: string;
	
	    static createFrom(source: any = {}) {
	        return new EnvSecretRef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.secret = source["secret"];
	        this.key = source["key"];
	    }
	}
	export class SecretInfo {
	    name: string;
	    type: string;
	    keys: string[];
	    resourceVersion: string;
	    updatedAt?: string;
	
	    static createFrom(source: any = {}) {
	        return new SecretInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.keys = source["keys"];
	        this.resourceVersion = source["resourceVersion"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class SecretKeyChange {
	    key: string;
	    change: string;
	
	    static createFrom(source: any = {}) {
	        return new SecretKeyChange(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.change = source["change"];
	    }
	}
	export class SecretPlan {
	    id: string;
	    namespace: string;
	    name: string;
	    type: string;
	    create: boolean;
	    keys: SecretKeyChange[];
	    contentSha256: string;
	    resourceVersion?: string;
	
	    static createFrom(source: any = {}) {
	        return new SecretPlan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.create = source["create"];
	        this.keys = this.convertValues(source["keys"], SecretKeyChange);
	        this.contentSha256 = source["contentSha256"];
	        this.resourceVersion = source["resourceVersion"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class WorkloadPatch {
	    container?: string;
	    addImagePullSecrets?: string[];
	    removeImagePullSecrets?: string[];
	    addEnvFromSecrets?: string[];
	    envFromOptional?: boolean;
	    removeEnvFromSecrets?: string[];
	    setEnvSecretRefs?: EnvSecretRef[];
	    removeEnv?: string[];
	
	    static createFrom(source: any = {}) {
	        return new WorkloadPatch(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.container = source["container"];
	        this.addImagePullSecrets = source["addImagePullSecrets"];
	        this.removeImagePullSecrets = source["removeImagePullSecrets"];
	        this.addEnvFromSecrets = source["addEnvFromSecrets"];
	        this.envFromOptional = source["envFromOptional"];
	        this.removeEnvFromSecrets = source["removeEnvFromSecrets"];
	        this.setEnvSecretRefs = this.convertValues(source["setEnvSecretRefs"], EnvSecretRef);
	        this.removeEnv = source["removeEnv"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace workspace {
	
	export class AIRequest {
	    prompt: string;
	    kind: string;
	    schema: string[];
	
	    static createFrom(source: any = {}) {
	        return new AIRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.prompt = source["prompt"];
	        this.kind = source["kind"];
	        this.schema = source["schema"];
	    }
	}
	export class AISettings {
	    endpoint: string;
	    model: string;
	    apiKey?: string;
	    hasKey: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AISettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.endpoint = source["endpoint"];
	        this.model = source["model"];
	        this.apiKey = source["apiKey"];
	        this.hasKey = source["hasKey"];
	    }
	}
	export class BrowseRequest {
	    database: string;
	    cursor: string;
	    filter: string;
	
	    static createFrom(source: any = {}) {
	        return new BrowseRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.database = source["database"];
	        this.cursor = source["cursor"];
	        this.filter = source["filter"];
	    }
	}
	export class ContainerInfo {
	    name: string;
	    kind: string;
	    image: string;
	    state: string;
	    ready: boolean;
	    restarts: number;
	    imageId?: string;
	
	    static createFrom(source: any = {}) {
	        return new ContainerInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.image = source["image"];
	        this.state = source["state"];
	        this.ready = source["ready"];
	        this.restarts = source["restarts"];
	        this.imageId = source["imageId"];
	    }
	}
	export class Property {
	    name: string;
	    value: string;
	
	    static createFrom(source: any = {}) {
	        return new Property(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.value = source["value"];
	    }
	}
	export class ClusterRef {
	    context: string;
	    namespace: string;
	    resource: string;
	    name: string;
	    uid: string;
	
	    static createFrom(source: any = {}) {
	        return new ClusterRef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.context = source["context"];
	        this.namespace = source["namespace"];
	        this.resource = source["resource"];
	        this.name = source["name"];
	        this.uid = source["uid"];
	    }
	}
	export class ClusterDetails {
	    ref: ClusterRef;
	    properties: Property[];
	    labels: Record<string, string>;
	    containers: ContainerInfo[];
	    definition: string;
	
	    static createFrom(source: any = {}) {
	        return new ClusterDetails(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ref = this.convertValues(source["ref"], ClusterRef);
	        this.properties = this.convertValues(source["properties"], Property);
	        this.labels = source["labels"];
	        this.containers = this.convertValues(source["containers"], ContainerInfo);
	        this.definition = source["definition"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class ConfigMapUpdate {
	    ref: ClusterRef;
	    resourceVersion: string;
	    data: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new ConfigMapUpdate(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ref = this.convertValues(source["ref"], ClusterRef);
	        this.resourceVersion = source["resourceVersion"];
	        this.data = source["data"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SSHConfig {
	    enabled: boolean;
	    host: string;
	    port: number;
	    username: string;
	    authMethod: string;
	    password?: string;
	    privateKeyPath: string;
	    passphrase?: string;
	    knownHostsPath: string;
	    hostKeyFingerprint: string;
	    hasPassword: boolean;
	    hasPassphrase: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SSHConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.username = source["username"];
	        this.authMethod = source["authMethod"];
	        this.password = source["password"];
	        this.privateKeyPath = source["privateKeyPath"];
	        this.passphrase = source["passphrase"];
	        this.knownHostsPath = source["knownHostsPath"];
	        this.hostKeyFingerprint = source["hostKeyFingerprint"];
	        this.hasPassword = source["hasPassword"];
	        this.hasPassphrase = source["hasPassphrase"];
	    }
	}
	export class Connection {
	    ssh: SSHConfig;
	    id: string;
	    name: string;
	    kind: string;
	    host: string;
	    port: number;
	    username: string;
	    password?: string;
	    database: string;
	    authSource?: string;
	    environment: string;
	    tls: boolean;
	    kubeconfig: string;
	    context: string;
	    hasPassword: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Connection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ssh = this.convertValues(source["ssh"], SSHConfig);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.username = source["username"];
	        this.password = source["password"];
	        this.database = source["database"];
	        this.authSource = source["authSource"];
	        this.environment = source["environment"];
	        this.tls = source["tls"];
	        this.kubeconfig = source["kubeconfig"];
	        this.context = source["context"];
	        this.hasPassword = source["hasPassword"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class KubeContext {
	    name: string;
	    cluster: string;
	    namespace: string;
	    execAuth: boolean;
	
	    static createFrom(source: any = {}) {
	        return new KubeContext(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.cluster = source["cluster"];
	        this.namespace = source["namespace"];
	        this.execAuth = source["execAuth"];
	    }
	}
	export class KubeconfigInfo {
	    currentContext: string;
	    contexts: KubeContext[];
	
	    static createFrom(source: any = {}) {
	        return new KubeconfigInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.currentContext = source["currentContext"];
	        this.contexts = this.convertValues(source["contexts"], KubeContext);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class NginxConfig {
	    path: string;
	    text: string;
	    revision: string;
	    message?: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new NginxConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.text = source["text"];
	        this.revision = source["revision"];
	        this.message = source["message"];
	        this.error = source["error"];
	    }
	}
	export class NginxUpdate {
	    revision: string;
	    text: string;
	
	    static createFrom(source: any = {}) {
	        return new NginxUpdate(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.revision = source["revision"];
	        this.text = source["text"];
	    }
	}
	export class OpsPod {
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
	
	    static createFrom(source: any = {}) {
	        return new OpsPod(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.namespace = source["namespace"];
	        this.uid = source["uid"];
	        this.workload = source["workload"];
	        this.ip = source["ip"];
	        this.node = source["node"];
	        this.nodeIP = source["nodeIP"];
	        this.phase = source["phase"];
	        this.ready = source["ready"];
	        this.total = source["total"];
	        this.restarts = source["restarts"];
	        this.createdAt = source["createdAt"];
	        this.ports = source["ports"];
	        this.containers = this.convertValues(source["containers"], ContainerInfo);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Workload {
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
	
	    static createFrom(source: any = {}) {
	        return new Workload(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.uid = source["uid"];
	        this.resource = source["resource"];
	        this.desired = source["desired"];
	        this.ready = source["ready"];
	        this.image = source["image"];
	        this.extraImages = source["extraImages"];
	        this.updatedAt = source["updatedAt"];
	        this.selector = source["selector"];
	    }
	}
	export class OpsCluster {
	    workloads: Workload[];
	    pods: OpsPod[];
	    fetchedAt: string;
	    truncated: boolean;
	    warnings: string[];
	
	    static createFrom(source: any = {}) {
	        return new OpsCluster(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.workloads = this.convertValues(source["workloads"], Workload);
	        this.pods = this.convertValues(source["pods"], OpsPod);
	        this.fetchedAt = source["fetchedAt"];
	        this.truncated = source["truncated"];
	        this.warnings = source["warnings"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class OpsMetric {
	    pod: string;
	    namespace: string;
	    cpu: number;
	    memory: number;
	    timestamp: string;
	
	    static createFrom(source: any = {}) {
	        return new OpsMetric(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pod = source["pod"];
	        this.namespace = source["namespace"];
	        this.cpu = source["cpu"];
	        this.memory = source["memory"];
	        this.timestamp = source["timestamp"];
	    }
	}
	
	export class PodLogRequest {
	    ref: ClusterRef;
	    container: string;
	    tailLines: number;
	    previous: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PodLogRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ref = this.convertValues(source["ref"], ClusterRef);
	        this.container = source["container"];
	        this.tailLines = source["tailLines"];
	        this.previous = source["previous"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PodLogs {
	    text: string;
	    truncated: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PodLogs(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.truncated = source["truncated"];
	    }
	}
	
	export class Resource {
	    name: string;
	    kind: string;
	    query: string;
	
	    static createFrom(source: any = {}) {
	        return new Resource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.query = source["query"];
	    }
	}
	export class Result {
	    resourceType?: string;
	    columns: string[];
	    rows: any[][];
	    duration: number;
	    truncated: boolean;
	    message: string;
	    nextQuery?: string;
	    previousQuery?: string;
	    offset: number;
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.resourceType = source["resourceType"];
	        this.columns = source["columns"];
	        this.rows = source["rows"];
	        this.duration = source["duration"];
	        this.truncated = source["truncated"];
	        this.message = source["message"];
	        this.nextQuery = source["nextQuery"];
	        this.previousQuery = source["previousQuery"];
	        this.offset = source["offset"];
	    }
	}
	export class ResourceDetails {
	    name: string;
	    database: string;
	    kind: string;
	    properties: Property[];
	    columns: Result;
	    indexes: Result;
	    preview: Result;
	    definition: string;
	
	    static createFrom(source: any = {}) {
	        return new ResourceDetails(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.database = source["database"];
	        this.kind = source["kind"];
	        this.properties = this.convertValues(source["properties"], Property);
	        this.columns = this.convertValues(source["columns"], Result);
	        this.indexes = this.convertValues(source["indexes"], Result);
	        this.preview = this.convertValues(source["preview"], Result);
	        this.definition = source["definition"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ResourcePage {
	    resources: Resource[];
	    nextCursor: string;
	    hasMore: boolean;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new ResourcePage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.resources = this.convertValues(source["resources"], Resource);
	        this.nextCursor = source["nextCursor"];
	        this.hasMore = source["hasMore"];
	        this.message = source["message"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	export class StoreHealth {
	    status: string;
	    message: string;
	    canRestore: boolean;
	    backupAvailable: boolean;
	    backupUpdatedAt: string;
	    backupWarning: boolean;
	    configVersion: number;
	
	    static createFrom(source: any = {}) {
	        return new StoreHealth(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.message = source["message"];
	        this.canRestore = source["canRestore"];
	        this.backupAvailable = source["backupAvailable"];
	        this.backupUpdatedAt = source["backupUpdatedAt"];
	        this.backupWarning = source["backupWarning"];
	        this.configVersion = source["configVersion"];
	    }
	}

}

