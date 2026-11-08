# DevOpsPad AI 构建与发布记录约定

将本段合并到实际应用仓库的 AGENTS.md，并填写本机配置；不要存储凭据：

- CLI：填写 devopspadcli 的绝对路径。
- 项目：填写本地 DevOpsPad 项目目录，或设置 DEVOPSPAD_PROJECT_DIR。
- 应用：填写 project.yaml 中的应用名称。
- 规范：阅读 DevOpsPad 的 docs/ai-execution.md 与 docs/integration/run-event.schema.json。

进行实际镜像构建或部署时：

1. 从当前仓库读取真实 repository、完整 commit SHA、branch、dirty 状态。记录当前 AI 名称与任务/会话 ID，不编造版本、摘要或测试结果。
2. 为每次执行生成 runId，为每次上报生成事件 id。先把事件 JSON 保存到执行端，再用 `devopspadcli runs record --project <dir> -f <file>` 上报。
3. start 记录成功后才执行构建或调用已授权的发布工具。记录失败时停止后续副作用，并报告原因。
4. 实际构建/推送后上报 build、push 步骤；成功必须取得真实完整 sha256 镜像摘要。任一步失败都记录 failed，最终补报 failed/cancelled。
5. AI 直接发布时关联成功 buildId，沿用仓库现有部署权限与操作规程；上报不会替代部署授权。实际核验 rollout、运行镜像后，记录 rollout、image-check，再结束。
6. 上报错误时保留事件 JSON，重试原 ID 和原内容，不得为补记录而重新执行构建或发布。已经发生的操作无法回滚为“未发生”；记录结果不确定时明确说明。
7. 对日志/错误摘要脱敏；不要上报密码、token、私钥、kubeconfig 或带认证参数的命令。原始日志保留本机 logs 目录，Git 同步只同步项目索引与执行记录。
8. 只有 finish 被接受且状态符合实际结果后，才报告记录完成。进程中断留下 running 时，先核实，不擅自改成 succeeded。

发布镜像时使用 `devopspadcli release plan` → `release apply --plan <id>`（或 `release image … --yes`），带迁移时加 `--migrate <job.yaml>`；不要用 `kubectl set image`。它会自动写发布记录并核对运行中的镜像摘要，此时不需要再用 `runs record` 上报 release。

需要新的 Secret、拉取凭据或让工作负载引用 Secret 时，使用 `devopspadcli secret set`（值从文件或 stdin）和 `devopspadcli workload patch`，不要 `kubectl create secret` / `kubectl edit`；不要把值写进命令行、日志或回复。

修改宿主机 Nginx 配置时使用 `devopspadcli nginx get` → 修改 → `devopspadcli nginx set --expect-revision <revision>`，不要 `scp` + `ssh` + `nginx -s reload`。

修改应用 ConfigMap 或重启 Deployment 时，使用 `devopspadcli config get/set` 和 `devopspadcli workload restart`（见 docs/ai-execution.md“修改应用配置与重启”），不要直接 `ssh` + `kubectl`。`config set` 必须带 `config get` 输出的 `--expect-revision`；不要把 ConfigMap 内容写进日志或上报内容。

本约定针对发布相关构建和部署；纯代码检查、单元测试、浏览器测试不自动创建制品记录。不要向生产项目写入测试样例。
