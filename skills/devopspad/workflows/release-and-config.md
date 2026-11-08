# 发版、改配置、重启

1. `devopspadcli describe service:<p>/<s> -o json`：确认目标环境的 deployment（集群、命名空间、工作负载）和 `release.method`。
2. 读 `releaseNotes` 文件；有前置步骤（先发 api、先跑迁移）按它执行，有疑问先问用户。
3. 按 method 执行：
   - `devopspad`：用 DevOpsPad 桌面发版，或 `devopspadcli config get/set`、`devopspadcli workload restart`（需要 DevOpsPad 应用项目里登记过该应用；见 `devopspadcli --help`）。
   - `devopspad-spec`：`devopspadcli release validate -f <代码仓库中的 spec>`。
   - `ci`：触发代码仓库的 CI，不要手工部署。
   - `manual`：按 release notes 执行，执行前把计划给用户确认。
4. 不要直接 `ssh` / `kubectl` 修改线上；`config set` 必须带 `config get` 返回的 `--expect-revision`。
5. 改的是档案库里登记的配置文件（`configs[].source`）时：先改档案库文件并提交，再把同样内容下发到线上，保持两边一致。
6. 线上操作不是由 DevOpsPad 记录的（manual、ci、手工 ssh）→ `devopspadcli note add --refs service:<p>/<s>@<env> -m "<做了什么、结果>" --actor ai/<名字>`，然后 `archive commit -m "record(service:<p>/<s>): …"`。
7. 汇报：做了什么、结果、验证方式（健康检查 URL 见 endpoints）。
