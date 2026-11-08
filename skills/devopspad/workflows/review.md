# 定期检查：到期、费用、闲置

0. 先刷新真实数据（只读网络访问，告诉用户）：`devopspadcli scan`（可用 `--kind domain|cluster|host` 限定）。
   - 域名：Cloudflare 的 zone、DNS 记录、注册到期日（没有 token 时只用 RDAP），以及每个 HTTPS endpoint 的证书。
   - 集群：节点和所有命名空间的工作负载（远端 kubeconfig 经 SSH 临时读取，用完删除）。
   - 机器：系统、规格、运行中的 systemd 服务、Nginx 配置文件哈希（只执行固定的只读命令）。
   结果只存本机 `.devopspad/observed/`，不进 Git。
   然后 `devopspadcli diff -o json`：
   - `error`：域名或证书已过期、证书无效 → 立刻告诉用户。
   - `warn`：档案和实际不一致（到期日、自动续费、DNS 记录指向、注册商）→ 以实际值为准更新档案，一次提交 `archive(domains): sync with Cloudflare scan`；证书 21 天内到期 → 提醒检查自动续期。
   - 集群：`no matching workload` → 档案里的部署在集群里找不到（下线了还是选择器写错了，问用户）；`running but not in the archive` → 集群里有、档案里没登记的服务，帮用户归到产品的 service.deployments。
   - 机器：Nginx 配置与档案副本不一致 → 先问用户以哪边为准；`not archived` 的配置 → 建议存进对应服务的 configs/；角色对应的服务没在运行 → 告诉用户。
   - `info`：Cloudflare 里有但档案里没有的域名 → 建议 `archive import --cloudflare`；未扫描 / 扫描过期 → 重新 scan。
1. `devopspadcli report renewals --observed -o json`（`--observed` 合并上次扫描到的证书到期日）（默认档案库设置的天数，`--within 60` 自定义，`--all` 全部）。`days < 0` 表示已过期；`autoRenew: false` 的要提醒用户手动续费。
2. `devopspadcli report cost`：按币种的月度合计（不换算汇率），每个产品独占的费用和共享资源。
3. `devopspadcli report orphans`：没有任何产品使用的资源，可能在浪费钱，逐个问用户是否下线（见 sunset.md）。
4. `devopspadcli secrets check`：本机缺失的密钥。
5. 用户确认续费或更新后，修改 `renewal.expiresAt` / `cost`，validate，commit `archive(<ref>): renew until <date>`。
