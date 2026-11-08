# 06 决策清单

编号 `Q-A*`，与 [../05](../05-open-questions.md) 的 Q1–Q15 区分。

## 已决定（2026-11-07）

| # | 问题 | 结论 | 来源 |
| --- | --- | --- | --- |
| Q-A2 | 密钥怎么存 | **不接密码管理器，不加密入库**。只支持 `keychain:` / `env:` / `file:` 引用；新增 `devopspadcli secrets set keychain:<account>` 写入 OS 钥匙串 | 用户：没用密码管理器，先不增加复杂度 |
| — | CLI 名称 | `devopspadctl` 改名为 **`devopspadcli`**（目录 `cmd/devopspadcli`、发布产物、文档） | 用户 |
| Q-A10 | 优先级 | 先做 A0 + A1 | 用户：按 plan 执行 |
| Q-A1 | 档案库放在哪 | 独立的私有 Git 仓库；`archive init` 新建并设为默认 | 按建议 |
| Q-A4 | IP、账号邮箱、费用能否入库 | 允许，前提是私有仓库 | 按建议 |
| Q-A6 | 面向哪些 AI 代理 | Claude Code 为主（`skill install`），同时生成通用 `AGENTS.md`；其他代理用 `skill install --dir` | 按建议 |
| Q-A7 | 协议标识 | `apiVersion: devopspad.io/v1alpha1` | 按建议 |
| Q-A8 | 多币种 | 按币种分别小计，不换算；费用不做分摊，只区分“独占 / 共享” | 按建议，并简化 |
| Q-A9 | `service.release.yaml` 放在哪 | 继续放在代码仓库，Service 的 `release.spec` 只引用路径 | 按建议 |
| Q-A3 | 观测快照是否进 Git | **不进**。`scan` 结果只存本机 `.devopspad/observed/`；`archive commit` 不提交它 | 用户：不保存镜像 |
| Q-A12 | 上百个产品怎么分组 | 约定产品标签 `line=<产品线>`、`revenue=paid\|free\|none`，只是约定、不强制校验；Skill 批量登记时沿用已有标签值 | 默认约定，可随时改 |
| Q-A11 | 域名注册商和 DNS | **Cloudflare**。已实现 `archive import --cloudflare`、`scan`、`diff`；token 只需 Zone Read + DNS Read，Registrar 数据读不到时用 RDAP | 用户 |
| Q-A5 | 规模 | **上百个产品，2 台机器**。按 300 个产品设计并有性能测试；A2 采集先做域名 / 证书；机群（M4）降为最低优先级 | 用户 |

“按建议”的几项如需改，直接说，影响都不大。

## 简化掉的内容（以后需要再加）

- 密码管理器引用（`op://`、`bw://`）和 sops 加密文件。
- 费用分摊比例（`providers[].share`）。
- `Archive.spec` 中的 currency、timezone、secretStores、git.autoCommitRecords。
- `product checklist`、`archive migrate`、`skill doctor` 命令；上线 / 下线清单由 Skill 的流程覆盖。
- 高熵字符串检测（误报多）；误报豁免改为行内注释 `devopspad:allow-secret`。
- 提交到仓库的 JSON Schema 文件；改为 `devopspadcli schema <kind>` 运行时生成。

## 仍待决定

暂无。
