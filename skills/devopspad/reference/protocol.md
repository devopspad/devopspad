# 档案库协议 devopspad.io/v1alpha1（速查）

完整字段定义：`devopspadcli schema <kind>`（JSON Schema，由校验器同一份代码生成）。示例档案库：DevOpsPad 仓库的 `examples/archive/`。

## 目录

```text
devopspad.yaml                         kind: Archive
inventory/providers|hosts|clusters|datastores|registries/<name>.yaml
inventory/domains/<fqdn>.yaml
products/<p>/product.yaml              kind: Product
products/<p>/services/<s>.yaml         kind: Service（metadata.product: <p>）
products/<p>/configs/<s>/<env>/<file>  配置原文，密钥用 ${secret:<ref>} 占位
products/<p>/notes/*.md, notes/*.md    笔记，可选 front matter: title, refs, date, tags
records/actions/<yyyy>/<mm>/<ULID>.json  操作记录（只追加）
INDEX.md, AGENTS.md                    生成文件
```

每个资源：`apiVersion: devopspad.io/v1alpha1`、`kind`、`metadata`（name 必填；title、description、labels、annotations 可选）、`spec`。未知字段报错，自由文本放 `metadata.description` / `metadata.annotations` / 笔记。名字：小写字母、数字、短横线（域名用小写 FQDN），必须等于文件名。

## 主要字段

| kind | spec 要点（* 必填） |
| --- | --- |
| Provider | categories, console, account, mfa, plan, cost, renewal, credentials[{name*, ref*, scope}] |
| Host | provider, state(active/standby/retired), addresses{public,private}, ssh{host*, port, user*, auth*(key/agent/password), keyRef, passphraseRef, passwordRef, hostKey, knownHosts}, os, size{cpu,memoryGiB,diskGiB}, roles, cost, renewal |
| Cluster | distribution*(k3s/k8s), hosts, tunnel, kubeconfig*{source*(remote: host+path / local: pathRef file:), context} |
| Datastore | engine*, runtime{host / cluster+namespace / provider+managed}, tunnel, endpoint, parts, credentials, backup{method,schedule,target,retentionDays}, cost |
| Registry | url*, provider, credentials |
| Domain | registrar, dns, cost, renewal, records[{name*, type*, value 或 target(host:)}], certificates[{names*, issuer, managedBy, method, expiresAt}] |
| Product | stage*(idea/building/live/paused/sunset), launchedAt, links, domains, providers[{ref*, purpose}], environments[{name*, cluster, namespace, hosts}] |
| Service | type*(web/api/worker/cron/static/other), repo{url*,path,defaultBranch}, build{method*(ci/local/none), ci{provider*,workflow}, image: registry:<r>#<repo>}, release{method*(devopspad/devopspad-spec/ci/manual), spec, notes}, deployments[{environment*, runtime*(k8s/host/static), cluster, namespace, selector, workload{kind*,name*,container}, hosts, unit, provider}], endpoints[{url*,health}], configs[{name*, type*(nginx/configmap/env-file/systemd/file), environment*, source*, target{host,path / configMap,key}}], dependsOn, secrets[{name*, ref*}] |

公共类型：`cost: { amount: "12.50", currency: USD, period: month|year|once }`（金额是字符串），`renewal: { expiresAt: YYYY-MM-DD, autoRenew: bool }`。

## 引用与密钥

- 引用：`<kind>:<name>`；服务 `service:<p>/<s>`；数据存储的库 `datastore:<d>#<part>`（part 须在 parts 中）；镜像 `registry:<r>#<repo>`；`@<env>` 只用于命令和记录。
- 密钥引用：`keychain:<account>`（OS 钥匙串，服务名 devopspad；新密钥建议 `archive/<kind>/<name>/<what>`），`env:<NAME>`，`file:<绝对路径或 ~/…>`。写入钥匙串：`devopspadcli secrets set keychain:<account>`。

## 校验错误码（`archive validate -o json` 的 code）

| code | 含义 / 修正 |
| --- | --- |
| yaml.syntax, yaml.empty, yaml.multiple_documents | YAML 写错；一个文件一个文档 |
| schema.unknown_field, schema.type | 字段名或类型不对；看 `schema <kind>` |
| api.version, kind.mismatch | apiVersion / kind 与目录不符 |
| name.format, name.file_mismatch, name.product_mismatch, label.format | 名字、文件名、metadata.product、label 格式 |
| field.required, field.enum, field.pattern, field.const, field.range, field.duplicate, field.not_allowed, format.date, format.url | 字段值问题，消息里有可选值 |
| ref.syntax, ref.kind, ref.unresolved, ref.part, ref.environment | 引用写错 / 指向不存在的资源（消息列出现有的） / 环境不在产品中 |
| secret.ref, secret.literal, secret.pattern | 密钥引用格式错 / 写了明文密钥：改成引用或 `${secret:…}` 占位 |
| file.path, file.missing, file.unreadable | configs.source / release.notes 必须是产品目录下存在的文件 |
| layout.unknown, layout.extension, layout.missing_product, layout.symlink | 目录结构问题 |
| kubeconfig.remote, kubeconfig.local, datastore.runtime, domain.record, deployment.k8s, deployment.selector, deployment.host, config.target, service.cycle | 跨字段规则，按消息补齐 |
| note.front_matter | 笔记 front matter 只允许 title, refs, date, tags |
| record.invalid, record.path, record.immutable | 记录格式错误 / 已结束的记录被修改或删除（用 git checkout 恢复） |

警告（不阻止提交）：generated.stale（commit 会自动重新生成）、renewal.expired、record.target_unresolved。
