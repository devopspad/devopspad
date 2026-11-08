# 查找：部署在哪、怎么发版、依赖什么

1. 不确定名字时先列：`devopspadcli get service`、`devopspadcli report matrix`。
2. 服务：`devopspadcli describe service:<product>/<service> -o json`。
   - `deployments`：已合并环境默认值的集群、命名空间、选择器、工作负载或宿主机 / systemd unit / 静态托管。
   - `releaseNotes`：发版注意事项文件，发版相关问题必须读它。
   - `references`：依赖的数据库（`#part` 是库名）、账号、镜像仓库、配置文件目标机器。
   - `recentRecords`：最近的操作记录。
3. 机器 / 集群 / 数据库 / 域名 / 账号：`devopspadcli refs <ref>` 看谁在用；`describe <ref>` 看详情和相关笔记。
4. 产品全貌：`devopspadcli describe product:<p>`，再对每个服务 describe。
5. 配置文件原文在 `products/<p>/<configs[].source>`，可以直接读（不含密钥，密钥是 `${secret:…}` 占位）。

回答时给出 ref 和文件路径，方便用户核对。档案里没有的信息直接说没有登记，不要猜。
