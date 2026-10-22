# OPC 机群发版规范

日期：2026-10-21

桌面负责查看和群控多台 OPC 机器。`infradexctl` 负责可重复发版。两者调用同一套 `internal/release`。本文件是该规范的仓库副本。查询工作台保持只读。发版和 Nginx 写入只来自 `internal/release`，当前只实现了规范校验和本机 fleet 文件。

## 机群

每台机器是一个独立管理端点，可同时承担 `cluster` 和 `edge` 角色。群控是对多台端点做同一动作。

fleet 文件位于 `os.UserConfigDir()/Infradex/fleet.json`，权限 0600。机器引用 Infradex 已有连接。SSH 秘密留在 `infradex` 钥匙串。一个 fleet 最多 32 台，名字在 fleet 内唯一。角色只有 `cluster` 和 `edge`，至少有一个。连接被删除时，fleet 校验失败并指出机器名。

远端 kubeconfig 通过 SFTP 读取绝对路径，最多 1 MiB，拒绝 exec 插件。内容不进入前端和 `workspace.json`。API 地址使用 kubeconfig 里的 server，经 SSH `direct-tcpip` 从 SSH 服务器侧拨号。

## 发版文件

规范文件放在服务仓库，建议命名为 `service.release.yaml`。

```yaml
apiVersion: infradex.release/v1
service: heartie-admin
version: 1.4.2
fleet: hk
machines: []
images:
  - workload: heartie-admin
    kind: Deployment
    namespace: default
    container: app
    image: registry.example/heartie-admin@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
configMaps:
  - file: deploy/configmap.yaml
nginx:
  - file: deploy/nginx/heartie.conf
    name: heartie.conf
migrate:
  jobFile: deploy/migrate-job.yaml
  waitTimeoutSeconds: 180
restart:
  - name: heartie-admin
    kind: Deployment
    namespace: default
check:
  url: https://heartie.example/healthz
  expectStatus: 200
  timeoutSeconds: 10
```

`machines` 为空表示使用 fleet 中具备该步骤所需角色的全部机器。非空时只能是 fleet 里已有的名字。

校验在本地完成：

1. `apiVersion` 只能是 `infradex.release/v1`。未知字段失败。
2. `service` 匹配 `^[a-z][a-z0-9-]{1,62}$`。`version` 匹配 `^[0-9]+\.[0-9]+\.[0-9]+([.-][A-Za-z0-9.]+)?$`。
3. `fleet` 非空。机器是否属于本机 fleet 文件，在 plan/apply 时检查。
4. 至少有一个 `images`、`configMaps`、`nginx`、`migrate` 或 `restart` 步骤。
5. 镜像引用匹配 `^[^[:space:]]+@sha256:[a-f0-9]{64}$`。含 `:latest` 的引用失败。`--allow-tag` 只放宽摘要要求，仍然拒绝 `:latest`，并在发布记录写明 `imagePolicy: tag`。
6. Kind 只允许 `Deployment`、`StatefulSet`、`DaemonSet`。迁移清单的 Kind 只能是 `Job`。
7. ConfigMap 和 Job 文件相对规范文件目录解析，拒绝绝对路径和 `..`。单文件最多 1 MiB。清单最多 16 个资源。
8. Nginx `name` 匹配 `^[A-Za-z0-9][A-Za-z0-9._-]{0,127}\.conf$`。内容最多 256 KiB。目标只可能是 `/etc/nginx/conf.d/<name>`。
9. `waitTimeoutSeconds` 和 `timeoutSeconds` 的范围是 1–180。
10. 规范文件不包含 DSN（`@tcp(`）、私钥、kubeconfig 正文或 shell 替换。

`infradexctl release validate -f <spec>` 只做上述文件校验。它不打开 SSH，也不写发布记录。

## 执行顺序

每台机器按这个顺序跑完或失败：

1. 读取现状，生成 plan。plan 不写远端。
2. ConfigMap：server-side dry-run，再按 `resourceVersion` 更新。
3. Nginx：SFTP 写入候选文件，运行固定命令 `nginx -t`。失败时用 SFTP 恢复原字节；新文件失败则 SFTP 删除。此阶段不 reload。
4. 迁移 Job。状态不是 success 时，该机器停止，不改镜像、不重启、不 reload。
5. 记录当前镜像后 set image。`resourceVersion` 不一致则失败。
6. rollout restart。
7. 本机 Nginx 文件有变更且 `-t` 已通过时，运行固定命令 `nginx -s reload`。
8. 可选 HTTP check。失败则该机器标记失败。不自动回滚镜像。发布记录保留改镜像前的引用。

允许的远端命令只有 `nginx -t` 和 `nginx -s reload`。列目录、读写和删除都用 SFTP，目录限定为 `/etc/nginx/conf.d` 和只读的 `/var/log/nginx`。

apply 默认串行、fail-fast。一台失败后，后续机器状态为 `not-started`。任一失败或未开始时退出码为 1。摘要逐台列出步骤。Nginx 文件恢复是唯一自动回滚，且只发生在当前这台机器的 `-t` 失败。

发布记录写到 `os.UserConfigDir()/Infradex/releases/<id>.json`，权限 0600。字段包括时间、service、version、spec sha256、imagePolicy、每台机器的步骤、改镜像前的引用和错误。Secret、kubeconfig 和 Nginx 文件正文不入库。

Git 中的规范文件是发版事实来源。漂移等于远端对象或文件哈希与规范文件不一致。

## 分期

| 期 | 交付 | 状态 |
| --- | --- | --- |
| 1 | 发版规范校验、fleet 文件、`infradexctl release validate` 和 `fleet list` | 已实现 |
| 2 | 单机和机群的 plan/apply，覆盖镜像、重启、ConfigMap、迁移 Job、Nginx 测试与 reload | 未实现 |
| 3 | 桌面机群状态、漂移、服务表、Nginx 只读 | 未实现 |
| 4 | 桌面确认后调用同一个 Apply；手工 Nginx 编辑带 `-t` 恢复 | 未实现 |

MySQL/PgSQL 控制台、用户/JWT、OSS 备份、实时日志、Pod exec 和 sudo 不在这四期内。
