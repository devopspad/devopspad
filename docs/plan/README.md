# DevOpsPad 规划讨论

日期：2026-11-06

本目录用于讨论项目下一阶段的方向和优先级。结论确认后，再回写到根目录 [PLAN.md](../../PLAN.md) 和相关规范。

| 文件 | 内容 |
| --- | --- |
| [01-current-state.md](01-current-state.md) | 现状盘点：已有功能、代码结构、测试与验证情况 |
| [02-issues-and-risks.md](02-issues-and-risks.md) | 问题与风险：文档漂移、写路径边界、规范与实现不一致、技术债 |
| [03-positioning.md](03-positioning.md) | 产品定位（已确定方向）、差异化、竞品 |
| [04-roadmap-draft.md](04-roadmap-draft.md) | 路线草案 M0–M5 |
| [05-open-questions.md](05-open-questions.md) | 决策清单（已决定 / 待决定） |
| [06-ai-devops-control-plane.md](06-ai-devops-control-plane.md) | AI 驱动的 DevOps 控制面架构草案：Action 引擎、群控、CI、MCP、策略审批 |
| [opc-archive/](opc-archive/README.md) | OPC 产品矩阵档案库（2026-11-07）：Git 管理的资产与配置档案、协议 v1alpha1、AI Skill、分期 A0–A4 |

## 方向

> **AI 驱动的本地优先 DevOps 控制面**：控制 CI、群控机器和 K3s 集群，让 AI 在人设定的策略和审批范围内完成构建、发版、运维和排障，所有动作都有计划和记录。

## 一句话结论

现有代码已经具备集群读取、单 Deployment 发版、Nginx / ConfigMap 编辑和 AI 执行记录的基础，但写路径分散、两套发版模型并存，AI 只能“自报”记录。下一步：

1. 入库现有代码（M0）。
2. 建立统一的 Action 引擎和领域模型（M1）。
3. 先让 AI 只读接入（M2），再上线策略和审批（M3）。
4. 最后扩展到群控机器（M4）和 CI（M5）。
