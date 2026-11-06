# 04 路线草案

按 [03](03-positioning.md) 确定的方向排列。时间只表示顺序，不是承诺。

原则：**先有统一的执行引擎和策略，再给 AI 写权限**；每个里程碑都要打通一条可在真实环境验收的完整链路。

## M0 入库与对齐（约 1 周）

- [ ] 将未提交的 Infra 工作拆成逻辑提交入库。
- [ ] 确认 CI 在 GitHub 托管 runner 上跑通。
- [ ] 重写 PLAN.md 的产品目标（以 03 为准），删除 demo 相关条目，旧分期移到附录。
- [ ] 在 README 中如实描述当前的三条写路径。

## M1 Action 引擎 + 统一模型（约 3 周）

见 06 第 1 节。

- [ ] 统一领域模型：Connection / Host / Environment / Service / Target（05-Q3）。
- [ ] `internal/action`（或扩展 `internal/release`）：Plan → Approve → Apply → Verify → Record。
- [ ] 将桌面发版（image / scale / restart / rollback）、ConfigMap 写入、Nginx 写入全部迁入引擎。
- [ ] 统一记录格式 `actions/`，兼容读取旧的 `releases/` 和 `runs/`。
- [ ] 测试约束：引擎之外不得调用写 API。
- [ ] 解决 Nginx 实现与规范的矛盾（05-Q4）。

**验收**：桌面上所有写操作都能看到计划、diff，并生成统一记录；行为与现在一致。

## M2 AI 只读接入（约 2 周）

先让 AI “看得见”，此时还不能写。

- [ ] `infradexctl mcp`：查询类工具（集群、日志、事件、指标、机器信息、记录、中间件只读查询）。
- [ ] 输出预算与 secret 过滤。
- [ ] 桌面内置助手改为 tool calling，复用同一组工具。
- [ ] 在真实 K3s 上验收：AI 能定位一个故障 Pod 的原因。

## M3 策略、审批与 AI 写操作（约 2–3 周）

- [ ] `policy.yaml`：按环境和动作给 AI 分级授权、设置审批要求和频率限制。
- [ ] 计划类和执行类 MCP 工具，以及 `action.wait` 审批等待。
- [ ] 桌面审批面板 + 系统通知；CLI `infradexctl approve`。
- [ ] 记录 actor（人 / AI + 会话）和 approver。

**验收**：AI 在 dev 环境自动发版；在 prod 环境生成计划，人在桌面审批后执行。

## M4 机器与机群（约 3 周）

- [ ] Host 连接类型 + 标签；机群 = 标签选择器；迁移 `fleet.json`。
- [ ] 主机事实采集（OS、负载、磁盘、服务、k3s 状态）。
- [ ] 结构化主机动作：systemd 服务、Nginx、限定目录文件；白名单命令；任意 shell 默认关闭。
- [ ] 机群执行：串行 / 分批 / canary、fail-fast、逐台摘要。
- [ ] 桌面机群总览和漂移检测（OPC 第 3 期）。
- [ ] OPC 发版规范的完整 plan / apply（OPC 第 2、4 期）作为一种多步 Action。

## M5 CI 控制（约 2–3 周）

- [ ] CI 适配器接口；第一个适配器取决于 05-Q10。
- [ ] 触发、跟踪、日志、取消；构建完成后自动生成制品（不再依赖自报）。
- [ ] 完整链路：AI 改代码 → `ci.trigger` → 制品 → `release.plan` → 审批 → 机群 apply → 验证。
- [ ] 可选：在指定 Host 上本地构建（buildx），作为无 CI 场景的适配器。

M4 和 M5 可以对调，取决于哪个更痛（05-Q11）。

## 以后再说

- `infradexd` 常驻进程：定时任务、webhook、远程 / 手机审批。
- 记录哈希链 / 防篡改审计。
- 中间件数据编辑（原 P2）、实时日志流、Pod exec。
- 商业化门槛：签名、授权、安装升级。

## 持续项

- 前端拆分：M1 时拆 `OpsConsole.tsx`（审批面板、机群视图都要在这里加）。
- 每个里程碑都在带 worker 节点的真实 K3s + Nginx 机器上做端到端验收。
- 每个里程碑结束时更新 PLAN.md 和 README。
