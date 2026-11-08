# 03 工作流：CLI、Git、采集、桌面与迁移

## 1. CLI 约定

延续现有 `devopspadcli` 的约定：

- 所有命令支持 `-o json`；成功退出码 0，执行或校验失败 1，参数错误 2。失败时 stderr 输出 JSON `error`。
- **读命令不访问网络**，只读档案库文件。访问线上的只有 `scan`、`config pull/diff --live` 和以后的 Action 命令。
- 不输出密钥值。需要密钥的命令在本机解析 secretRef，用完即丢。
- 定位档案库：`--archive <dir>` > `DEVOPSPAD_ARCHIVE` > 从当前目录向上找 `devopspad.yaml` > DevOpsPad 配置目录中的 `archive-path`。

## 2. 命令一览

| 命令 | 作用 | 阶段 |
| --- | --- | --- |
| `version` | CLI 版本和协议版本（Skill 用来确认兼容） | A1 ✅ |
| `skill install [--scope user\|archive] [--dir d]` / `skill print [file]` | 安装或打印内置的 AI Skill | A1 ✅ |
| `archive init [dir] [--name n]` | 新建档案库：根文件、目录、`.gitignore`、`AGENTS.md`、`INDEX.md`、git init、pre-commit 钩子，并设为默认档案库；对已有档案库（如刚克隆）只重新安装钩子 | A1 ✅ |
| `archive use <dir>` | 设为默认档案库（写入配置目录的 `archive-path`），之后在任何目录都能用 | A1 ✅ |
| `archive validate [--staged] [-o json]` | 按 [02 第 9 节](02-protocol.md#9-校验规则) 校验 | A1 ✅ |
| `archive index` | 重新生成 `INDEX.md`、`AGENTS.md` | A1 ✅ |
| `archive status` / `commit` / `pull` / `push` | Git 操作，见第 3 节 | A1 ✅ |
| `archive import --workspace --fleet --project <dir> [--dry-run]` | 从现有文件生成档案，见第 6 节 | A1 ✅ |
| `archive import --cloudflare [--token-ref r] [--dry-run]` | 把 Cloudflare 所有 zone 导入为 Domain（到期日来自 Registrar 或 RDAP） | A2 ✅ |
| `archive migrate --to <version>` | 协议升级 | v1 时 |
| `get <kind> [name] [-l <selector>]` | 列出 / 读取资源 | A1 ✅ |
| `describe <ref>` | 资源详情 + 解析后的引用 + 被谁引用 + 最近记录 | A1 ✅ |
| `refs <ref>` | 反向引用：谁用了它 | A1 ✅ |
| `schema [kind]` | 输出 JSON Schema | A1 ✅ |
| `new <kind> <name> [--product p]` | 按模板生成骨架文件（带 schema 注释） | A1 ✅ |
| `report matrix` | 产品矩阵总览：阶段、域名、环境、机器、月费 | A1 ✅ |
| `report renewals [--within <days>] [--all]` | 域名 / 证书 / 机器 / 订阅到期 | A1 ✅ |
| `report cost` | 月度费用按币种汇总（不换算），每个产品的独占费用和共享资源 | A1 ✅ |
| `report orphans` | 没有被任何产品引用的机器、域名、账号（可能在浪费钱） | A1 ✅ |
| `secrets check` | 哪些 secretRef 在本机解析不到（不输出值），有缺失时退出码 1 | A1 ✅ |
| `secrets set keychain:<account>` | 把密钥写入 OS 钥匙串（终端隐藏输入，或从 stdin 读取），不进档案库 | A1 ✅ |
| `note add --refs <ref,...> -m <text> [--actor ...]` | 补记人工操作，生成 `action: manual` 记录 | A1 ✅ |
| `connect sync [--dry-run]` | 按档案的集群和数据库生成 / 更新本机 DevOpsPad 连接（按 connection-id 注解或名字匹配，不删除） | A2 ✅ |
| `scan [<ref> ...] [--kind domain\|cluster\|host]` | 域名：Cloudflare zone / DNS / Registrar 到期日、RDAP、HTTPS 证书；集群：节点和工作负载；机器：系统、规格、服务、Nginx 配置哈希。存本机 `.devopspad/observed/`（不进 Git） | A2 ✅ |
| `diff [<ref> ...]` | 期望 vs 观测；有 error 时退出码 1 | A2 ✅ |
| `report renewals --observed` | 到期清单合并扫描到的证书 / 域名到期日 | A2 ✅ |
| `config pull <service>@<env> [--name n]` | 把线上配置文件存档到 `configs/` | A3 |
| `config diff <service>@<env> [--name n]` | 档案配置 vs 线上 | A3 |
| `config push ...` | 档案配置下发到线上（经计划 / 审批 / 记录） | A4 |

示例：

```sh
devopspadcli refs host:hk-1
# KIND     REF                         FIELD
# cluster  cluster:hk-k3s              spec.hosts
# product  product:heartie             spec.environments[pro].hosts
# service  service:heartie/admin       spec.configs[nginx].target.host
# domain   domain:heartie.app          spec.records[@].target

devopspadcli report renewals --within 60
# DUE         KIND      REF                     AUTO  COST
# 2026-12-01  host      host:sg-2               no    CNY 34/month
# 2027-01-02  domain    domain:thumbsnap.app    yes   USD 14/year
```

## 3. Git 工作流

档案库是一个**独立的私有 Git 仓库**（建议，见 [06 Q-A1](06-open-questions.md)），和产品代码仓库分开。

### 提交约定

```text
<type>(<scope>): <summary>

<body>

DevOpsPad-Actor: ai/claude-code
DevOpsPad-Session: b1f3…
DevOpsPad-Action: 01JC8Z6Q3W4XK7N2V5R8T0YB1M
```

| type | 用途 |
| --- | --- |
| `archive` | 期望状态变更（新增产品、改域名、换机器） |
| `config` | `configs/` 中配置文件变更 |
| `record` | 操作记录（通常由引擎自动提交） |
| `note` | 笔记 |
| `migrate` | 协议升级、导入 |

`scope` 用资源引用，例如 `archive(product:heartie): add dev environment`。trailer 用于区分人和 AI 的改动，`git log --grep 'DevOpsPad-Actor: ai/'` 可以查出所有 AI 提交。

### 规则

- `archive commit` 先跑 `validate`，再提交；只暂存档案库管理的路径，不碰其他文件。
- `archive init` 安装 pre-commit 钩子执行 `devopspadcli archive validate --staged`，手工 `git commit` 也会校验。
- `pull` 只允许 `--ff-only`，和现有“Sharing & sync”一致；有分叉时提示在终端处理，不自动合并、不强推。
- 冲突控制：一个资源一个文件；记录 ID 是 ULID，不会冲突；`INDEX.md` 冲突时直接重新生成。
- A1 中 `note add` 只写记录文件，由 `archive commit` 提交；A4 引擎写记录后再考虑自动提交。
- Git 不克隆钩子：新克隆的档案库先运行一次 `devopspadcli archive init .` 安装钩子。
- 一个人多台电脑：桌面和 CLI 在档案库落后远端时提示先 pull。

## 4. 采集与漂移（A2）

`scan` 只读，结果只存本机 `.devopspad/observed/`，不提交。按优先级（产品多、机器少，域名最多）：

| 资源 | 方式 | 采集内容 |
| --- | --- | --- |
| Domain（优先） | RDAP 查询到期日；对每个 endpoint 做 TLS 握手读取证书；DNS 服务商 API（需 secretRef） | 到期日、证书到期日与签发者、DNS 记录 |
| Cluster | 现有 client-go 读取 | 节点、版本、各命名空间工作负载与当前镜像（核对上百个产品的部署是否都登记了） |
| Host | SSH，只执行白名单只读命令 + SFTP 读取 | OS、内核、CPU / 内存 / 磁盘规格、systemd 服务、Nginx conf 文件哈希、k3s 版本 |
| Datastore | 现有只读连接 | 版本、库列表 |

`diff <ref>` 输出期望和观测的差异：

- Host：规格、OS 变化；档案中登记的 Nginx 配置与线上哈希不一致。
- Domain：`records` 与实际 DNS 不一致；证书 30 天内到期。
- Service：线上镜像与最近一条成功发版记录不一致（有人绕过 DevOpsPad 发版）；工作负载不存在。
- 档案没有、线上存在的东西（例如集群里多了一个 Deployment），作为“未登记”列出。

## 5. 桌面

在导航栏增加 **Archive（档案）** 入口：

1. **总览**：产品矩阵表（阶段、域名、环境、机器、月费）、即将到期、漂移、未登记资源、本机缺失密钥。
2. **资源浏览**：按 kind 分组的树；详情页显示字段、正向引用、反向引用、相关记录和观测快照。
3. **编辑**：CodeMirror YAML 编辑器，保存前调用 Go 端 `validate`，错误定位到行；保存沿用现有的 SHA-256 比较、锁文件和原子写入。
4. **Git**：复用现有 Sharing & sync（状态、提交、ff-only 拉取、推送），提交信息自动带 trailer。
5. **连接**：从 Host / Cluster / Datastore 一键生成本机连接，提示输入缺失的密钥（A2）。

Infra 现有页面逐步改为从档案读取环境和应用映射（A4）。

## 6. 从现有文件迁移

`archive import` 只读源文件，生成档案文件，不删除、不修改原文件。可重复执行；目标文件已存在且内容不同时跳过并报告。

| 来源 | 生成 | 需要人工确认的地方 |
| --- | --- | --- |
| `workspace.json` 中带 SSH 的连接 | Host（`ssh` 段、`keyRef` 用原私钥路径） | 机器名、provider、费用、到期日 |
| Kubernetes / K3s 连接 | Cluster | 集群由哪些 Host 组成 |
| MySQL / Redis / Mongo / ClickHouse / Kafka 连接 | Datastore | 归属哪台机器或哪个托管服务 |
| `fleet.json` | Host 的 `labels.fleet` 和 `roles` | — |
| `project.yaml` | Product + Service | 一个项目可能混了多个产品（例如环境叫 `heartie-pro`），按环境名前缀给出分组建议 |
| `artifacts/`、`releases/`、`runs/` | `records/` | —（A4 再做） |

已有的 keychain 密钥不复制：生成的 secretRef 指向原连接的 keychain 条目（`keychain:connection/<id>/<field>`），DevOpsPad 能直接解析。

`--dry-run` 输出将要生成的文件和待确认清单（`-o json`），适合交给 AI：AI 读清单，问人补齐信息，再写文件、校验、提交。
