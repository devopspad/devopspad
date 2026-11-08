# 05 分期与验收

原则：**先文件，后线上**。A0–A1 只读写档案库文件、不访问线上，风险低，可以插在 [M1 Action 引擎](../04-roadmap-draft.md) 之前做；它同时确定了 M1 需要的统一领域模型。

## A0 协议定稿 ✅（2026-11-07）

- [x] 确认 [06](06-open-questions.md) 中的决策项，回写 [02](02-protocol.md)。
- [x] Go 结构体（`internal/archive/types.go`）作为唯一来源：`schema` 标签同时驱动校验和 `devopspadcli schema <kind>` 的 JSON Schema。
- [x] 完整示例档案库 `examples/archive/`：2 个产品、4 个服务、3 台机器、1 个集群、2 个域名、6 个账号、1 个数据库、1 个镜像仓库、配置文件和笔记。
- [x] 错误码及其测试样例（`internal/archive/archive_test.go` 的 `TestValidationCodes`，在示例副本上逐项改坏）。

## A1 档案库 MVP ✅（2026-11-07，不访问线上）

- [x] `internal/archive`：加载、严格解析、引用解析、反向引用、校验、密钥扫描、报表、导入、Git、记录。
- [x] CLI：`archive init/use/validate/index/status/commit/pull/push/import`、`get`、`describe`、`refs`、`schema`、`new`、`report matrix/renewals/cost/orphans`、`secrets check/set`、`note add`、`skill install/print`、`version`。
- [x] `archive import`：`workspace.json`、`fleet.json`、`project.yaml` → 档案（`--dry-run` + 待补齐清单，可重复执行）。
- [x] pre-commit 钩子（校验暂存区）；提交 trailer `DevOpsPad-Actor` / `DevOpsPad-Session`。
- [x] Skill v1：`skills/devopspad/`（SKILL.md + 8 个 workflow + 协议速查），嵌入 CLI。
- [x] 文档：[`docs/archive.md`](../../archive.md)，README 增加入口。

**已验证**（自动测试 + 临时目录手工走查）：示例档案库零错误零警告；28 个错误码样例；格式错误文件里的密钥照样报出且不回显；`DB_PASSWORD=…` 这类带前缀的赋值被拦截；pre-commit 钩子拦截明文密钥；已结束记录被改或删会被拒绝（工作区和暂存区）；导入可重复执行；`secrets set` 不输出密钥值。

**待你在真实环境验收**：

1. 用 `archive import` + 人工补齐，把现有全部产品、机器、域名、账号登记进档案库，`validate` 通过。
2. `report matrix`、`report renewals`、`report cost` 的结果与实际一致。
3. 在另一台电脑克隆档案库，`archive init .` 装钩子，`secrets check` 准确列出缺失的密钥。
4. [04 评测用例](04-ai-skill.md#评测用例) 1–8 在 Claude Code 中符合预期（需要先 `devopspadcli skill install`）。

## A2 采集、连接同步与桌面档案 ✅（2026-11-07）

- [x] `scan` 域名部分（2026-11-07）：Cloudflare zone、DNS 记录、Registrar 到期日 / 自动续费（读不到时用 RDAP），每个 HTTPS endpoint 的 TLS 证书；8 路并发；结果只存本机 `.devopspad/observed/`，并确保 `.gitignore` 忽略它。
- [x] `diff` 域名部分：到期 / 自动续费 / 注册商 / DNS 服务商不一致、关键 DNS 记录指向变化、证书过期或 21 天内到期、Cloudflare 有但档案没有的 zone。
- [x] `archive import --cloudflare`：一次导入全部 zone。
- [x] `report renewals --observed`。
- [x] `scan` 集群：节点、全部命名空间的 Deployment / StatefulSet / DaemonSet（分页，最多 5,000 个）；远端 kubeconfig 经 SSH 读到临时目录，用完删除。`diff`：档案部署找不到、未就绪、集群里有档案没有的工作负载。
- [x] `scan` 机器：一段固定的只读命令（系统、规格、systemd 服务、Nginx 配置哈希、k3s 版本）。`diff`：Nginx 配置与档案副本不一致或未存档、角色对应服务未运行、规格未登记。
- [x] `workspace.SSHOutput` / `SSHReadFile`：复用现有 SSH 认证和主机密钥校验的只读辅助函数。
- [x] `connect sync`：档案的集群和数据库 → 本机 DevOpsPad 连接，按 connection-id 注解或名字匹配，只新建 / 更新；密钥缺失时照样创建并提示。Datastore 增加 `username`、`tls` 字段。
- [x] 桌面 Archive 入口（导航栏第三项）：总览、按类型浏览和搜索、资源详情（YAML 编辑 + 保存后校验、引用 / 被引用、部署、记录、扫描结果）、Git 面板、Scan 和 Sync connections；无档案库时可新建 / 打开。Playwright 覆盖。

**验收**：所有产品域名的真实到期日和证书到期日能自动核对；集群里未登记的工作负载能列出；新电脑上“克隆 → 补密钥 → connect sync”后 Infra / Data 页面可直接使用。

## A3 配置归档 ✅（2026-11-08）

- [x] `config pull` / `config diff`：Nginx 和其它宿主机文件（SSH 只读）、ConfigMap 键（集群读取）。
- [x] `${secret:…}` 占位符只在本机渲染；比较时换回占位符、其余像密钥的值显示为 `***`；`pull` 遇到其它密钥拒绝写入。

## A4 并入执行路径 ✅ 核心部分（2026-11-08）

- [x] `config push`：Nginx（远端脚本，revision + `nginx -t` + 恢复）和 ConfigMap 键（dry-run + resourceVersion），渲染占位符，写 `config.push` 档案记录。
- [x] 操作记录统一：桌面和 CLI 所有写操作结束时同时写入档案库 `records/actions/`（按集群连接名 + 命名空间 + 选择器匹配服务），`DEVOPSPAD_ACTOR` 标明 AI；`archive import --records` 导入旧记录（幂等）。Cloudflare 路由 / DNS 和 `config push` 直接写档案记录。
- [x] `archive project [--write]`：由档案生成桌面 `project.yaml`，Infra、`release`、`config`、`nginx` 命令以档案为唯一来源（配合 `connect sync`）。
- [ ] `fleet.json` 改为 Host 标签选择器：只有 2 台机器，和 M4 一起降为最低优先级。
- [ ] 协议定为 `v1`：在真实矩阵上使用一段时间、字段稳定后再定。

## 与现有路线的关系

```text
M0 入库现有代码
 └─ A0 协议定稿
     └─ A1 档案库 MVP + Skill v1        ← 新增，建议优先
         ├─ M1 Action 引擎 + A4 并入    ← A1 的领域模型就是 M1 的输入
         ├─ A2 采集 / 连接同步          ← 可与 M2（AI 只读）并行，scan 结果正好是 AI 的上下文
         └─ A3 配置归档
             └─ M3 策略审批 → M5 CI（M4 机群只有 2 台机器，降为最低优先级）
```

## 代码落点

| 位置 | 内容 |
| --- | --- |
| `internal/archive/` | 模型、加载、校验、引用、报表、导入；不依赖 Wails |
| `internal/archive/schema/` | 生成的 JSON Schema（embed） |
| `internal/archive/scan/` | 采集器（A2），复用 `internal/workspace` 的 SSH 和集群读取 |
| `cmd/devopspadcli/archive*.go` | CLI 子命令 |
| `app_archive.go` + `frontend/src/archive/` | 桌面绑定和界面（A2）；新界面不再加进 `OpsConsole.tsx` |
| `skills/devopspad/` | Skill 源文件 |
| `examples/archive/` | 示例档案库，同时作为测试夹具 |

## 风险

| 风险 | 应对 |
| --- | --- |
| 建模过度，填档案太累 | 除名字和引用外字段都可选；`import` 和 AI 负责填写；先登记能回答四个痛点的字段 |
| 密钥泄露进 Git | 只存引用；字段名 + 内容双重扫描；pre-commit 钩子；私有仓库；可再叠加 gitleaks |
| 档案过时，变成没人信的文档 | `scan` / `diff` 定期对照；`diff` 显示快照采集时间；AI 改线上后必须同步档案 |
| 过渡期两套来源（`project.yaml` 与档案） | A4 前档案只做补充，`project.yaml` 仍为桌面发版依据；A4 一次切换，保留只读兼容 |
| 协议频繁变更 | v1alpha1 期间提供 `archive migrate`；Skill 与 CLI 同版本 |
