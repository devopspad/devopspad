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
