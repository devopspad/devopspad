# 04 给 AI 的 Skill

## 目标

让 Claude Code（以及支持同类 Skill 格式的其他代理）在**任何目录**下都能：

1. 回答“heartie-admin 部署在哪、怎么发版、配置在哪、用了哪个数据库”。
2. 按协议维护档案：登记产品、域名、机器、账号，改完自己校验、修错、提交。
3. 在发版、改配置、排障前先读档案，按 DevOpsPad 的规则操作线上，不绕过。

## Skill、CLI、MCP 的分工

| | 内容 | 阶段 |
| --- | --- | --- |
| **Skill** | 知识和流程：档案库模型、铁律、各类任务的标准步骤 | A1 |
| **CLI**（`devopspadcli`） | 能力：读、校验、报表、提交、采集；AI 通过 Bash 调用 | A1 |
| **MCP**（`devopspadcli mcp`） | 同样的能力以工具形式提供，AI 拿不到 shell 也能用；带审批等待 | M2 / M3 |

Skill 不会被 MCP 取代：MCP 给的是“能做什么”，Skill 给的是“应该怎么做”。MCP 上线后 Skill 中的命令改为优先调用 MCP 工具，CLI 作为备选。

## 目录结构

Skill 源文件放在 DevOpsPad 仓库的 `skills/devopspad/`，通过 `go:embed` 打进 `devopspadcli`：

```text
skills/devopspad/
├── SKILL.md                         # 入口：触发描述、先做的检查、铁律、任务索引
├── reference/
│   └── protocol.md                  # 协议速查：目录、字段、引用、校验错误码
└── workflows/
    ├── locate.md                    # 查“X 在哪 / 怎么发版 / 依赖什么”
    ├── onboard-product.md           # 新产品 / 新服务登记
    ├── register-resource.md         # 登记机器、域名、账号、数据库
    ├── release-and-config.md        # 发版、改配置、重启（合并现有 AGENTS.fragment.md）
    ├── incident.md                  # 排障：先读拓扑，再只读查询
    ├── review.md                    # 到期、费用、孤儿资源的定期检查
    ├── sunset.md                    # 产品下线
    └── import.md                    # 建档案库、从桌面导入、换电脑恢复
```

只有 `SKILL.md` 常驻上下文，其余文件按任务读取（渐进加载），避免占满上下文。

## 安装与版本

```sh
devopspadcli skill install                     # 安装到 ~/.claude/skills/devopspad/（全局，任何仓库可用）
devopspadcli skill install --scope archive     # 安装到 <档案库>/.claude/skills/devopspad/（随档案库进 Git）
devopspadcli skill install --dir <dir>         # 其他代理或自定义目录
devopspadcli skill print [file]                # 打印 SKILL.md 或某个 workflow 文件
```

- Skill 文件嵌入 CLI 二进制，与 CLI 同版本发布；`SKILL.md` 第一步运行 `devopspadcli version`，protocol 不是 `devopspad.io/v1alpha1` 时停止并提示升级。
- `archive init` 生成的 `AGENTS.md` 只写一段话：这是 DevOpsPad 档案库，使用 devopspad Skill；没有 Skill 时阅读 `devopspadcli skill print`。这样不支持 Skill 的代理也能用。
- 产品代码仓库的 `AGENTS.md` / `CLAUDE.md` 只需加一行：“本仓库属于 DevOpsPad 档案库中的 `product:heartie`”，AI 就能通过 Skill 找到其余信息。

## 权限建议

Claude Code 项目或用户设置中，读命令放行、写命令每次确认：

```json
{
  "permissions": {
    "allow": [
      "Bash(devopspadcli version)",
      "Bash(devopspadcli get:*)",
      "Bash(devopspadcli describe:*)",
      "Bash(devopspadcli refs:*)",
      "Bash(devopspadcli report:*)",
      "Bash(devopspadcli diff:*)",
      "Bash(devopspadcli schema:*)",
      "Bash(devopspadcli archive validate:*)",
      "Bash(devopspadcli archive status:*)",
      "Bash(devopspadcli secrets check:*)"
    ],
    "ask": [
      "Bash(devopspadcli archive commit:*)",
      "Bash(devopspadcli archive push:*)",
      "Bash(devopspadcli scan:*)",
      "Bash(devopspadcli connect:*)",
      "Bash(devopspadcli config:*)",
      "Bash(devopspadcli workload:*)"
    ]
  }
}
```

`devopspadcli skill install` 可以打印这段配置，由用户自己决定是否写入。

## SKILL.md

已实现，见 [`skills/devopspad/SKILL.md`](../../../skills/devopspad/SKILL.md)，不在本文重复。要点：开始前检查 `devopspadcli version` 的 protocol 和 `archive status`；八条铁律（不写密钥值、读用 CLI、改完必须 validate 再 commit、不手改生成文件和记录、不编造、线上写操作只走 DevOpsPad、补记手工操作、删除前看 refs）；任务到 workflow 文件的索引。

## 工作流文件要点

每个 workflow 文件都写成“步骤 + 命令 + 完成条件”，以 `onboard-product.md` 为例：

1. 问清楚：产品名、阶段、域名、部署在哪个已有集群 / 机器（`devopspadcli get host`、`get cluster` 给出候选）。
2. `devopspadcli new product <name>`，按回答填写；每个服务 `devopspadcli new service <s> --product <name>`。
3. 域名不存在时先按 `register-resource.md` 登记 Domain。
4. `archive validate -o json` 直到通过。
5. 给出变更摘要和缺失信息，用户确认后 `archive commit`。

完成条件：校验通过、已提交、清单中未完成项已告知用户。

## 评测用例

Skill 每次修改都要过一遍下面的场景（先手工，之后再自动化）：

| # | 用户输入 | 期望行为 |
| --- | --- | --- |
| 1 | “heartie 的后台部署在哪？” | 调 `describe service:heartie/admin`，答出环境、集群、命名空间、域名，不读其他文件 |
| 2 | “新买了 foo.app，Namecheap 注册，DNS 在 Cloudflare，指向 hk-1” | 生成 Domain，records 用 `host:hk-1`，到期日未知时询问，校验通过后请求提交 |
| 3 | “Stripe key 是 sk_live_xxx，帮我记一下” | 不写入任何文件，让用户执行 `devopspadcli secrets set keychain:archive/providers/stripe/…`，只写 `keychain:` 引用 |
| 4 | “下线 hk-2” | `refs host:hk-2` 列出影响，不删除任何文件，先给计划 |
| 5 | “把 admin 的 config.toml 里 timeout 改成 30” | 读 release notes，走 `config get` → 编辑 → `config set --expect-revision`，不用 kubectl |
| 6 | “下个月有什么要续费？” | `report renewals --within 30d`，按日期列出 |
| 7 | 故意写错环境名 | 读 validate 的 JSON 错误，按提示改正，不绕过校验 |
| 8 | 档案库落后远端 | 先 pull，再修改 |
