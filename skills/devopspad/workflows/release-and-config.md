# 发版、改配置、重启

1. `devopspadcli describe service:<p>/<s> -o json`：确认目标环境的 deployment（集群、命名空间、工作负载）和 `release.method`。
2. 读 `releaseNotes` 文件；有前置步骤（先发 api、先跑迁移）按它执行，有疑问先问用户。
3. 按 method 执行：
   - `devopspad`：镜像用 `devopspadcli release plan … --artifact <runId>` 给用户看计划，确认后 `devopspadcli release apply --plan <id>`；有迁移时加 `--migrate <job.yaml>`。配置用 `devopspadcli config get/set`，重启用 `devopspadcli workload restart`（都需要 DevOpsPad 应用项目里登记过该应用；见 `devopspadcli --help`）。
   - `devopspad-spec`：`devopspadcli release validate -f <代码仓库中的 spec>`。
   - `ci`：触发代码仓库的 CI，不要手工部署。
   - `manual`：按 release notes 执行，执行前把计划给用户确认。
4. 不要直接 `ssh` / `kubectl` 修改线上；`config set` 必须带 `config get` 返回的 `--expect-revision`。
5. 改的是档案库里登记的配置文件（`configs[].source`）时：先改档案库文件并提交，再把同样内容下发到线上，保持两边一致。
6. 线上操作不是由 DevOpsPad 记录的（manual、ci、手工 ssh）→ `devopspadcli note add --refs service:<p>/<s>@<env> -m "<做了什么、结果>" --actor ai/<名字>`，然后 `archive commit -m "record(service:<p>/<s>): …"`。
7. 汇报：做了什么、结果、验证方式（健康检查 URL 见 endpoints）。

## Cloudflare 路由和 DNS

暴露新的路径前缀（例如 `/admx`）通常要同时改 Nginx 和 Cloudflare：

1. `devopspadcli edge routes list --zone <zone>` 看现有路由；需要绕过 Worker 时 `edge routes add --zone <zone> --pattern '<zone>/admx*' --script none`（不带 --yes 先给用户看前后对比）。
2. 用户确认后加 `--yes` 执行；DNS 用 `edge dns set`。不要让用户去 Cloudflare 控制台手改。
3. 生成的记录用 `devopspadcli archive commit -m "record(domain:<zone>): add route …" --actor ai/<名字>` 提交。
