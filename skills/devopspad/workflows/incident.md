# 排障

1. 定位拓扑：`devopspadcli describe service:<p>/<s>`（部署位置、依赖、配置、最近记录）；涉及机器时 `describe host:<name>` 看相关笔记。
2. 查最近变更：describe 的 recentRecords，以及 `git -C <archive> log --oneline -20`。
3. 只读排查：DevOpsPad 桌面的日志 / 事件 / 指标，或用户授权的只读命令。没有用户确认不要重启、改配置。
4. 需要修复时按 release-and-config.md 执行。
5. 结束后写笔记：`products/<p>/notes/<日期>-<主题>.md` 或 `notes/…`，front matter 带 `refs`、`date`、`tags: [incident]`；内容写现象、原因、处理、后续。再 `archive commit -m "note(<ref>): …"`。
