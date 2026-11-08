# 登记机器、域名、账号、数据库、镜像仓库

通用步骤：`devopspadcli new <kind> <name>` → 编辑生成的文件（取消注释需要的字段）→ `archive validate -o json` → 用户确认 → `archive commit -m "archive(<kind>:<name>): register …"`。

## 机器 Host
- 名字用用途 + 地区，如 `hk-1`。`ssh` 段写连接参数；`keyRef: file:~/.ssh/<key>`；密码登录写 `passwordRef: keychain:archive/hosts/<name>/ssh`，并让用户执行 `devopspadcli secrets set keychain:archive/hosts/<name>/ssh`。
- `roles` 写跑了什么（k3s-server、nginx、mysql、docker）；`cost`、`renewal` 有就写。

## 域名 Domain
- 文件名就是域名：`devopspadcli new domain foo.app`。
- `registrar`、`dns` 指向 provider；关键解析写 `records`，指向自己的机器时用 `target: host:<name>` 而不是 IP。
- `renewal.expiresAt` 不知道就问用户，不要编。

## 账号 Provider
- `categories` 选分类；`account` 写登录名（不是密码）；`mfa` 写两步验证在哪。
- API token：`credentials: [{ name: api-token, ref: keychain:archive/providers/<name>/api-token, scope: … }]`，值由用户 `secrets set`。

## 数据库 Datastore
- `runtime` 三选一：`host:`、`cluster:`+`namespace`、`provider:`+`managed`。
- 库名 / 桶名写 `parts`，服务里用 `datastore:<name>#<part>` 依赖。

## 镜像仓库 Registry
- `url` 写到命名空间为止；服务 `build.image: registry:<name>#<repository>`。

登记后用 `devopspadcli refs <ref>` 确认引用关系正确。
