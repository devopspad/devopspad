# 02 问题与风险

按影响从大到小排列。每项都标了证据位置，方便核对。

## 1. 产品定位漂移，主计划已过时

`PLAN.md`（2026-10-02）仍把产品描述为“接近 Navicat 的连接客户端”，并写着：

- 演示模式 + SQLite 演示执行器 —— 现已删除（`frontend/src/demo.ts` 已删，新增 `no-demo.spec.ts`）。
- 视觉方向是“浅灰任务面板、白色编辑区” —— 现为 Orca 深色。
- “所有可见产品文案使用美式英文” —— Infra 界面和错误信息大量使用中文（如 `app_ops.go` 的“制品不存在，请刷新项目”，导航“中间件”）。

实际投入最多的是 Infra / 发版 / AI 执行记录，PLAN.md 里只有一个“OPC 机群发版（进行中）”小节。**主计划已经不能指导开发。**

## 2. “只读工作台、写入只来自 internal/release”这条边界已失守

README 和 OPC 规范都写着：查询工作台只读，发版和 Nginx 写入只来自 `internal/release`。实际有三条写路径：

| 写操作 | 实现位置 |
| --- | --- |
| Deployment 镜像 / 扩缩容 / 重启 / 回退 | `internal/release/desktop.go` ✅ |
| ConfigMap 保存 | `internal/workspace/configmap.go:60`（直接 `Update`）❌ |
| Nginx 配置写入与 reload | `internal/workspace/nginx.go` + `nginx_remote.py` ❌ |

后两者有版本校验，但不经过 dry-run、Lease、计划 / 确认，也不统一写发版记录格式。安全模型靠“写入集中在一个包”来保证，这一点现在不成立。

## 3. Nginx 实现与 OPC 规范直接矛盾

[OPC 规范](../superpowers/specs/2026-10-21-opc-release-design.md) 规定：

- 只允许 `nginx -t` 和 `nginx -s reload` 两条远端命令，读写删用 SFTP。

实际实现（`internal/workspace/nginx.go:84`）：

- 通过 SSH 执行 `python3 -c '<内嵌脚本>'`，脚本内调用 `systemctl reload nginx`。
- 远端要求 Python 3 + systemd。

两种做法必须选一个，然后改代码或改规范。

## 4. 两套发版模型并存，没有收敛

- **桌面发版**：以 `project.yaml` 的环境 / 应用为中心，单 Deployment、单容器，有 Lease 和计划。
- **OPC 规范**：以 `service.release.yaml` + `fleet.json` 为中心，多机串行，含 ConfigMap、迁移 Job、Nginx、健康检查。目前只有校验，plan/apply 未实现。

规范写着“两者调用同一套 `internal/release`”，但 `desktop.go` 和 `spec.go` 互不引用。同时存在三份描述“部署到哪里”的配置：

| 配置 | 核心概念 |
| --- | --- |
| `workspace.json` | 连接（Connection） |
| `project.yaml` | 环境 → 连接 + 命名空间；应用 → 选择器；`nginxPath` |
| `fleet.json` | 机群 → 机器 → 连接 + 角色（cluster / edge） |

“环境”和“机群”、“应用”和“service”是什么关系，没有定义。继续开发 OPC 第 2 期之前必须先统一领域模型，否则会出现第三套发版路径。

## 5. 大量未提交代码

见 [01-current-state.md](01-current-state.md#提交状态)。Infra 全部功能、相关测试和文档都只在工作区里。建议先拆成几个逻辑提交入库，再做任何重构。

## 6. 前端巨型组件

`OpsConsole.tsx`（约 2550 行）和 `App.tsx`（约 2100 行）承载了大部分状态和视图。新增 OPC 机群视图前，最好先按页面拆分（节点、负载、制品、发布记录、配置），并抽出数据请求和缓存层。

## 7. 安全与信任模型需要重新审视

产品从“只读”变成“能改生产”后，原有的保护不够：

- 环境标签只是提示，没有“生产环境需要二次确认 / 输入名称”之类的分级。
- 发版记录、运行记录是本地 JSON，可被任意修改，文档也写明“不是防篡改审计账本”。小团队可以接受，但要明确对外承诺。
- `runs record` 接受执行端自报的 `rollout`、`image-check` 证据，DevOpsPad 不做独立验证。
- Nginx 写入需要远端用户有配置与 reload 权限，实际往往意味着 sudo / root。

## 8. 商业化门槛仍全部未完成

PLAN.md 第 13 节列出的 P0 门槛（收费方式、签名公证、安装升级、真实桌面验收）没有进展。如果近期不打算商业化，应在文档中明确写“当前为自用 / 内部工具”，减少无效约束。

## 9. 验收缺口

真实 worker 节点上的 rollout 和日志、真实 Registry、真实 SSH + RBAC 组合尚未验收。发版这类写操作，比只读查询更需要真实环境的端到端测试。
