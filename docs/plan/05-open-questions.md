# 05 待决策清单

每个问题都附了建议，可以直接回复“同意”或改选。

## 已决定

- **Q1 产品主线**：AI 驱动的 DevOps 控制面，覆盖 CI、群控机器和集群（2026-11-06）。见 [03](03-positioning.md)。
- **Q8 AI 角色**：AI 通过 DevOpsPad 的工具执行，DevOpsPad 负责计划、策略和记录。见 [06](06-ai-devops-control-plane.md)。

## 新增（与 CI / 群控 / AI 相关）

### Q10 用什么 CI？

GitHub Actions / GitLab CI / Gitea Actions / Woodpecker / Jenkins / 目前没有 CI？

这决定 M5 第一个适配器。如果目前没有 CI，建议先做“指定 Host 上本地构建”，以后再接 CI。

### Q11 先做群控还是先做 CI？

- 群控机器（M4）先做（建议）：已有 SSH、Nginx、OPC 规范的基础，复用度高。
- CI 控制（M5）先做：如果当前最大的痛点是构建 / 发版流程。

### Q12 有多少台机器，是什么形态？

- 机器数量，是否都跑 K3s，是否都有 Nginx。
- 是否有不跑 K3s、只用 docker / systemd 的机器。

这决定 Host 动作优先支持哪些（systemd、docker compose、k3s）。

### Q13 是否允许 AI 执行任意 shell？

- 默认禁止，只能在人工逐次审批后执行（建议）。
- 在 dev 环境允许，prod 环境禁止。
- 完全不提供。

### Q14 AI 接入方式

- MCP Server（`devopspadctl mcp`，stdio）作为主接口，CLI JSON 作为备用（建议）。
- 主要用哪个 AI 代理：Claude Code、Codex、Cursor，还是桌面内置助手？

### Q15 是否需要常驻服务 `devopspadd`？

- 暂不需要，坚持本地优先（建议）。
- 需要：例如要接 CI webhook、做定时巡检、用手机审批、让远程 AI 使用。

## 仍待决定

### Q2 目标用户和规模

自用 / 2–3 人团队，还是计划对外发布或收费？建议先按自用 + 小团队设计，商业化暂缓。

### Q3 统一领域模型

- 方案 1（建议，已体现在 06）：Host 成为一等连接类型；机群 = 标签选择器；`project.yaml` 的环境引用 Host / 集群；`fleet.json` 迁移为标签后废弃。
- 方案 2：保留 `fleet.json`，`project.yaml` 引用 fleet 名称。

子问题：发版规范 `service.release.yaml` 放在服务仓库（规范现状），还是放在项目目录？

### Q4 Nginx 写入方式

- 方案 1（建议）：按规范使用 SFTP + 固定的 `nginx -t` / `nginx -s reload`，远端零依赖。
- 方案 2：保留 Python 脚本 + systemctl，修改规范。

M4 引入结构化主机动作后，选方案 1 更一致。

### Q5 写路径约束

所有写入都必须经过 Action 引擎的计划、审批和记录。建议接受；AI 写权限依赖这一点。

### Q6 界面语言

建议界面统一中文。

### Q7 中间件工作台

建议冻结在只读，作为 AI 排障时的数据查询工具。

### Q9 未提交代码的入库方式

建议开分支 `feat/infra-ops`，按逻辑拆成多个提交后合并。
