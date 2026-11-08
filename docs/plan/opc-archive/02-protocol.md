# 02 档案库协议 v1alpha1

状态：A1 已实现（2026-11-07），代码在 `internal/archive`，示例在 `examples/archive/`。本文与实现不一致时，以 `devopspadcli schema <kind>` 和校验器为准。

本文定义档案库的文件格式。目标是**人能读、AI 能写、程序能严格校验**。协议稳定前版本号为 `devopspad.io/v1alpha1`，可能有不兼容修改；A4 完成后定为 `v1`。

## 1. 设计原则

1. **一个资源一个文件**，文件路径由 kind 和名字决定。Git diff 和冲突都按资源隔离。
2. **统一的资源格式**：`apiVersion` / `kind` / `metadata` / `spec`，沿用 Kubernetes 风格，AI 和人都熟悉。
3. **三类数据分开**：

   | 类别 | 目录 | 谁写 | 能否手改 |
   | --- | --- | --- | --- |
   | 期望状态（desired） | `inventory/`、`products/`、`policies/` | 人 / AI | 能，改完必须 `validate` |
   | 观测快照（observed） | `.devopspad/observed/`（本机，不进 Git） | `devopspadcli scan` | 不能，会被覆盖 |
   | 操作记录（records） | `records/` | 执行引擎 / `note` 命令 | 不能，结束后只读 |

4. **只存引用，不存密钥**。凭据统一写成 `secretRef`，值在 keychain / 密码管理器里。
5. **严格校验**：未知字段、悬空引用、疑似密钥都直接失败。自由文本放 `metadata.description`、`metadata.annotations` 或 `notes/`。
6. **全部是纯文本、确定性输出**：YAML 两空格缩进、键顺序固定、列表按名字排序（工具写入时保证），避免无意义的 diff。

## 2. 目录布局

```text
<archive>/
├── devopspad.yaml                       # kind: Archive，根标记
├── AGENTS.md                            # 生成：给 AI 的入口说明，指向 Skill
├── INDEX.md                             # 生成：矩阵总览，便于在 Git 网页上浏览
├── .gitignore                           # 生成：忽略 .devopspad/
├── inventory/                           # 跨产品共享的资源
│   ├── providers/<name>.yaml            # kind: Provider   第三方账号
│   ├── hosts/<name>.yaml                # kind: Host       机器
│   ├── clusters/<name>.yaml             # kind: Cluster    K3s / K8s 集群
│   ├── datastores/<name>.yaml           # kind: Datastore  数据库、缓存、对象存储桶
│   ├── registries/<name>.yaml           # kind: Registry   镜像仓库
│   └── domains/<fqdn>.yaml              # kind: Domain     域名（文件名就是域名）
├── products/<product>/
│   ├── product.yaml                     # kind: Product
│   ├── services/<service>.yaml          # kind: Service
│   ├── configs/<service>/<env>/<file>   # 配置文件原文（不含密钥）
│   └── notes/<slug>.md                  # 该产品的运维笔记 / runbook
├── notes/<slug>.md                      # 全局笔记
├── policies/policy.yaml                 # kind: Policy（A4 / M3 再启用）
├── records/
│   ├── actions/<yyyy>/<mm>/<id>.json    # kind: ActionRecord（引擎生成，只追加）
│   ├── runs/<id>.json                   # 兼容现有 AI 执行记录
│   └── artifacts/<product>/<service>/<id>.yaml   # 兼容现有制品索引
└── .devopspad/                          # 本机状态，不进 Git：observed/ 采集快照、缓存、锁
```

`devopspadcli` 从当前目录向上查找 `devopspad.yaml` 定位档案库（和 Git 找 `.git` 一样），也可以用 `--archive <dir>` 或 `DEVOPSPAD_ARCHIVE` 指定。

## 3. 通用资源格式

```yaml
apiVersion: devopspad.io/v1alpha1
kind: Host
metadata:
  name: hk-1                    # 必填，见下方命名规则
  title: 香港 1 号机             # 可选，显示名
  description: 主力边缘节点，跑 K3s 和 Nginx
  labels:                       # 可选，用于选择器和报表分组
    region: hk
    env: pro
  annotations:                  # 可选，自由键值，不参与校验，放自定义信息
    purchased-via: 双 11 活动
spec:
  ...                           # 各 kind 自己定义
```

### 命名规则

| 适用 | 规则 |
| --- | --- |
| 大多数 kind | `^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`（DNS label），和文件名一致 |
| Domain | 小写 FQDN，例如 `heartie.app`；文件名 `heartie.app.yaml` |
| Service | 在所属产品内唯一；`metadata.product` 必须等于所在目录名 |
| label 键 | `^([a-z0-9.-]+/)?[a-z0-9]([a-z0-9._-]{0,61}[a-z0-9])?$`；值 ≤ 63 字符 |

### 公共类型

```yaml
# Cost：金额用字符串，避免浮点误差
cost: { amount: "34.00", currency: CNY, period: month }    # period: month | year | once

# Renewal：一切会到期的东西
renewal: { expiresAt: 2027-01-10, autoRenew: false }

# 日期：YYYY-MM-DD；时间戳：RFC 3339（带时区）
```

## 4. 引用语法

资源之间只通过**引用字符串**关联，校验时全部解析，悬空引用直接失败。

```text
<kind>:<name>                 host:hk-1   cluster:hk-k3s   provider:cloudflare   domain:heartie.app
service:<product>/<service>   service:heartie/admin
product:<name>                product:heartie
<ref>@<environment>           service:heartie/admin@pro      （记录和 CLI 中用来指定环境）
<ref>#<part>                  datastore:main-mysql#heartie   （数据存储中的某个库 / 桶，必须在 parts 中）
                              registry:ghcr#heartie-admin    （镜像仓库中的某个 repository，不预先登记）
```

kind 一律小写。每个引用字段在 Schema 中声明允许的 kind，例如 `Cluster.spec.hosts` 只接受 `host:`。

**反向引用不存储**：“hk-1 上跑了什么”由工具实时计算（`devopspadcli refs host:hk-1`），避免两处维护不一致。

## 5. 密钥引用（secretRef）

凡是凭据，只能出现在以 `Ref` 结尾的字段或 `credentials[].ref` 中，值是一个 URI：

| 形式 | 含义 | 阶段 |
| --- | --- | --- |
| `keychain:<account>` | OS 钥匙串，服务名 `devopspad`（与桌面共用）。新密钥建议账户名 `archive/<kind>/<name>/<用途>`，用 `devopspadcli secrets set keychain:<account>` 写入；导入的桌面连接引用原条目，如 `keychain:<连接 ID>`、`keychain:<连接 ID>:ssh-password` | A1 |
| `env:<NAME>` | 本机环境变量 | A1 |
| `file:<path>` | 本机文件路径（如 SSH 私钥），绝对路径或 `~/`；只存路径，不读内容入库 | A1 |

不接入密码管理器，不做加密文件入库（[06 Q-A2](06-open-questions.md)）。

规则：

- DevOpsPad 解析 secretRef 只在执行时、在本机进行；值不写入档案库、记录、日志或 AI 的输出。
- `devopspadcli secrets check` 只报告“哪些引用在本机解析不到”，不输出值。
- 配置文件里需要密钥时，写占位符 `${secret:<ref>}`，例如 `password = "${secret:keychain:archive/heartie/api/db-password}"`。下发时（A4）才渲染。

## 6. 资源类型

### Archive（根）

```yaml
apiVersion: devopspad.io/v1alpha1
kind: Archive
metadata:
  name: askuy-opc
spec:
  defaults:
    renewalWarningDays: 30      # report renewals 的默认天数
```

### Provider（第三方账号）

```yaml
apiVersion: devopspad.io/v1alpha1
kind: Provider
metadata:
  name: cloudflare
spec:
  categories: [dns, cdn, registrar]   # cloud | registrar | dns | cdn | storage | payment | email
                                      # | appstore | analytics | ai | ci | registry | monitoring | saas | other
  console: https://dash.cloudflare.com
  account: ops@example.com            # 登录标识，不是密钥
  mfa: 手机验证器                     # 两步验证放在哪
  plan: free
  cost: { amount: "0", currency: USD, period: month }
  renewal: null
  credentials:
    - name: dns-token
      ref: keychain:archive/providers/cloudflare/dns-token
      scope: Zone.DNS edit, all zones
```

### Host（机器）

```yaml
apiVersion: devopspad.io/v1alpha1
kind: Host
metadata:
  name: hk-1
  labels: { region: hk, env: pro }
spec:
  provider: provider:aliyun
  state: active                       # active | standby | retired
  addresses: { public: 203.0.113.10, private: 10.0.0.2 }
  ssh:
    host: 203.0.113.10
    port: 22
    user: root
    auth: key                         # key | agent | password
    keyRef: file:~/.ssh/id_ed25519    # auth=password 时用 passwordRef
    hostKey: SHA256:AbCd...           # 可选；不填则用 known_hosts
  os: ubuntu-22.04
  size: { cpu: 2, memoryGiB: 4, diskGiB: 60 }
  roles: [k3s-server, nginx, edge]    # 自由标签，但 nginx / k3s-server / docker 会启用对应采集
  cost: { amount: "34.00", currency: CNY, period: month }
  renewal: { expiresAt: 2027-01-10, autoRenew: false }
```

`ssh` 段是建立本机 DevOpsPad 连接所需的全部**非密钥**参数。新电脑上可以由此生成连接（A2），再补密钥。

### Cluster（集群）

```yaml
apiVersion: devopspad.io/v1alpha1
kind: Cluster
metadata:
  name: hk-k3s
spec:
  distribution: k3s                   # k3s | k8s
  hosts: [host:hk-1]
  # tunnel: host:hk-1                 # 经这台机器的 SSH 访问 API Server
  kubeconfig:
    source: remote                    # remote：经 SSH 从 host 读取；local：本机文件
    host: host:hk-1
    path: /etc/rancher/k3s/k3s.yaml
    context: default
  # source: local 时：
  #   pathRef: file:~/.kube/hk.yaml
  #   context: hk
```

`remote` 方式与现有 fleet 的远端 kubeconfig 读取一致（SFTP、≤ 1 MiB、拒绝 exec 插件）。

### Datastore（数据存储）

```yaml
apiVersion: devopspad.io/v1alpha1
kind: Datastore
metadata:
  name: main-mysql
spec:
  engine: mysql                       # mysql | postgres | redis | mongodb | clickhouse | kafka | s3 | other
  runtime:                            # 三选一
    host: host:hk-1                   # 宿主机上
    # cluster: cluster:hk-k3s, namespace: db     # 集群里
    # provider: provider:aliyun, managed: rds    # 托管服务
  # tunnel: host:hk-1                 # 经这台机器的 SSH 连接
  endpoint: 10.0.0.2:3306
  parts: [heartie, thumbsnap]         # 库 / 桶，供 datastore:main-mysql#heartie 引用
  credentials:
    - { name: admin, ref: keychain:archive/datastores/main-mysql/admin }
  backup:
    method: mysqldump-cron            # 自由文本，描述怎么备份
    schedule: "0 3 * * *"
    target: provider:r2
    retentionDays: 14
```

### Registry（镜像仓库）

```yaml
apiVersion: devopspad.io/v1alpha1
kind: Registry
metadata:
  name: ghcr
spec:
  url: ghcr.io/askuy
  provider: provider:github
  credentials:
    - { name: push, ref: env:GHCR_TOKEN }
```

### Domain（域名）

```yaml
apiVersion: devopspad.io/v1alpha1
kind: Domain
metadata:
  name: heartie.app
spec:
  registrar: provider:namecheap
  dns: provider:cloudflare
  cost: { amount: "14.00", currency: USD, period: year }
  renewal: { expiresAt: 2027-05-01, autoRenew: true }
  records:                            # 可选，只登记关键解析，用于反查和漂移检测
    - { name: "@", type: A, target: host:hk-1, proxied: true }   # target 可以是 host 引用
    - { name: admin, type: CNAME, value: heartie.app }
  certificates:
    - names: [heartie.app, "*.heartie.app"]
      issuer: letsencrypt
      managedBy: host:hk-1            # 谁负责签发 / 续期：host（certbot / acme.sh）或 provider
      method: acme.sh dns-01
```

`records` 不是 DNS 的完整镜像。完整记录由 `scan` 从 DNS 服务商读到 `.devopspad/observed/domains/`（A2）。

### Product（产品）

```yaml
apiVersion: devopspad.io/v1alpha1
kind: Product
metadata:
  name: heartie
  title: Heartie
  labels: { line: consumer, revenue: paid }
spec:
  stage: live                         # idea | building | live | paused | sunset
  launchedAt: 2026-03-01
  links:
    site: https://heartie.app
    repo: https://github.com/askuy/heartie
    analytics: https://plausible.io/heartie.app
  domains: [domain:heartie.app]
  providers:                          # 本产品用到的账号及用途；成本分摊用
    - { ref: provider:stripe, purpose: 收款 }
    - { ref: provider:resend, purpose: 事务邮件 }
  environments:
    - name: pro
      cluster: cluster:hk-k3s
      namespace: default
      hosts: [host:hk-1]              # 本环境用到的宿主机（Nginx、宿主机进程）
    - name: dev
      cluster: cluster:hk-k3s
      namespace: heartie-dev
```

费用不做分摊：`report cost` 列出每个产品独占的费用，以及与其他产品共享的资源。

### Service（服务）

```yaml
apiVersion: devopspad.io/v1alpha1
kind: Service
metadata:
  name: admin
  product: heartie                    # 必须等于所在目录名
spec:
  type: web                           # web | api | worker | cron | static | other
  repo:
    url: git@github.com:askuy/heartie.git
    path: admin                       # monorepo 子目录，可选
    defaultBranch: main
  build:
    method: ci                        # ci | local | none
    ci: { provider: provider:github, workflow: .github/workflows/admin.yml }
    image: registry:ghcr#heartie-admin
  release:
    method: devopspad                 # devopspad | devopspad-spec | ci | manual
    spec: deploy/service.release.yaml # method=devopspad-spec 时，相对代码仓库路径
    notes: notes/release-admin.md     # 发版注意事项，AI 发版前必读
  deployments:
    - environment: pro                # 必须是 Product.spec.environments 中的名字
      runtime: k8s                    # k8s | host | static
      selector: app=heartie-admin     # cluster / namespace 继承自环境，可覆盖
      workload: { kind: Deployment, name: heartie-admin, container: app }
    # - environment: pro
    #   runtime: host
    #   hosts: [host:hk-2]
    #   unit: heartie-worker.service  # systemd unit 或 compose 文件路径
  endpoints:
    - { url: https://admin.heartie.app, health: /healthz }
  configs:
    - name: nginx
      type: nginx                     # nginx | configmap | env-file | systemd | file
      environment: pro
      source: configs/admin/pro/heartie-admin.conf        # 相对产品目录
      target: { host: host:hk-1, path: /etc/nginx/conf.d/heartie-admin.conf }
    - name: app
      type: configmap
      environment: pro
      source: configs/admin/pro/config.toml
      target: { configMap: heartie-admin-config, key: config.toml }   # 集群 / 命名空间继承自部署
  dependsOn: [datastore:main-mysql#heartie, service:heartie/api]
  secrets:                            # 服务运行需要的密钥（只登记引用和用途）
    - { name: DB_PASSWORD, ref: keychain:archive/heartie/api/db-password }
```

### Note（笔记）

Markdown 文件，可选 YAML frontmatter，不做严格校验，只检查引用和密钥：

```markdown
---
refs: [service:heartie/admin, host:hk-1]
date: 2026-11-07
tags: [incident]
---
# admin 502 排查
...
```

### Policy

沿用 [../06 第 5 节](../06-ai-devops-control-plane.md) 的设计，A4 / M3 启用。A1 阶段存在该文件时只做格式校验。

## 7. 操作记录（ActionRecord）

所有对线上产生影响的操作，以及人工补记的操作，都写成一条记录：

```json
{
  "apiVersion": "devopspad.io/v1alpha1",
  "kind": "ActionRecord",
  "id": "01JC8Z6Q3W4XK7N2V5R8T0YB1M",
  "action": "config.set",
  "targets": ["service:heartie/admin@pro"],
  "actor": { "type": "ai", "name": "claude-code", "session": "b1f…", "machine": "mbp-askuy" },
  "approver": { "type": "human", "name": "askuy", "at": "2026-11-07T10:02:11+08:00" },
  "archiveRevision": "4c3100f",
  "plan": { "digest": "sha256:…", "summary": "config.toml +3 -1" },
  "status": "succeeded",
  "message": "调大 admin 超时",
  "steps": [
    { "name": "dry-run", "status": "succeeded", "at": "2026-11-07T10:02:12+08:00" },
    { "name": "update", "status": "succeeded", "at": "2026-11-07T10:02:13+08:00", "message": "resourceVersion 81234 → 81240" }
  ],
  "startedAt": "2026-11-07T10:02:12+08:00",
  "finishedAt": "2026-11-07T10:02:20+08:00",
  "links": { "run": null, "artifact": null }
}
```

规则：

- `id` 用 ULID（按时间排序），路径 `records/actions/<yyyy>/<mm>/<id>.json`。
- A1 只有 `devopspadcli note add` 写记录（`action: manual`）；引擎写记录在 A4。
- `action` 取值：`release.image`、`release.rollback`、`workload.scale`、`workload.restart`、`config.set`、`nginx.set`、`manual`（人工补记）、以后的 `ci.trigger`、`host.service` 等。
- `actor.type`：`human` | `ai`。AI 必须写名字和会话 ID。
- `archiveRevision`：生成计划时档案库的 Git 提交，用于事后对照“当时的期望状态”。
- 状态到达 `succeeded` / `failed` / `cancelled` / `unknown` 后**不可再改**；`running` 期间由引擎原子更新。校验会拒绝对已结束记录的修改和删除。
- 不保存配置正文、日志正文和密钥；只保存摘要、哈希、行数、资源版本。单条 ≤ 64 KiB。
- 现有 `releases/*.json`、`runs/*.json` 在 A4 前继续按原格式读取，`devopspadcli archive import` 可转换（见 [03](03-workflows.md#从现有文件迁移)）。

## 8. 观测快照（Observation）

`devopspadcli scan` 生成，A2 先实现域名：

```json
{
  "apiVersion": "devopspad.io/v1alpha1",
  "kind": "Observation",
  "ref": "domain:heartie.example",
  "observedAt": "2026-11-07T09:00:00+08:00",
  "collector": "devopspadcli/0.2.0",
  "domain": {
    "expiresAt": "2027-06-01",
    "expirySource": "cloudflare-registrar",
    "autoRenew": true,
    "registrar": "provider:cloudflare",
    "dns": "provider:cloudflare",
    "zoneStatus": "active",
    "nameServers": ["ada.ns.cloudflare.com"],
    "records": [{ "name": "@", "type": "A", "content": "203.0.113.10", "proxied": true }],
    "certificates": [{ "host": "admin.heartie.example", "notAfter": "2027-01-30", "issuer": "Let's Encrypt", "valid": true }],
    "errors": []
  }
}
```

- 存放在 `.devopspad/observed/domains/<fqdn>.json`（Cloudflare zone 列表在 `providers/cloudflare.json`），**不提交到 Git**（[06 Q-A3](06-open-questions.md)）：每台电脑需要时自己 `scan`；`scan` 会确保 `.gitignore` 包含 `.devopspad/`，`validate` 缺少时警告。
- 每个资源一个文件，整文件覆盖。到期日来源：Cloudflare Registrar（需要时读取，失败不影响其他数据）→ 公开 RDAP。证书：服务 endpoints 中属于该域名的 HTTPS 主机，没有则检查根域名；证书过期或不匹配也会读出日期。
- 发现值得留档的变化（证书已续期、DNS 被改），由人或 AI 按 `devopspadcli diff` 更新期望状态或补一条记录，而不是提交快照。

## 9. 校验规则

`devopspadcli archive validate` 按顺序检查，任何错误导致退出码 1：

1. 根目录有 `devopspad.yaml`，`apiVersion` 是支持的版本。
2. 已知目录下每个 YAML 能解析，`kind` 与目录匹配，`metadata.name` 与文件名一致（Service 还要校验 `metadata.product`）。
3. 字段严格匹配 Schema，**未知字段失败**。
4. 名字、label、日期、金额、URL、枚举值格式正确。
5. 所有引用可解析，且 kind 符合字段声明；datastore 的 `#part` 存在于目标的 `parts`；`@environment` 存在于产品环境。
6. `Service.spec.deployments[].environment` 存在；`dependsOn` 中的服务依赖无环。
7. `configs[].source` 存在、位于档案库内（拒绝绝对路径、`..`、符号链接），单文件 ≤ 1 MiB。
8. **密钥扫描**（覆盖 YAML、配置文件、笔记）：
   - 字段名形如 `password`、`token`、`secret`、`apiKey`、`privateKey`、`accessKey` 且不以 `Ref` 结尾、值不是 secretRef → 失败。
   - 内容匹配 PEM 私钥、kubeconfig 证书数据、带密码的 DSN（`://user:pass@`、`@tcp(`）、常见令牌前缀（`AKIA`、`ghp_`、`github_pat_`、`sk-`、`xox`、`glpat-`）、JWT → 失败。
   - 配置文件（`configs/`）中 `KEY = value` 形式的赋值，KEY 以 password / token / secret / api_key 等结尾（含 `DB_PASSWORD` 这类前缀）→ 失败；`${secret:…}` 占位符不算。
   - 格式错误、被丢弃的文件同样扫描；错误信息不回显密钥内容。
   - 误报：在该行加注释 `devopspad:allow-secret`。
9. `records/` 中已结束的记录未被修改或删除（与 Git HEAD 比较；`--staged` 时与暂存区比较）。
10. 单个 YAML ≤ 256 KiB；记录 ≤ 64 KiB。
11. 警告（不失败）：已过期的 `renewal`、`INDEX.md` / `AGENTS.md` 过期。

输出两种格式：默认人类可读；`-o json` 给 AI 和 CI 用：

```json
{ "ok": false, "errors": [
  { "file": "products/heartie/services/admin.yaml", "path": "spec.deployments[0].environment",
    "line": 9, "code": "ref.environment",
    "message": "environment \"staging\" is not defined in product:heartie; defined: pro, dev" }
], "warnings": [] }
```

错误信息要写出**可选值或修正方式**，方便 AI 自己修。错误码一览见 `skills/devopspad/reference/protocol.md`。

`--staged` 校验 Git 暂存区（导出到临时目录后校验），`archive init` 安装的 pre-commit 钩子就是调用它。

## 10. Schema 与版本

- JSON Schema 由 Go 类型和其 `schema` 标签在运行时生成（`devopspadcli schema <kind>`），与校验器同源，不单独维护 Schema 文件。跨字段规则和引用是否存在不在 Schema 里，由 `validate` 检查。
- 编辑器集成（YAML LSP 的 `$schema` 注释）以后再做。
- 升级时 `devopspadcli archive migrate --to <version>` 改写文件，单独一个提交。旧版本至少保留一个大版本的只读兼容。

## 11. 与现有文件的关系

| 现有 | 位置 | 档案库中的对应 | 处理 |
| --- | --- | --- | --- |
| `workspace.json` 连接 | 本机配置目录 | Host（`ssh`）、Cluster、Datastore | 连接保留在本机；档案描述“应该有哪些连接”，A2 起可双向同步 |
| keychain 密钥 | 本机 | secretRef `keychain:` | 不变 |
| `project.yaml`（`devopspad.project/v1`） | 应用项目目录 | Product.environments + Service.deployments | `archive import` 转换；A4 前两者并存 |
| `fleet.json` | 本机配置目录 | Host + labels / roles | `archive import` 转换；fleet = label 选择器 |
| `service.release.yaml`（`devopspad.release/v1`） | 服务代码仓库 | Service.release.spec 引用 | 不搬迁，仍是代码仓库里的发版事实来源 |
| `artifacts/`、`releases/`、`runs/` | 应用项目目录 | `records/` | A4 统一；之前原样读取 |
