export namespace main {
	
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

