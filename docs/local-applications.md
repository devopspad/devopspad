# Infra：集群与应用

最左侧只有“Data”和“Infra”两个入口。Infra 就是原应用服务，按环境与应用组织制品、发布、Deployment、Pod、监控、日志、事件和配置，默认沿用 Orca 深色风格。`/#infra` 打开 Infra，旧的 `/#apps` 链接自动归并到该入口。

## 环境与集群节点

Infra 左上角选择项目环境后，“Nodes”显示对应集群的 Ready 状态、角色、内外网 IP、CPU、内存、系统和 Kubernetes 版本。节点查询覆盖整个集群，不受应用或命名空间筛选影响；点击节点可查看详情。多台机器属于同一 K3s 集群时，通过一个集群连接查看所有返回的节点；独立集群分别添加连接并配置环境映射。节点列表需要 `nodes` 的 list 权限，详情需要 get 权限；读取失败和结果截断会明确显示。

“Cluster resources”保留 Kubernetes / K3s 连接管理、按连接 Environment 筛选，以及命名空间和资源查询。Environment 支持自定义名称，例如 `heartie-pro`。Data 入口显示数据库、缓存和消息系统连接。配置内容超过窗口高度时，在主内容区滚动即可查看后续 ConfigMap、环境变量和挂载信息。

## 本机使用

```sh
npm ci --prefix frontend
wails dev
```

首次打开显示已保存的真实连接；尚未配置时显示空状态。之后记住上次工作区及 Infra 的环境、应用和页面。浏览器运行 `npm run dev --prefix frontend` 可查看界面和空状态，实际连接、查询及发布需要 Wails 桌面版。产品不包含演示工作区、预置假数据、模拟发布或本地 SQLite 演示引擎。旧工作区格式只迁移真实连接的查询草稿。

1. 在 Cluster resources 添加 Kubernetes / K3s 连接，例如命名为 `local-k3s`。现有 kubeconfig、SSH、钥匙串与认证限制仍然适用。
2. 在“Infra”点击“Connect K3s”，选择本机已有连接（也可新建），输入命名空间并读取负载，选择应用后接入。向导从真实负载读取选择器，仅保存本地映射。也可以打开“Project mapping”，参考 [project.yaml 示例](../examples/application-project/project.yaml) 添加环境和应用。`connection` 匹配本机连接名称，`namespace` 必须是具体命名空间，`selector` 匹配应用的 Pod 标签。各人的连接名称一致，凭据可以不同。
3. 在 Artifacts 登记已有镜像的完整地址，例如 `registry.lan:5000/svc-user@sha256:<64 位小写十六进制摘要>`。制品登记不会构建、推送镜像，也不会校验 Registry 中是否存在该摘要；实际拉取由集群执行。
4. 选择制品，选择已有 Deployment 及其容器，生成发布计划。检查当前镜像、目标镜像和环境后确认执行。
5. 在 Deployment 行内使用“Scale”和“Restart”：生成计划，核对目标与副本数后执行。扩缩容支持 0–1,000，设为 0 会停止全部 Pod；只修改 replicas，不触发 Pod 模板滚动更新。若负载受 HPA 管理，自动扩缩容可能覆盖手动副本数。
6. 在发布记录中打开成功的镜像发布，选择“Roll back to this version”。预检查读取当前镜像，以该历史记录的不可变摘要生成新计划；确认后产生独立的回退记录。此操作只回退目标容器镜像，保留当前其他配置。
7. 在 Deployment / Pod 页面查看状态，进入日志和应用事件；监控页读取 CPU 和内存用量。
8. “App config”直接读取所选环境、应用的负载，显示普通容器及 init 容器的环境变量、配置挂载，以及通过 env、envFrom、volume 和 projected volume 引用的 ConfigMap 内容。ConfigMap 不需要与应用同名或带相同标签；Secret 仅显示引用，不读取内容。权限不足和配置读取失败会明确显示，切换应用或环境时按当前范围加载配置。项目级 `project.yaml` 在“Project mapping”编辑，支持格式化长行 YAML。

ConfigMap 文本按文件显示行号及语法高亮，支持 YAML、JSON、TOML、Nginx 和 Properties/INI。点击“编辑配置”后可以修改、取消或“保存到集群”；修改过的 YAML、JSON、TOML 会先校验语法。保存需要 ConfigMap 的 get/update 权限，并校验 UID 和 resourceVersion，发现资源被替换或版本冲突时保留编辑内容并提示刷新，不覆盖集群的新版本。未编辑的文本项、二进制数据和元数据保持原样；immutable ConfigMap 和二进制项只读。切换页面、应用或环境时，未保存的草稿按集群连接、上下文、命名空间和 ConfigMap 暂存在内存中，退出或重新加载应用后不保留。保存配置不会自动重启负载；环境变量和 subPath 挂载需要重启应用后生效，普通挂载仍取决于应用的重新加载方式。

镜像仓库可以自建在本机或局域网，目标节点必须能访问其地址。镜像正文保存在 OCI Registry 中，不写入项目 YAML。应用不要求云端账号。

## 共享项目目录

默认目录是 `os.UserConfigDir()/DevOpsPad/applications`，macOS 对应 `~/Library/Application Support/DevOpsPad/applications`。可在“Sharing & sync”选择其他本地目录；最后选择的路径保存在 DevOpsPad 配置目录的 `project-path` 文件。

```text
project.yaml             # 项目名、环境、应用选择器
artifacts/<uuid>.yaml     # 每个镜像制品一份索引
releases/<uuid>.json      # 每次桌面发布一份记录，执行时原子更新
runs/<uuid>.json          # AI 构建/外部发布记录，包含步骤、源码及执行者
```

配置写入先比较原文件的 SHA-256，防止覆盖外部编辑；进程间保存使用独占锁文件。每次写入先落临时文件再 rename，文件权限 0600。若进程在保存中异常退出留下 `.devopspad-write.lock`，检查文件和运行进程后再手动删除锁文件。项目目录应当是本地磁盘上的独立副本，不能作为多机同时写入的网络共享数据库。

两三人协作时：

1. 每人克隆同一个 Git 仓库，并在应用中选择该仓库根目录。也可以使用局域网 Git 服务或 SSH bare 仓库。
2. 各自设置 Git 用户名、邮箱和远端认证。应用使用本机 Git，禁用交互式密码提示。
3. “Show status”显示分支和工作区状态。“Commit and push”只提交有效的 `project.yaml`、制品索引、发布记录、AI 执行记录及这些记录的删除，不包含其他暂存文件。推送失败保留本地提交。
4. “拉取更新”要求干净的工作区，并且仅允许 `git pull --ff-only`。分叉需要在终端合并；应用不会 reset、自动选边或强推。
5. Git 同步是手动的。发布后的记录要推送后其他人才能拉取看到；它不是实时 CRDT 同步，也不会同步 kubeconfig 或系统钥匙串。

YAML 是第一版配置事实来源。CRDT 协同编辑、自动冲突解决和中心同步服务尚未实现。

## AI 执行端接入

通过 `devopspadctl runs record --project <目录> -f <事件.json>` 上报 start / step / finish。成功构建派生制品索引，可直接在桌面发布；外部发布关联成功的 buildId。桌面“AI runs”每 10 秒及窗口获得焦点时读取更新，正在编辑项目配置时不覆盖草稿。协议、JSON Schema、重试约定及 AI 指令模板见 [AI 构建与发布接入规范](ai-execution.md)。

也可用 `devopspadctl cluster inspect --connection <本机连接名称> --namespace <命名空间>` 进行只读验证，用 `cluster attach --connection <名称> --namespace <命名空间> --environment <环境名>` 把此命名空间中发现的负载登记到本地项目。K3s 通过已有 kubeconfig 和可选 SSH 接入；[K3s 官方集群访问说明](https://docs.k3s.io/cluster-access) 说明了本机 kubeconfig 路径及远程访问方式。

首次部署尚不存在的应用时，在 `cluster attach` 后同时添加 `--application <应用名> --selector <Pod 标签选择器>`，即可在已有项目中登记本地映射并开始上报构建记录；此模式不访问或修改集群。相同映射可重复登记，冲突映射会拒绝覆盖。

## 发布行为与边界

桌面发布引擎位于 `internal/release/desktop.go`，与只读查询适配器分离。当前支持 **已有 Deployment 的单容器镜像发布、历史镜像版本回退、扩缩容和重启**，执行后均等待就绪并写入记录。StatefulSet、DaemonSet 可以查看，不能从此入口修改。创建/删除工作负载仍需已有的集群管理流程。

- 计划绑定应用标签、命名空间、Deployment UID、resourceVersion、容器、镜像摘要及读取时的集群端点。计划仅在当前进程内保存，10 分钟失效，并且只能执行一次。
- 生成计划会执行 Kubernetes server-side dry-run。确认后先在目标命名空间取得 `coordination.k8s.io/v1` Lease，然后再次核对 UID/resourceVersion。
- 同一个 Deployment 的 DevOpsPad 客户端使用同一个 Lease 名称；执行期限 180 秒，Lease 有效期 300 秒，无续约。客户端需要时钟同步。异常退出后锁到期才能重试；其他运维工具不会自动遵循此锁，更新仍依赖资源版本校验。
- 每一步写入本地发布记录。新增记录的 `action` 区分 release、rollback、scale、restart；副本操作另记 `beforeReplicas` 与 `replicas`，旧记录缺少 action 时按镜像发布显示。写入记录失败时停止继续执行；已经提交到集群的更新无法因此撤销。
- 镜像更新和重启添加 `devopspad.io/release` Pod 模板注解；扩缩容保持 Pod 模板不变。成功要求控制器观察到新 generation，且 updated、available、total replicas 与期望值一致。
- 超时、连接中断或 Deployment 被其他操作修改会记为未完成，提示核对实际状态，不自动重试或回滚。进程崩溃留下的 running 记录显示“执行中 / 待核实”。

原 `devopspadctl release validate` 继续只校验完整 release spec。桌面这条发布路径尚未覆盖该规范的 ConfigMap、迁移 Job、Nginx、机群串行发布和健康检查步骤。

## 集群权限和可观测

按需给所用身份授予目标命名空间的权限：

| API | 资源 | 操作 |
| --- | --- | --- |
| apps/v1 | deployments、statefulsets、daemonsets | get、list |
| apps/v1 | deployments | update（dry-run 和确认执行） |
| core/v1 | pods | get、list |
| core/v1 | configmaps | get（应用配置按引用读取） |
| core/v1 | pods/log | get |
| core/v1 | events | list |
| coordination.k8s.io/v1 | leases | get、create、update |
| metrics.k8s.io/v1beta1 | pods | list |

缺少某种工作负载的读取权限不会伪装成空列表，页面显示相应警告。资源列表有 500 条上限，超出时明确提示缩小选择器。Pod 分组匹配完整 LabelSelector，包括 matchExpressions。

Deployment 页面只读取负载，Pod、日志和监控页面按需读取 Pod。三类负载并行查询。按应用读取时，优先使用 [Kubernetes Table 内容协商](https://kubernetes.io/docs/reference/using-api/api-concepts/#receiving-resources-as-tables) 获取不含完整对象的轻量索引，再获取可能匹配的对象并验证完整标签；元数据标签和 Pod 模板标签两种匹配方式均保留。索引不支持、筛选范围过宽、列格式未知或只有 list 权限时，回退到完整列表。

同一应用页面内复用进行中的请求和最近结果。15 秒内切回不重复读取，超过 15 秒先显示上次结果并后台刷新；底部标明读取时间。手动刷新、配置变更及发布/扩缩容等操作完成后会使旧结果失效。缓存仅存在当前页面内存中，重新打开应用仍需要连接集群。

监控依赖集群现有 metrics-server，显示 CPU（mCPU）、内存（MiB）与采样时间，尚无历史时序。日志沿用 UID 固定的容器快照接口，支持普通/init/ephemeral 容器、上一实例和 100–2,000 行，最多 256 KiB。可选每 10 秒刷新；它不是 follow 日志流。日志正文及指标不写入项目目录。事件按 Pod UID 查询，避免混入同名旧 Pod 的事件。

接口依据：[Kubernetes Lease](https://kubernetes.io/docs/concepts/architecture/leases/)、[Deployment 就绪状态](https://kubernetes.io/docs/concepts/workloads/controllers/deployment/)、[资源指标](https://kubernetes.io/docs/tasks/debug/debug-cluster/resource-usage-monitoring/)、[HPA 与副本管理](https://kubernetes.io/docs/concepts/workloads/autoscaling/horizontal-pod-autoscale/)。

## 验证范围

Go 测试覆盖 YAML 校验、陈旧配置保存、并发保存锁、Git 两个本地副本同步、无关暂存文件隔离、Pod/指标按应用筛选、服务端 dry-run 参数、发布锁、过期和陈旧计划、rollout 判定与端点固定。浏览器测试通过独立的 Wails 接口测试桩覆盖环境隔离、日志与事件、配置冲突、发布失败，以及无连接时的真实空状态；测试数据不进入产品构建。

新增测试覆盖扩缩容至零且不改变 Pod 模板、重启保持容器配置、无效操作/目标拒绝、跨环境回退拒绝、Wails 单次计划、失败记录落盘，以及浏览器中的扩缩容/重启/回退、环境隔离、主题切换与重载持久化。

这些测试使用临时目录、本地 Git remote、HTTP 协议服务器和模拟集群，不会修改用户的真实环境。真实 Registry 拉取、带工作节点的 Deployment rollout、真实 SSH/集群 RBAC 组合及原生目录选择器仍需要在目标环境验收。

## 宿主机 Nginx

应用映射可配置 `nginxPath: /etc/nginx/conf.d/thumbnailsnap.conf`。在「Nginx config」或「App config」中读取、编辑并保存。环境必须使用已保存的 SSH 连接；远端需要 Python 3、Nginx 和 systemd，SSH 用户须有配置与 reload 权限。

仅允许 `/etc/nginx/conf.d/` 下的普通 `.conf` 文件，拒绝符号链接。读取返回内容摘要，保存校验摘要并加锁，原子替换后执行 `nginx -t` 和 `systemctl reload nginx`，失败恢复原文件。错误时保留编辑草稿，切换项目、环境或应用分别隔离草稿。草稿仅在当前窗口内存中保存。操作结果写入发布记录，记录中不包含配置正文。

外部 AI 上报的 release 记录也显示在「Releases」，按应用与目标环境筛选，保留“External release / Reported”的来源区分，详情可查看发布步骤和对应执行记录。
