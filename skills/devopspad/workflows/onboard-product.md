# 新产品 / 新服务登记

1. 问清楚（缺什么问什么，不要猜）：产品名（小写短横线）、显示名、阶段（idea/building/live/paused/sunset）、域名、部署环境（用哪个集群 / 命名空间 / 机器）、代码仓库、用到的第三方账号。
   - 列出候选：`devopspadcli get cluster`、`get host`、`get domain`、`get provider`。
2. 依赖的资源不存在时，先按 register-resource.md 登记（域名、账号、机器）。
3. `devopspadcli new product <name>`，编辑 `products/<name>/product.yaml`：stage、domains、providers、environments。
4. 每个服务：`devopspadcli new service <svc> --product <name>`，填写 type、repo、build、release、deployments（environment 必须是产品里定义的）、endpoints、dependsOn、secrets（只写引用）。
   - 有发版注意事项时写到 `products/<name>/notes/release-<svc>.md`，并在 `release.notes` 引用。
   - 有 Nginx / ConfigMap 等配置文件时放到 `products/<name>/configs/<svc>/<env>/`，在 `configs` 登记 target。
5. `devopspadcli archive validate -o json` 直到通过。
6. 告诉用户新增了哪些文件、还缺哪些信息，确认后 `devopspadcli archive commit -m "archive(product:<name>): add product" --actor ai/<你的名字>`。

完成条件：校验通过、已提交、缺失信息已告知用户。

## 批量登记（一次几十个产品）

矩阵里产品很多、而且大多共用两台机器和同一个集群时：

1. 先确认共享资源已登记：`devopspadcli get host`、`get cluster`、`get provider`。
2. 跟用户约定一张清单（产品名、阶段、域名、命名空间、标签），缺的字段先空着，不要编。
   - 标签约定（`metadata.labels`）：`line=<产品线>`（如 tools、games、ai、content）和 `revenue=paid|free|none`。用户另有分类时以用户为准，并在整个档案库保持一致；先 `devopspadcli get product -o json` 看已有的标签值，不要造新的同义词。
3. 逐个 `devopspadcli new product <name>`（域名不存在时 `new domain <fqdn>`），按清单填写；产品只有一个站点时，service 可以先不建。
4. 全部写完后只跑一次 `devopspadcli archive validate -o json`，按错误统一修正。
5. 一次提交：`devopspadcli archive commit -m "archive(products): register <N> products" --actor ai/<你的名字>`。
6. 用 `devopspadcli report matrix -l <标签>` 让用户核对，列出还缺哪些信息（到期日、费用、仓库）。
