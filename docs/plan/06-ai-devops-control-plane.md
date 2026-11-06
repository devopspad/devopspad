# 06 AI 驱动的 DevOps 控制面（架构草案）

日期：2026-11-06

## 目标

让 AI（Claude Code、Codex 等编码代理，以及桌面内置助手）能**安全地**控制完整的 DevOps 链路：

```text
代码 → CI 构建 / 测试 → 制品 → 发版计划 → 群控机器执行 → 验证 → 记录 / 回退
```

人负责设定边界和审批，AI 负责编排和执行，Infradex 负责**能力、策略、执行和记录**。

## 和现状的关键区别

| | 现状 | 目标 |
| --- | --- | --- |
| AI 的角色 | 外部执行者，自己调 docker / kubectl，再向 Infradex **自报**记录 | 通过 Infradex 的工具执行，记录由 Infradex **自动生成**，可信 |
| CI | 不涉及，只接收构建结果 | 触发、观察、取消、读取日志，产物自动入制品库 |
| 机器 | 只通过 K3s 连接间接访问；Nginx 单文件读写 | 机器是一等对象，可分组、批量执行结构化动作 |
| 写操作 | 三条分散的写路径 | 统一的 Action 引擎：计划 → 策略 → 审批 → 执行 → 记录 |

## 分层

```text
┌──────────────────────────────────────────────────────────┐
│ 入口: 桌面 UI │ infradexctl CLI │ MCP Server（给 AI） │ 桌面内置 AI │
├──────────────────────────────────────────────────────────┤
│ 策略与审批 Policy: 风险分级、环境规则、审批、预算/频率限制      │
├──────────────────────────────────────────────────────────┤
│ Action 引擎: Plan → Approve → Apply → Verify → Record       │
├────────────┬────────────┬─────────────┬──────────────────┤
│ CI 适配器   │ 机器 / 机群  │ K8s / K3s   │ 观测（日志/指标/事件）│
├────────────┴────────────┴─────────────┴──────────────────┤
│ 连接 + 凭据（workspace.json + keychain）、项目目录（Git 共享） │
└──────────────────────────────────────────────────────────┘
```

**所有入口调用同一个 Action 引擎**。桌面、CLI、AI 走同一条路径，权限和记录完全一致。这是对 [02 第 2、4 项](02-issues-and-risks.md) 的根本解法。

## 1. Action 引擎（核心）

每个写操作都是一个 Action：

```text
Action {
  kind:      image | scale | restart | rollback | configmap | nginx | migrate
             | ci.trigger | ci.cancel | host.service | host.file | host.exec ...
  targets:   [Target]          # 一台机器、一个集群、一个 CI 仓库
  plan:      读取到的现状 + 目标变更 + diff + 校验结果（dry-run）
  risk:      read | low | medium | high | critical（由策略计算）
  actor:     human | ai(name, session)
  approval:  auto | pending | approved(by, at) | rejected
  result:    逐 target 的步骤、输出摘要、错误
}
```

- **Plan 不产生副作用**；Apply 只接受未过期、未被篡改的 Plan ID（沿用现有 DesktopPlan 的 10 分钟单次使用设计）。
- 机群执行支持串行 / 分批（canary：先 1 台，再 N 台）、fail-fast、`not-started` 状态。
- 每个 Action 自动写入记录（扩展现有的 `releases/`、`runs/`，统一为 `actions/`），AI 不再需要自报。

## 2. 机器与机群

现在没有“机器”这种连接类型，机器只能挂在 K3s 连接的 SSH 配置上。需要：

- 新增 **Host 连接**：SSH 地址、认证、标签（`role=edge`、`region=hk`）。复用现有 SSH 隧道代码和 keychain。
- **机群 = 标签选择器**（例如 `env=prod,role=edge`），不再维护静态名单；`fleet.json` 迁移为标签。
- 主机上的能力分三档，AI 默认只能用前两档：

| 档位 | 示例 | 说明 |
| --- | --- | --- |
| 结构化动作 | systemd 服务状态/重启、Nginx 配置 test/reload、文件读写（限定目录）、磁盘/内存/进程 | 有明确 Plan 和 diff，可自动验证 |
| 白名单命令 | `docker ps`、`k3s kubectl get ...`、`journalctl -u x -n 200` | 只读或低风险，参数受限 |
| 任意 shell | 任意命令 | 默认关闭；开启后每次都必须人工审批，记录完整命令和输出摘要 |

- 采集：定期（或按需）拉取每台机器的基本信息（OS、负载、磁盘、服务、k3s 状态），作为 AI 的上下文。
- 无 Agent：继续只用 SSH，不在目标机器上装常驻程序（保持“零服务端”的卖点）。规模超过几十台时再评估。

## 3. CI 控制

现有 run 协议的思路是“AI 自己构建，然后上报”。CI 控制把构建交给 CI 系统，由 Infradex 编排：

- **适配器接口**：`ListPipelines / Trigger(ref, inputs) / Status / Logs / Cancel / Artifacts`。
- **第一个适配器**：取决于你用的 CI（见 05-Q10）。GitHub Actions（`workflow_dispatch` + runs API）最通用；Gitea / Woodpecker 适合完全自托管。
- **触发后自动跟踪**：轮询状态（桌面和 CLI 都没有常驻服务，不依赖 webhook），完成后从 CI 输出中解析镜像摘要，自动生成制品，关联 commit SHA。
- **可选的本地构建器**：没有 CI 的场景，在指定 Host 上用 `docker buildx` / BuildKit 构建（作为一种特殊的 CI 适配器）。
- 原 `runs record` 协议保留，用于不经过 Infradex 的外部构建。

## 4. 给 AI 的接口：MCP Server

`infradexctl mcp`（stdio）把能力暴露为 MCP 工具，Claude Code 等代理可直接接入，不需要任何服务端：

| 工具组 | 示例 | 风险 |
| --- | --- | --- |
| 查询 | `fleet.list`、`host.facts`、`k8s.get`、`logs.tail`、`metrics.get`、`ci.status`、`actions.list` | read，自动允许 |
| 计划 | `release.plan`、`host.plan`、`ci.plan_trigger` | read（只生成计划） |
| 执行 | `action.apply(planId)` | 按计划的 risk 和策略：自动 / 等待审批 |
| 审批 | `action.wait(planId)` | 阻塞，直到人在桌面或 CLI 审批 |

要点：

- **AI 拿不到凭据**。工具只返回结果，SSH 密钥、kubeconfig、CI token 都留在 keychain。
- **输出脱敏和截断**：沿用现有的结果预算（行数、字节数），日志做 secret 模式过滤。
- 桌面内置 AI 助手使用同一组工具（tool calling），不再只是“生成查询建议”。
- `AGENTS.fragment.md` 改为“通过 MCP 工具操作”，不再要求 AI 手工构造事件 JSON。

## 5. 策略与审批

示例（`project.yaml` 或单独的 `policy.yaml`，可 Git 共享）：

```yaml
policy:
  - match: { env: dev }
    ai: { allow: [read, low, medium], approve: [] }
  - match: { env: prod }
    ai: { allow: [read, low, medium, high], approve: [medium, high] }
    rollout: { canary: 1, batch: 3, pauseSeconds: 60 }
  - match: { action: host.exec }
    ai: { allow: [], approve: [critical] }     # 默认禁止 AI 用任意 shell
limits:
  aiActionsPerHour: 30
```

- 审批入口：桌面通知 + 审批面板（展示 diff 和风险），也可以用 CLI `infradexctl approve <planId>`。
- 每条记录区分 actor（人 / AI 名称 + 会话）和 approver。
- 记录仍是本地 JSON + Git 共享，不承诺防篡改；如果以后需要，可以加哈希链。

## 6. 运行形态

| 形态 | 用途 | 何时做 |
| --- | --- | --- |
| 桌面 + CLI + MCP（stdio） | 人和本机 AI 代理 | 现在 |
| `infradexd` 可选常驻进程（跑在一台运维机或机群中的一台） | 定时任务、CI webhook、长时间发版、远程 AI / 手机审批 | 确实需要时再做 |

先坚持本地优先：长时间的机群发版由 CLI 进程持有，中断后可从记录恢复或核实。
