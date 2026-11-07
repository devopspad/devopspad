# AI 构建与发布接入规范

AI 是实际执行者；DevOpsPad 接收执行记录、关联源码与镜像、展示构建和发布历史。无需创建流水线，也无需运行桌面窗口。`devopspadctl` 与桌面读取同一个本地项目目录，可以在其他机器上对各自的 Git 副本上报后同步。

## 调用入口

```sh
go build -o build/bin/devopspadctl ./cmd/devopspadctl
build/bin/devopspadctl runs record --project /absolute/path/to/project -f event.json
build/bin/devopspadctl runs record --project /absolute/path/to/project -f - < event.json
build/bin/devopspadctl runs list --project /absolute/path/to/project
build/bin/devopspadctl runs get --project /absolute/path/to/project --id <run-id>
```

不传 `--project` 时依次读取 `DEVOPSPAD_PROJECT_DIR`、DevOpsPad 配置目录下 `project-path`、默认 `applications` 目录。配置目录可通过 `DEVOPSPAD_CONFIG_DIR` 指定。项目必须已存在，应用须先在桌面或 CLI 接入；传错目录不会悄悄建立一个空项目。

新应用首次构建前，可显式登记本地映射，无需先创建 Kubernetes 工作负载：

```sh
build/bin/devopspadctl cluster attach --connection heartie-pro --namespace default --environment heartie-pro --application thumbspark --selector app=thumbspark
```

`--application` 和 `--selector` 必须同时提供，且只能用于 `attach`。此模式验证已保存的集群连接及本地项目配置，复用配置版本校验和写入锁，只登记映射，不连接集群、创建或修改工作负载。省略两个参数时仍从集群发现已有工作负载。

`runs` 成功时 stdout 为完整 JSON 执行记录，退出码 0；记录失败退出码 1，stderr 为 JSON `error`。参数错误退出码 2。`runs` 不执行 Docker、推送镜像或变更 Kubernetes。现有的 AI/构建工具/发布工具继续负责实际执行；上报成功只表示记录已落盘。

## 修改应用配置与重启

修改应用 ConfigMap、滚动重启 Deployment 时使用下面的命令，不要直接 `ssh` + `kubectl`。它们和桌面“应用配置”“重启”走同一套检查：按项目环境和应用解析本机同名集群连接，只允许读写该应用工作负载引用的 ConfigMap，重启前做服务端 dry-run，执行时取 Deployment 发布锁并核对 resourceVersion，等待 rollout 就绪，每次变更写入一条操作记录（桌面“发布记录”可见）。

```sh
# 读取一个键，写入 0600 文件；stdout 打印 resourceVersion 和 sha256
devopspadctl config get --environment heartie-pro --application thumbspark \
  --configmap thumbspark-config-toml --key config.toml -o config.toml

# 写回：先打印 diff；ConfigMap 自 get 之后被改过则拒绝写入
devopspadctl config set --environment heartie-pro --application thumbspark \
  --configmap thumbspark-config-toml --key config.toml -f config.toml \
  --expect-revision <resourceVersion> [--dry-run]

# 写入后在同一条记录里滚动重启（需要 --yes）
devopspadctl config set ... --expect-revision <resourceVersion> --restart --yes [--workload thumbspark]

# 只重启：不带 --yes 只打印计划，带 --yes 执行并等待就绪
devopspadctl workload restart --environment heartie-pro --application thumbspark \
  --workload thumbspark [--yes] [--timeout 180s]
```

- `config get` 必须指定 `-o <文件>` 或 `-o -`（输出到 stdout，元数据改到 stderr），避免把可能含密钥的内容意外打到终端或日志。
- `config set` 只修改已存在的文本键，保留其他键、binaryData 和元数据；不可变 ConfigMap 拒绝修改。内容不变时不写入、不记录。`--dry-run` 只打印 diff；带 `--restart` 时还会验证重启计划。
- 记录只保存 ConfigMap/键名、增删行数、修改前后 sha256 前缀和 resourceVersion，不保存内容。diff 只打印到执行端 stdout，执行端自行决定是否保留。
- `--workload` 默认等于应用名。`--timeout` 最长 180 秒（发布锁有效期 300 秒）。超时或失败时变更可能已生效，先核对集群状态，不要直接重试。
- 退出码：成功 0；读取、校验或执行失败 1（stdout 仍输出最终记录 JSON）；参数错误 2。修改配置不会自动重启，环境变量和 subPath 挂载需要重启后生效。

## 数据契约

机器可读结构见 [run-event.schema.json](integration/run-event.schema.json)。CLI 还执行跨事件和项目关联校验；JSON Schema 单独校验不代表记录可接受。

每次执行创建新的 UUID `runId`，每次上报创建新的 UUID `id`。把待发送 JSON 先存到执行端本地，收到成功响应前保留原文件。网络/进程或项目锁问题导致重试时，**重发同一文件，保持两个 ID 及内容不变**。同一 runId 下重复事件返回原记录，不重复追加；同一事件 ID 携带不同内容会被拒绝。

| 事件 | 必填信息 | 接受后的状态 |
| --- | --- | --- |
| start | kind、application、version、actor、source、recipe | running，服务端记录开始时间 |
| step | name、status（succeeded/failed）、message 可选 | 追加时间戳和步骤结果 |
| finish | status（succeeded/failed/cancelled）、message；成功时 image | 结束记录，不能再追加或修改 |

`actor` 包含执行端类型 `ai` / `human`、名称、会话或任务 ID。`source` 包含仓库、完整 40/64 位提交 SHA，可带分支及 dirty 标记。`recipe` 填构建/发布方式的简短名称，不要填带凭据的完整命令。外部时间不是完成依据，记录时间由 CLI 所在主机生成。

一次运行最多 256 条事件、1 MiB；单次输入最大 64 KiB。运行中断不会自动算成功；页面显示“执行中 / 待核实”，需要执行端核实后补报失败或取消。失败后的重新执行使用新的 runId。事件记录是本地项目文件，不是防篡改审计账本。

## 构建协议

1. 在任何实际构建之前发送 `start`，`kind: build`。记录失败时停止，不继续构建。
2. 执行既有构建命令，记录名为 `build` 的步骤结果。
3. 推送镜像，记录名为 `push` 的步骤结果，并从实际 Registry / 构建器输出取得完整镜像摘要。
4. 发送成功 `finish`，image 必须为 `registry/repository@sha256:<64 位小写摘要>`。只有成功 build、push 步骤和有效摘要齐备才允许成功；其他仍失败的步骤也会阻止成功。
5. 失败时发送失败步骤及 `finish.status: failed`，message 说明失败原因，省略 image。取消时发送 cancelled。

成功构建直接成为制品库条目，制品 ID 与 runId 一致。制品由成功运行记录派生，不需要同时改写两份文件，避免中途退出产生半份成功数据。该条目可以在桌面生成真实发布计划，之后桌面发布记录保留 `runId`，可回到构建详情。

[示例事件](../examples/ai-integration/)仅供字段结构参考。提交 SHA、会话、版本、镜像摘要和事件 ID 必须替换为实际值，不得将示例伪装成真实执行记录。分支或镜像 tag 不能代替完整提交 SHA / digest。

## AI 直接发布的协议

外部 AI 通过已有发布系统执行时，新建 `kind: release` 的 start，额外提供：

```json
{
  "target": {
    "environment": "heartie-pro",
    "workload": "your-deployment",
    "container": "your-container",
    "buildId": "successful-build-run-uuid"
  }
}
```

目标环境必须存在。buildId 必须是同一应用、版本、源码的成功构建；镜像从该构建读取，连接名称和命名空间从项目环境读取。开始记录后才调用现有发布工具。

记录发布步骤后，必须记录 `rollout` 与 `image-check` 两个成功步骤。实际读取集群确认就绪和镜像摘要与构建一致后，才能发送成功 finish；finish 的 image 必须等于构建摘要。失败或不确定的发布结果不得按成功上报。

这些步骤是执行端提交的证据，CLI 不伪装成独立集群验证。外部 release 同时显示在“AI runs”和对应环境、应用的“Releases”页，标记“External release / Reported”，详情可回到该次发布执行记录。桌面自身控制 Kubernetes 的记录保留原有状态和回退功能；外部记录不提供未验证的直接回退按钮，可从制品库重新发布已知版本。上报外部 release 事件本身不会执行一次额外发布。

## 存储、共享与日志

`runs/<runId>.json` 存放规范版本、状态、来源、执行者、目标及事件历史。文件 0600，使用跨进程独占锁和原子替换；终态只接受已存在事件的完全相同重放。Git 同步包含 runs，构建派生制品随运行记录共享。两台电脑应创建不同的 runId；同一 run 不支持多机离线同时编辑，也不是 CRDT。

原始日志保留在执行端；事件只保存有限摘要和可选 `logs/...` 相对路径。DevOpsPad 不自动读取或同步 logs。调用方须先清除 token、密码、环境变量、认证 URL、私钥和 kubeconfig 正文；接口拒绝带用户名密码或查询参数的 URL 仓库地址，但自由文本不是自动脱敏工具。

将 [AGENTS 接入片段](../examples/ai-integration/AGENTS.fragment.md) 合并到实际服务仓库的 AI 指令中，让后续 AI 构建/发布遵守本规范。仅安装 CLI 不会拦截其他工具，也不会自动捕获绕过此接口的操作。
