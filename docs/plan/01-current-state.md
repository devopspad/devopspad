# 01 现状盘点

基于 2026-11-06 工作区（含未提交改动）。

## 技术栈

Wails v2 桌面应用。后端 Go（ego / egorm / eredis、client-go、x/crypto/ssh），前端 React + Vite + Tailwind + CodeMirror。另有命令行 `devopspadcli`，与桌面共用 `internal/`。

## 两大功能区

### 中间件（查询工作台，只读）

- 7 类连接：MySQL、Redis、MongoDB、Kafka、ClickHouse、Kubernetes、K3s。
- 全引擎单跳 SSH 隧道（密码 / 私钥 / Agent，known_hosts 或指纹校验，不回退直连）。
- 资源树、多标签查询、数据库/上下文作用域、结果筛选排序分页、CSV 导出、收藏、会话历史。
- 集群资源详情：Overview / Events（按 UID）/ Logs 快照 / JSON。
- 结果预算（500 行、8 MiB）、20 秒超时、取消。
- 凭据存 OS keychain；草稿恢复（默认关闭）。
- AI：OpenAI 兼容接口，只生成建议，不自动执行，默认不发送资源名。

### Infra（应用运维，有写操作）

- 本地项目目录：`project.yaml`（环境、应用、选择器）、`artifacts/`、`releases/`、`runs/`，可通过 Git 手动共享。
- 节点 / 机器视图、Deployment / Pod、日志快照、事件、metrics-server 指标。
- 桌面发版（`internal/release/desktop.go`）：已有 Deployment 的单容器镜像发布、回退、扩缩容、重启。server dry-run → 计划（10 分钟、单次）→ Lease 锁 → resourceVersion 校验 → rollout 验证 → 本地记录。
- 应用配置：读取引用的 ConfigMap，可编辑并保存回集群（UID + resourceVersion 校验）。
- 宿主机 Nginx：通过 SSH 读写 `/etc/nginx/conf.d/*.conf`，`nginx -t` 后 reload，失败恢复。
- AI 执行记录：`devopspadcli runs record` 上报 build / release 事件，成功构建自动成为制品。
- `devopspadcli`：`release validate`（OPC 发版规范校验）、`fleet list`、`runs`、`cluster inspect/attach`。

## 代码规模与结构

| 区域 | 说明 |
| --- | --- |
| `app.go`、`app_ops.go`、`app_nginx.go`、`app_diagnostics.go` | Wails 绑定，约 50 个导出方法 |
| `internal/workspace/` | 连接、keychain、引擎适配器、SSH、AI、集群读取，以及 ConfigMap 和 Nginx 写入 |
| `internal/ops/` | 本地项目目录、制品、运行记录、Git 同步、attach |
| `internal/release/` | OPC 规范校验（`spec.go`）+ 桌面发版引擎（`desktop.go`） |
| `internal/fleet/` | 本机 `fleet.json` |
| `frontend/src/` | `App.tsx` 约 2100 行，`OpsConsole.tsx` 约 2550 行，其余组件 200–400 行 |

Go + TS 业务代码约 2 万行。

## 测试与验证

- Go：96 个测试函数，`go test ./...` 当前全部通过（2026-11-06 本地运行）。
- Playwright：16 个 spec，约 49 个用例，使用 Wails 接口测试桩。
- 本地 Docker 集成测试覆盖 6 类服务 + K3s API（无 worker 节点）+ macOS keychain。
- CI：GitHub Actions 构建、测试、未签名 macOS 产物；尚未确认托管 runner 实际跑通。

尚未验收：运行中 Pod 的真实日志、带 worker 的真实 rollout、真实 Registry 拉取、原生文件对话框、Windows / Linux、签名公证。

## 提交状态

最近一次提交是 `c19f8f1`（Orca 深色风格）。**整个 Infra 功能区（`internal/ops`、`internal/release/desktop.go`、ConfigMap / Nginx 写入、约 10 个前端组件、7 个新 spec、`docs/ai-execution.md` 等）都还没有提交**，并删除了 demo 模式。这是当前最大的单点风险。
