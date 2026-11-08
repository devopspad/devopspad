# OPC 档案库

档案库是一个私有 Git 仓库，记录产品矩阵的全部资产：产品、服务、环境、机器、集群、域名、数据库、镜像仓库、第三方账号、配置文件、笔记和操作记录。`devopspadcli` 负责读写和校验；AI 通过内置的 devopspad Skill 按同一规则使用它。

- 只存密钥**引用**（`keychain:` / `env:` / `file:`），密钥值在本机 OS 钥匙串里。
- 除 `scan` 和 `archive import --cloudflare`（只读访问 Cloudflare、RDAP 和站点证书）外，命令只读写档案库文件，不访问线上机器或集群。
- 协议与设计：[docs/plan/opc-archive](plan/opc-archive/README.md)；完整示例：[examples/archive](../examples/archive/)。

## 安装 CLI

```sh
go build -o /usr/local/bin/devopspadcli ./cmd/devopspadcli
devopspadcli version          # protocol devopspad.io/v1alpha1
```

发布版的 `devopspadcli-<版本>-darwin-arm64` 包里也有同一个二进制。

## 新建档案库

```sh
devopspadcli archive init ~/opc-archive --name my-opc
git -C ~/opc-archive remote add origin <私有仓库地址>
```

`init` 会：建目录和 `devopspad.yaml`、`git init`、安装 pre-commit 校验钩子，并把它设为默认档案库（之后在任何目录运行 `devopspadcli` 都会用它；`archive use <dir>` 可切换，`--archive <dir>` 或 `DEVOPSPAD_ARCHIVE` 临时指定）。

已有 DevOpsPad 桌面配置时，先预览再导入：

```sh
devopspadcli archive import --workspace --fleet --dry-run
devopspadcli archive import --workspace --fleet --project "$HOME/Library/Application Support/DevOpsPad/applications" --product heartie
```

导入只读源文件，不复制密钥（引用指向桌面已有的钥匙串条目和私钥路径），可重复执行；输出末尾列出需要人工补齐的内容。

## 目录

```text
devopspad.yaml                         根文件
inventory/providers|hosts|clusters|datastores|registries|domains/
products/<产品>/product.yaml
products/<产品>/services/<服务>.yaml
products/<产品>/configs/<服务>/<环境>/  配置文件原文，密钥写成 ${secret:keychain:…}
products/<产品>/notes/*.md, notes/*.md  笔记
records/actions/                       操作记录（只追加）
INDEX.md, AGENTS.md                    自动生成
.devopspad/                            本机状态（采集快照、缓存），不进 Git
```

## 日常使用

```sh
devopspadcli new domain foo.app                 # 生成带注释的骨架
devopspadcli new service web --product foo
devopspadcli archive validate                   # 改完必跑；-o json 给 AI 用
devopspadcli archive commit -m "archive(domain:foo.app): register domain"
devopspadcli archive push

devopspadcli report matrix                      # 产品矩阵
devopspadcli report matrix -l line=tools --stage live   # 产品多时按标签 / 阶段筛选
devopspadcli report renewals                    # 30 天内到期（--within 60 / --all）
devopspadcli report cost                        # 月度费用，按币种汇总
devopspadcli report orphans                     # 没有产品在用的资源
devopspadcli describe service:heartie/admin     # 部署位置、依赖、发版注意事项、最近记录
devopspadcli refs host:hk-1                     # 谁在用这台机器
devopspadcli get hosts -l region=hk
devopspadcli schema service                     # 字段定义（JSON Schema）
```

提交信息格式：`<type>(<scope>): <summary>`，type 为 archive、config、record、scan、note、migrate、skill。`archive commit` 会先校验并重新生成 `INDEX.md` / `AGENTS.md`，只提交档案库路径，并加上 `DevOpsPad-Actor`（默认 `human/<git user.name>`，AI 用 `--actor ai/<名字>`）。

## 密钥

```sh
devopspadcli secrets set keychain:archive/providers/cloudflare/dns-token   # 隐藏输入，或从 stdin 读取
devopspadcli secrets check                                                 # 本机缺哪些（不显示值）
```

YAML 中只写引用，例如 `credentials: [{ name: dns-token, ref: keychain:archive/providers/cloudflare/dns-token }]`、`keyRef: file:~/.ssh/id_ed25519`。校验会拒绝明文：`password:` 之类字段、私钥、常见 token 格式、带密码的 URL / DSN，以及配置文件里 `DB_PASSWORD=…` 这样的赋值。确属误报时，在该行加注释 `devopspad:allow-secret`。

## 操作记录

DevOpsPad 之外做的线上操作，补一条记录：

```sh
devopspadcli note add --refs service:heartie/admin@pro -m "手工重启，原因 OOM"
devopspadcli archive commit -m "record(service:heartie/admin): manual restart"
```

已提交的记录不能修改或删除，校验和钩子都会拒绝。

## 域名（Cloudflare）

域名的注册和 DNS 都在 Cloudflare 时：

1. 在 Cloudflare 控制台创建 API token，权限 **Zone Read + DNS Read**，范围所有 zone。
2. 存入本机钥匙串（不进档案库）：`devopspadcli secrets set keychain:archive/providers/cloudflare/api-token`
3. 一次导入全部域名：

```sh
devopspadcli archive import --cloudflare --dry-run   # 预览
devopspadcli archive import --cloudflare             # 生成 provider:cloudflare 和每个 zone 一个 Domain
```

到期日优先取 Cloudflare Registrar 数据，取不到时用公开 RDAP；自动续费状态读不到时记为 false 并列入待确认。之后把域名关联到产品（Product 的 `spec.domains`），`report orphans` 会列出还没关联的。

定期核对：

```sh
devopspadcli scan                       # 读取 Cloudflare 域名 / DNS / 到期日，检查每个 HTTPS endpoint 的证书
devopspadcli diff                       # 档案与实际的差异：过期、到期日不一致、DNS 指向变了、证书快到期、未登记的 zone
devopspadcli report renewals --observed # 到期清单，合并扫描到的证书到期日
```

`scan` 结果只存本机 `.devopspad/observed/`，不进 Git；按 `diff` 更新档案后再提交。没有 token 时 `scan` 仍会用 RDAP 查到期日并检查证书。

### Workers 路由和 DNS 变更

`scan` 会读取每个 zone 的 Workers 路由（需要 Workers Routes Read），在桌面域名详情里显示。修改通过计划 → 确认 → 记录：

```sh
devopspadcli edge routes list --zone example.com
devopspadcli edge routes add --zone example.com --pattern 'example.com/admx*' --script none   # 打印前后路由列表
devopspadcli edge routes add --zone example.com --pattern 'example.com/admx*' --script none --yes
devopspadcli edge routes remove --zone example.com --pattern 'example.com/admx*' --yes
devopspadcli edge dns set --zone example.com --name admx --type CNAME --content example.com --proxied --yes
devopspadcli edge dns set --zone example.com --name @ --type A --target host:hk-1 --proxied --yes
```

`--script none` 表示该路径不经过 Worker、直接到源站。执行前会重新读取路由 / 记录，和计划时不一致就拒绝。每次变更写一条档案库操作记录（`edge.route.add|remove`、`edge.dns.set`），包含变更前后的路由列表或记录值，之后用 `archive commit` 提交。修改需要 token 有 Workers Routes Edit / DNS Edit；只读扫描用的 token 可以保持只读，修改时用 `--token-ref` 指定另一个。

## 集群和机器

`devopspadcli scan --kind cluster` 读取每个集群的节点和全部命名空间的工作负载（`source: remote` 的 kubeconfig 经 SSH 临时读到系统临时目录，用完删除）；`--kind host` 在每台机器上执行一段固定的只读命令，读取系统、规格、运行中的 systemd 服务和 `/etc/nginx/conf.d/*.conf` 的哈希。`diff` 会列出：

- 档案里登记了、集群里找不到的部署，以及没有全部就绪的工作负载；
- 集群里在跑、档案里没登记的服务（`kube-system` 等系统命名空间除外）；
- 机器上 Nginx 配置与档案 `configs/` 副本不一致、或还没存档的配置文件；角色（nginx、k3s、docker、mysql）对应的服务没在运行。

## 配置文件归档与下发

服务的 `spec.configs` 登记了 Nginx 文件或 ConfigMap 键时：

```sh
devopspadcli config diff heartie/admin@pro --name nginx    # 档案副本 vs 线上；不一致时退出码 1
devopspadcli config pull heartie/admin@pro --name nginx    # 线上内容写回档案（之后 archive commit）
devopspadcli config push heartie/admin@pro --name app      # 打印要写入的差异
devopspadcli config push heartie/admin@pro --name app --yes
```

- 档案里的 `${secret:keychain:…}` 只在本机渲染：比较时，线上与渲染结果相同的行会换回占位符，其它像密钥的值显示为 `***`，差异里永远不出现密钥值。`pull` 遇到占位符以外的密钥会拒绝，并提示先在档案副本里改成占位符。
- `push` 支持 Nginx（经远端脚本：加锁、revision 检查、`nginx -t`、reload、失败恢复，需要 Python 3）和 ConfigMap 键（dry-run + resourceVersion 检查，只改这一个键，不自动重启）。其它类型只支持 diff / pull。每次下发写一条 `config.push` 档案记录，只含行数和哈希。

## 操作记录统一进档案库

配置了档案库后（`archive init` / `archive use` / 桌面 Archive 页面打开过），桌面和 CLI 的每次发布、回退、扩缩容、重启、改配置、Nginx、Secret、工作负载修改、Job 结束时，都会在档案库 `records/actions/` 写一条对应的记录（按集群连接名、命名空间和应用选择器匹配到服务；匹配不到的只留在原项目目录）。CLI 用环境变量 `DEVOPSPAD_ACTOR=ai/claude-code` 标明执行者，默认 `human/<git user.name>`。旧记录可一次导入：`devopspadcli archive import --records <桌面项目目录>`（可重复执行）。

## 用档案生成 Infra 映射

`devopspadcli archive project` 预览由档案生成的 `project.yaml`（环境 `<产品>-<环境>`、应用 `<产品>-<服务>`、选择器、nginxPath），`--write` 写入桌面项目目录。之后改映射请改档案再重新生成，档案就是唯一来源；配合 `connect sync`（连接名就是集群名）即可直接使用 Infra 页面和 `release`/`config`/`nginx` 命令。

## 生成本机连接

档案里的集群和数据库可以直接生成 DevOpsPad 桌面连接（新电脑恢复时最有用）：在桌面 **Archive** 页面点 **Sync connections**，先看计划再应用；或者**关闭桌面应用后**运行：

```sh
devopspadcli connect sync --dry-run
devopspadcli connect sync
```

按 `devopspad.io/connection-id` 注解（导入时写入）或名字匹配已有连接，只新建或更新、不删除。密钥从档案引用的钥匙串条目读取后存进连接自己的钥匙串条目；读不到时照样建连接并提示。远端 kubeconfig 保存到 DevOpsPad 配置目录的 `kube/`（权限 0600）。

## 桌面 Archive 页面

左侧导航栏的 **Archive**（`#archive`）：总览（产品数、即将到期、扫描差异）、按类型浏览和搜索（支持 `line=tools` 这样的标签）、资源详情（YAML 编辑后立即校验、引用 / 被引用、部署位置、最近记录、上次扫描结果）、Git 面板（提交、拉取、推送），以及 Scan 和 Sync connections 按钮。还没有档案库时可以在这里新建或打开。

## 换电脑

```sh
git clone <私有仓库地址> ~/opc-archive
devopspadcli archive init ~/opc-archive     # 已有档案库时只安装钩子并设为默认（Git 不会克隆钩子）
devopspadcli secrets check                  # 按提示 secrets set，私钥文件从备份恢复
```

## 给 AI 使用

```sh
devopspadcli skill install                  # 安装到 ~/.claude/skills/devopspad/，任何目录可用
devopspadcli skill install --scope archive  # 或装进档案库 .claude/skills/，随仓库同步
```

之后在 Claude Code 里直接问“heartie 的后台部署在哪”“把新买的 foo.app 登记一下”“下个月有什么要续费”。Skill 规定 AI 只通过 `devopspadcli` 读写、改完必须校验、不写密钥值、不手改记录。产品很多时可以让 AI 批量登记（Skill 的 onboard-product 流程：一次登记几十个产品，校验一次，提交一次），建议用 `metadata.labels` 给产品分组，约定 `line=<产品线>` 和 `revenue=paid|free|none`，例如 `report matrix -l line=tools,revenue=paid`。产品代码仓库的 `CLAUDE.md` / `AGENTS.md` 加一行“本仓库属于 DevOpsPad 档案库的 `product:<名字>`”即可关联。

建议在 Claude Code 设置里放行只读命令（`devopspadcli get/describe/refs/report/diff/schema/version`、`archive validate/status`、`secrets check`），`scan`（访问网络）和 `archive commit/push` 保持确认。
