# 产品或机器下线

只出计划，执行前逐项得到用户确认。

## 产品
1. `devopspadcli describe product:<p>` 和每个服务的 describe。
2. 对产品引用的每个域名、账号、机器、数据库执行 `devopspadcli refs <ref>`：
   - 只被这个产品使用 → 列入“可以取消 / 删除”（域名关闭自动续费、订阅取消、机器释放、数据库备份后删除）。
   - 还被其他产品使用 → 只解除引用。
3. 把清单给用户。用户执行后：产品 `stage: sunset`（保留档案），删除不再存在的资源文件或把机器设为 `state: retired`，`note add` 记录处理过程，validate，commit。

## 机器
1. `devopspadcli refs host:<name>`：列出集群、服务、域名解析、配置目标。
2. 每个引用都要先迁移或删除，validate 通过后才能删除 / 退役机器。
3. 退役：`state: retired`（保留历史）；彻底删除文件前确认没有引用。
