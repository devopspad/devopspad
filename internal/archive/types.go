// Package archive implements the DevOpsPad archive protocol: a Git-friendly
// directory describing every product, service, host, domain and account of an
// OPC product matrix. It reads and validates files only; it never contacts
// live infrastructure and never stores secret values.
package archive

// APIVersion is the only archive protocol version this package reads.
const APIVersion = "devopspad.io/v1alpha1"

// Header is shared by every resource file.
type Header struct {
	APIVersion string   `yaml:"apiVersion" json:"apiVersion" schema:"required,const=devopspad.io/v1alpha1"`
	Kind       string   `yaml:"kind" json:"kind" schema:"required"`
	Metadata   Metadata `yaml:"metadata" json:"metadata" schema:"required"`
}

func (h *Header) head() *Header { return h }

type Metadata struct {
	Name        string            `yaml:"name" json:"name" schema:"required"`
	Product     string            `yaml:"product,omitempty" json:"product,omitempty" schema:"desc=Service only: the product directory name"`
	Title       string            `yaml:"title,omitempty" json:"title,omitempty"`
	Description string            `yaml:"description,omitempty" json:"description,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty" json:"annotations,omitempty" schema:"desc=Free-form notes; not validated except for secrets"`
}

// Cost amounts are decimal strings to avoid floating point rounding.
type Cost struct {
	Amount   string `yaml:"amount" json:"amount" schema:"required,pattern=^[0-9]+(\\.[0-9]{1,4})?$"`
	Currency string `yaml:"currency" json:"currency" schema:"required,pattern=^[A-Z]{3}$"`
	Period   string `yaml:"period" json:"period" schema:"required,enum=month|year|once"`
}

type Renewal struct {
	ExpiresAt string `yaml:"expiresAt" json:"expiresAt" schema:"required,format=date"`
	AutoRenew bool   `yaml:"autoRenew" json:"autoRenew"`
}

// Credential names a secret and where it lives. Ref is a secretRef, never a value.
type Credential struct {
	Name  string `yaml:"name" json:"name" schema:"required"`
	Ref   string `yaml:"ref" json:"ref" schema:"required,secretref"`
	Scope string `yaml:"scope,omitempty" json:"scope,omitempty"`
}

type Root struct {
	Header `yaml:",inline"`
	Spec   RootSpec `yaml:"spec" json:"spec"`
}
type RootSpec struct {
	Defaults RootDefaults `yaml:"defaults,omitempty" json:"defaults"`
}
type RootDefaults struct {
	RenewalWarningDays int `yaml:"renewalWarningDays,omitempty" json:"renewalWarningDays,omitempty" schema:"min=1,max=365,desc=Default window for report renewals (30)"`
}

type Provider struct {
	Header `yaml:",inline"`
	Spec   ProviderSpec `yaml:"spec" json:"spec"`
}
type ProviderSpec struct {
	Categories  []string     `yaml:"categories,omitempty" json:"categories,omitempty" schema:"enum=cloud|registrar|dns|cdn|storage|payment|email|appstore|analytics|ai|ci|registry|monitoring|saas|other"`
	Console     string       `yaml:"console,omitempty" json:"console,omitempty" schema:"format=url"`
	Account     string       `yaml:"account,omitempty" json:"account,omitempty" schema:"desc=Login identifier, not a secret"`
	MFA         string       `yaml:"mfa,omitempty" json:"mfa,omitempty" schema:"desc=Where the second factor lives"`
	Plan        string       `yaml:"plan,omitempty" json:"plan,omitempty"`
	Cost        *Cost        `yaml:"cost,omitempty" json:"cost,omitempty"`
	Renewal     *Renewal     `yaml:"renewal,omitempty" json:"renewal,omitempty"`
	Credentials []Credential `yaml:"credentials,omitempty" json:"credentials,omitempty"`
}

type Host struct {
	Header `yaml:",inline"`
	Spec   HostSpec `yaml:"spec" json:"spec"`
}
type HostSpec struct {
	Provider  string    `yaml:"provider,omitempty" json:"provider,omitempty" schema:"ref=provider"`
	State     string    `yaml:"state,omitempty" json:"state,omitempty" schema:"enum=active|standby|retired"`
	Addresses Addresses `yaml:"addresses,omitempty" json:"addresses"`
	SSH       *SSH      `yaml:"ssh,omitempty" json:"ssh,omitempty"`
	OS        string    `yaml:"os,omitempty" json:"os,omitempty"`
	Size      Size      `yaml:"size,omitempty" json:"size"`
	Roles     []string  `yaml:"roles,omitempty" json:"roles,omitempty"`
	Cost      *Cost     `yaml:"cost,omitempty" json:"cost,omitempty"`
	Renewal   *Renewal  `yaml:"renewal,omitempty" json:"renewal,omitempty"`
}
type Addresses struct {
	Public  string `yaml:"public,omitempty" json:"public,omitempty"`
	Private string `yaml:"private,omitempty" json:"private,omitempty"`
}
type SSH struct {
	Host          string `yaml:"host" json:"host" schema:"required"`
	Port          int    `yaml:"port,omitempty" json:"port,omitempty" schema:"min=1,max=65535"`
	User          string `yaml:"user" json:"user" schema:"required"`
	Auth          string `yaml:"auth" json:"auth" schema:"required,enum=key|agent|password"`
	KeyRef        string `yaml:"keyRef,omitempty" json:"keyRef,omitempty" schema:"secretref,desc=Private key file, e.g. file:~/.ssh/id_ed25519"`
	PassphraseRef string `yaml:"passphraseRef,omitempty" json:"passphraseRef,omitempty" schema:"secretref"`
	PasswordRef   string `yaml:"passwordRef,omitempty" json:"passwordRef,omitempty" schema:"secretref"`
	HostKey       string `yaml:"hostKey,omitempty" json:"hostKey,omitempty" schema:"pattern=^SHA256:[A-Za-z0-9+/]{43}=?$"`
	KnownHosts    string `yaml:"knownHosts,omitempty" json:"knownHosts,omitempty"`
}
type Size struct {
	CPU       int     `yaml:"cpu,omitempty" json:"cpu,omitempty" schema:"min=0"`
	MemoryGiB float64 `yaml:"memoryGiB,omitempty" json:"memoryGiB,omitempty" schema:"min=0"`
	DiskGiB   float64 `yaml:"diskGiB,omitempty" json:"diskGiB,omitempty" schema:"min=0"`
}

type Cluster struct {
	Header `yaml:",inline"`
	Spec   ClusterSpec `yaml:"spec" json:"spec"`
}
type ClusterSpec struct {
	Distribution string     `yaml:"distribution" json:"distribution" schema:"required,enum=k3s|k8s"`
	Hosts        []string   `yaml:"hosts,omitempty" json:"hosts,omitempty" schema:"ref=host"`
	Tunnel       string     `yaml:"tunnel,omitempty" json:"tunnel,omitempty" schema:"ref=host,desc=Reach the API server through this host's SSH"`
	Kubeconfig   Kubeconfig `yaml:"kubeconfig" json:"kubeconfig" schema:"required"`
}
type Kubeconfig struct {
	Source  string `yaml:"source" json:"source" schema:"required,enum=remote|local"`
	Host    string `yaml:"host,omitempty" json:"host,omitempty" schema:"ref=host,desc=source=remote: read over SSH from this host"`
	Path    string `yaml:"path,omitempty" json:"path,omitempty" schema:"desc=source=remote: absolute path on the host"`
	PathRef string `yaml:"pathRef,omitempty" json:"pathRef,omitempty" schema:"secretref,desc=source=local: file:<path>"`
	Context string `yaml:"context,omitempty" json:"context,omitempty"`
}

type Datastore struct {
	Header `yaml:",inline"`
	Spec   DatastoreSpec `yaml:"spec" json:"spec"`
}
type DatastoreSpec struct {
	Engine      string       `yaml:"engine" json:"engine" schema:"required,enum=mysql|postgres|redis|mongodb|clickhouse|kafka|s3|other"`
	Runtime     Runtime      `yaml:"runtime,omitempty" json:"runtime"`
	Tunnel      string       `yaml:"tunnel,omitempty" json:"tunnel,omitempty" schema:"ref=host"`
	Endpoint    string       `yaml:"endpoint,omitempty" json:"endpoint,omitempty" schema:"desc=host:port as reached from the tunnel host (or directly)"`
	Username    string       `yaml:"username,omitempty" json:"username,omitempty" schema:"desc=Login user for connect sync; the password is a credential reference"`
	TLS         bool         `yaml:"tls,omitempty" json:"tls,omitempty"`
	Parts       []string     `yaml:"parts,omitempty" json:"parts,omitempty" schema:"desc=Databases or buckets, referenced as datastore:<name>#<part>"`
	Credentials []Credential `yaml:"credentials,omitempty" json:"credentials,omitempty"`
	Backup      *Backup      `yaml:"backup,omitempty" json:"backup,omitempty"`
	Cost        *Cost        `yaml:"cost,omitempty" json:"cost,omitempty"`
}
type Runtime struct {
	Host      string `yaml:"host,omitempty" json:"host,omitempty" schema:"ref=host"`
	Cluster   string `yaml:"cluster,omitempty" json:"cluster,omitempty" schema:"ref=cluster"`
	Namespace string `yaml:"namespace,omitempty" json:"namespace,omitempty"`
	Provider  string `yaml:"provider,omitempty" json:"provider,omitempty" schema:"ref=provider"`
	Managed   string `yaml:"managed,omitempty" json:"managed,omitempty" schema:"desc=Managed service name, e.g. rds"`
}
type Backup struct {
	Method        string `yaml:"method,omitempty" json:"method,omitempty"`
	Schedule      string `yaml:"schedule,omitempty" json:"schedule,omitempty"`
	Target        string `yaml:"target,omitempty" json:"target,omitempty" schema:"ref=provider|host|datastore"`
	RetentionDays int    `yaml:"retentionDays,omitempty" json:"retentionDays,omitempty" schema:"min=0"`
}

type Registry struct {
	Header `yaml:",inline"`
	Spec   RegistrySpec `yaml:"spec" json:"spec"`
}
type RegistrySpec struct {
	URL         string       `yaml:"url" json:"url" schema:"required"`
	Provider    string       `yaml:"provider,omitempty" json:"provider,omitempty" schema:"ref=provider|host"`
	Credentials []Credential `yaml:"credentials,omitempty" json:"credentials,omitempty"`
}

type Domain struct {
	Header `yaml:",inline"`
	Spec   DomainSpec `yaml:"spec" json:"spec"`
}
type DomainSpec struct {
	Registrar    string        `yaml:"registrar,omitempty" json:"registrar,omitempty" schema:"ref=provider"`
	DNS          string        `yaml:"dns,omitempty" json:"dns,omitempty" schema:"ref=provider"`
	Cost         *Cost         `yaml:"cost,omitempty" json:"cost,omitempty"`
	Renewal      *Renewal      `yaml:"renewal,omitempty" json:"renewal,omitempty"`
	Records      []DNSRecord   `yaml:"records,omitempty" json:"records,omitempty" schema:"desc=Key records only, used for reverse lookup"`
	Certificates []Certificate `yaml:"certificates,omitempty" json:"certificates,omitempty"`
}
type DNSRecord struct {
	Name    string `yaml:"name" json:"name" schema:"required,desc=@ for the apex"`
	Type    string `yaml:"type" json:"type" schema:"required,enum=A|AAAA|CNAME|TXT|MX|NS|CAA|SRV"`
	Value   string `yaml:"value,omitempty" json:"value,omitempty"`
	Target  string `yaml:"target,omitempty" json:"target,omitempty" schema:"ref=host|provider,desc=Use instead of value when the record points at a host"`
	Proxied bool   `yaml:"proxied,omitempty" json:"proxied,omitempty"`
}
type Certificate struct {
	Names     []string `yaml:"names" json:"names" schema:"required"`
	Issuer    string   `yaml:"issuer,omitempty" json:"issuer,omitempty"`
	ManagedBy string   `yaml:"managedBy,omitempty" json:"managedBy,omitempty" schema:"ref=host|provider"`
	Method    string   `yaml:"method,omitempty" json:"method,omitempty"`
	ExpiresAt string   `yaml:"expiresAt,omitempty" json:"expiresAt,omitempty" schema:"format=date"`
}

type Product struct {
	Header `yaml:",inline"`
	Spec   ProductSpec `yaml:"spec" json:"spec"`
}
type ProductSpec struct {
	Stage        string            `yaml:"stage" json:"stage" schema:"required,enum=idea|building|live|paused|sunset"`
	LaunchedAt   string            `yaml:"launchedAt,omitempty" json:"launchedAt,omitempty" schema:"format=date"`
	Links        map[string]string `yaml:"links,omitempty" json:"links,omitempty"`
	Domains      []string          `yaml:"domains,omitempty" json:"domains,omitempty" schema:"ref=domain"`
	Providers    []ProviderUse     `yaml:"providers,omitempty" json:"providers,omitempty"`
	Environments []Environment     `yaml:"environments,omitempty" json:"environments,omitempty"`
}
type ProviderUse struct {
	Ref     string `yaml:"ref" json:"ref" schema:"required,ref=provider"`
	Purpose string `yaml:"purpose,omitempty" json:"purpose,omitempty"`
}
type Environment struct {
	Name      string   `yaml:"name" json:"name" schema:"required"`
	Cluster   string   `yaml:"cluster,omitempty" json:"cluster,omitempty" schema:"ref=cluster"`
	Namespace string   `yaml:"namespace,omitempty" json:"namespace,omitempty"`
	Hosts     []string `yaml:"hosts,omitempty" json:"hosts,omitempty" schema:"ref=host"`
}

type Service struct {
	Header `yaml:",inline"`
	Spec   ServiceSpec `yaml:"spec" json:"spec"`
}
type ServiceSpec struct {
	Type        string       `yaml:"type" json:"type" schema:"required,enum=web|api|worker|cron|static|other"`
	Repo        *Repo        `yaml:"repo,omitempty" json:"repo,omitempty"`
	Build       *Build       `yaml:"build,omitempty" json:"build,omitempty"`
	Release     *Release     `yaml:"release,omitempty" json:"release,omitempty"`
	Deployments []Deployment `yaml:"deployments,omitempty" json:"deployments,omitempty"`
	Endpoints   []Endpoint   `yaml:"endpoints,omitempty" json:"endpoints,omitempty"`
	Configs     []Config     `yaml:"configs,omitempty" json:"configs,omitempty"`
	DependsOn   []string     `yaml:"dependsOn,omitempty" json:"dependsOn,omitempty" schema:"ref=service|datastore|provider|registry"`
	Secrets     []Credential `yaml:"secrets,omitempty" json:"secrets,omitempty" schema:"desc=Secrets the service needs at runtime (references only)"`
}
type Repo struct {
	URL           string `yaml:"url" json:"url" schema:"required"`
	Path          string `yaml:"path,omitempty" json:"path,omitempty"`
	DefaultBranch string `yaml:"defaultBranch,omitempty" json:"defaultBranch,omitempty"`
}
type Build struct {
	Method string `yaml:"method" json:"method" schema:"required,enum=ci|local|none"`
	CI     *CI    `yaml:"ci,omitempty" json:"ci,omitempty"`
	Image  string `yaml:"image,omitempty" json:"image,omitempty" schema:"ref=registry,desc=registry:<name>#<repository>"`
}
type CI struct {
	Provider string `yaml:"provider" json:"provider" schema:"required,ref=provider"`
	Workflow string `yaml:"workflow,omitempty" json:"workflow,omitempty"`
}
type Release struct {
	Method string `yaml:"method" json:"method" schema:"required,enum=devopspad|devopspad-spec|ci|manual"`
	Spec   string `yaml:"spec,omitempty" json:"spec,omitempty" schema:"desc=method=devopspad-spec: path in the code repository"`
	Notes  string `yaml:"notes,omitempty" json:"notes,omitempty" schema:"desc=Markdown file relative to the product directory; read before every release"`
}
type Deployment struct {
	Environment string    `yaml:"environment" json:"environment" schema:"required"`
	Runtime     string    `yaml:"runtime" json:"runtime" schema:"required,enum=k8s|host|static"`
	Cluster     string    `yaml:"cluster,omitempty" json:"cluster,omitempty" schema:"ref=cluster,desc=Defaults to the environment's cluster"`
	Namespace   string    `yaml:"namespace,omitempty" json:"namespace,omitempty" schema:"desc=Defaults to the environment's namespace"`
	Selector    string    `yaml:"selector,omitempty" json:"selector,omitempty"`
	Workload    *Workload `yaml:"workload,omitempty" json:"workload,omitempty"`
	Hosts       []string  `yaml:"hosts,omitempty" json:"hosts,omitempty" schema:"ref=host"`
	Unit        string    `yaml:"unit,omitempty" json:"unit,omitempty" schema:"desc=runtime=host: systemd unit or compose file"`
	Provider    string    `yaml:"provider,omitempty" json:"provider,omitempty" schema:"ref=provider,desc=runtime=static: hosting provider"`
}
type Workload struct {
	Kind      string `yaml:"kind" json:"kind" schema:"required,enum=Deployment|StatefulSet|DaemonSet|CronJob"`
	Name      string `yaml:"name" json:"name" schema:"required"`
	Container string `yaml:"container,omitempty" json:"container,omitempty"`
}
type Endpoint struct {
	URL    string `yaml:"url" json:"url" schema:"required,format=url"`
	Health string `yaml:"health,omitempty" json:"health,omitempty"`
}
type Config struct {
	Name        string       `yaml:"name" json:"name" schema:"required"`
	Type        string       `yaml:"type" json:"type" schema:"required,enum=nginx|configmap|env-file|systemd|file"`
	Environment string       `yaml:"environment" json:"environment" schema:"required"`
	Source      string       `yaml:"source" json:"source" schema:"required,desc=File relative to the product directory"`
	Target      ConfigTarget `yaml:"target,omitempty" json:"target"`
}
type ConfigTarget struct {
	Host      string `yaml:"host,omitempty" json:"host,omitempty" schema:"ref=host"`
	Path      string `yaml:"path,omitempty" json:"path,omitempty"`
	ConfigMap string `yaml:"configMap,omitempty" json:"configMap,omitempty"`
	Key       string `yaml:"key,omitempty" json:"key,omitempty"`
}

// NoteMeta is the optional YAML front matter of a Markdown note.
type NoteMeta struct {
	Title string   `yaml:"title,omitempty" json:"title,omitempty"`
	Refs  []string `yaml:"refs,omitempty" json:"refs,omitempty"`
	Date  string   `yaml:"date,omitempty" json:"date,omitempty"`
	Tags  []string `yaml:"tags,omitempty" json:"tags,omitempty"`
}
