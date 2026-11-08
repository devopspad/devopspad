---
name: devopspad
description: 查询和维护 OPC 产品矩阵的 DevOpsPad 档案库（产品、服务、环境、机器、集群、域名、数据库、第三方账号、配置文件、操作记录），并按 DevOpsPad 规则发版和改配置。Use when the user asks where or how a product/service is deployed, what runs on a host, wants to add or change a product, service, host, domain, provider or datastore, reviews renewals, costs or unused resources, changes live config or releases through DevOpsPad, records a manual ops change, or when the working directory contains devopspad.yaml or an AGENTS.md that mentions a DevOpsPad archive.
---

# DevOpsPad 档案库

档案库是一个 Git 仓库，记录产品矩阵的全部资产。期望状态是 YAML，凭据只存引用，操作记录只追加。你通过 `devopspadcli` 读写它，不要自己拼关系。

## 开始前

1. `devopspadcli version`：输出的 protocol 必须是 `devopspad.io/v1alpha1`。命令不存在或协议不同 → 停止，请用户安装或升级 DevOpsPad CLI。
2. `devopspadcli archive status -o json`：确认找到档案库（不在档案库目录里也能用，CLI 会找默认档案库）。
   - 找不到 → 问用户档案库路径，用 `--archive <dir>`；还没有档案库 → 读 workflows/import.md。
   - 准备修改前先 `devopspadcli archive status --fetch -o json`：`behind > 0` 先 `devopspadcli archive pull`；`changes` 非空且不是你改的 → 先问用户。

## 模型

| kind | 文件 | 说明 |
| --- | --- | --- |
| Product | products/<p>/product.yaml | 产品、阶段、域名、用到的账号、环境（环境 → 集群 / 命名空间 / 机器） |
| Service | products/<p>/services/<s>.yaml | 代码仓库、构建、发版方式和注意事项、各环境部署、配置文件、依赖、所需密钥 |
| Host / Cluster / Datastore / Registry | inventory/<kind>s/<name>.yaml | 跨产品共享的基础设施 |
| Domain / Provider | inventory/domains/<fqdn>.yaml、inventory/providers/<name>.yaml | 域名、第三方账号，含费用和到期日 |
| 笔记 | notes/*.md、products/<p>/notes/*.md | Markdown，可带 front matter `refs: [...]` |
| ActionRecord | records/actions/… | 操作记录，只读 |

引用写法：`host:hk-1`、`domain:foo.app`、`service:heartie/admin`、`datastore:main-mysql#heartie`；命令和记录里可加环境 `service:heartie/admin@pro`。

## 铁律

1. **不写密钥值**。凭据只能写成引用：`keychain:<account>`、`env:<NAME>`、`file:<path>`。用户给你密码、token、私钥时：不要写进任何文件或回复，让用户自己执行 `devopspadcli secrets set keychain:archive/<kind>/<name>/<what>`（值从输入读取），你只写引用。配置文件里用 `${secret:keychain:<account>}` 占位。
2. **读用 CLI**：`get <kind> [name] [-l <labels>]`、`describe <ref>`、`refs <ref>`、`report matrix [-l <labels>] [--stage <stage>]|renewals|cost|orphans`，都可加 `-o json`。产品有上百个，先用标签（约定 `line=<产品线>`、`revenue=paid|free|none`）或阶段筛选，不要一次读全部文件。
3. **改期望状态**：新资源用 `devopspadcli new <kind> <name> [--product p]` 生成骨架 → 编辑 YAML → `devopspadcli archive validate -o json` → 按错误提示修正直到 `ok: true` → 向用户说明改了什么 → `devopspadcli archive commit -m "<type>(<ref>): <summary>" --actor ai/<你的名字> --session <会话 ID>`。type：archive、config、record、note。不要自动 push，除非用户要求（`devopspadcli archive push`）。
4. **不要手改** `records/`、`INDEX.md`、`AGENTS.md`，也不要提交 `.devopspad/`（本机状态）；不要为了通过校验删除别人的数据或加 `devopspad:allow-secret`，除非用户确认是误报。
5. **不编造事实**。IP、到期日、费用、账号、规格不知道就不写（字段都是可选的），并告诉用户还缺什么。域名到期日、DNS、证书用 `scan` 读真实值（只读，会访问 Cloudflare / RDAP / 站点，先告诉用户），再按 `diff` 修正档案。
6. **线上写操作只用 DevOpsPad 命令**（`devopspadcli config get/set`、`workload restart`、桌面发版），不要直接 `ssh` / `kubectl` 修改。发版前先读 `describe` 给出的 Release notes。
7. 在 DevOpsPad 之外做过的线上操作，用 `devopspadcli note add --refs <ref> -m "<做了什么>" --actor ai/<你的名字>` 补记，再 commit。
8. 删除或下线资源前先 `refs <ref>` 看影响，并得到用户确认。

## 任务 → 先读的文件

| 任务 | 文件 |
| --- | --- |
| X 部署在哪 / 怎么发版 / 依赖什么 / 机器上跑了什么 | workflows/locate.md |
| 新产品、新服务 | workflows/onboard-product.md |
| 登记机器、域名、账号、数据库、镜像仓库 | workflows/register-resource.md |
| 发版、改配置、重启 | workflows/release-and-config.md |
| 排障 | workflows/incident.md |
| 到期、费用、闲置资源检查；域名 / 证书 / DNS 核对（scan、diff） | workflows/review.md |
| 产品或机器下线 | workflows/sunset.md |
| 建档案库、从 Cloudflare 或 DevOpsPad 桌面导入、换电脑恢复 | workflows/import.md |
| 字段定义、校验错误码 | reference/protocol.md，或 `devopspadcli schema <kind>` |
