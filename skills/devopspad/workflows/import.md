# 建档案库、导入、换电脑恢复

## 新建
1. 建议独立私有 Git 仓库：`devopspadcli archive init ~/opc-archive --name <name>`（会 git init、安装 pre-commit 校验钩子，并设为默认档案库）。
2. 用户添加远端：`git -C ~/opc-archive remote add origin <私有仓库 URL>`。

## 从 Cloudflare 导入全部域名

1. 让用户在 Cloudflare 创建 API token（权限 Zone Read + DNS Read，范围所有 zone），然后自己执行 `devopspadcli secrets set keychain:archive/providers/cloudflare/api-token` 粘贴 token。不要让用户把 token 发给你。
2. 预览：`devopspadcli archive import --cloudflare --dry-run`；确认后去掉 `--dry-run`。会生成 `provider:cloudflare`（引用上面的 token）和每个 zone 一个 Domain，到期日来自 Cloudflare Registrar 或 RDAP。
3. 按 TODO 处理：自动续费未知、非 Cloudflare 注册的域名、把域名关联到产品（`report orphans` 列出未关联的）。
4. validate 通过后 commit `migrate(domains): import Cloudflare zones`。

## 从 DevOpsPad 桌面导入
1. 预览：`devopspadcli archive import --workspace --fleet --dry-run`；有应用项目时加 `--project <目录> --product <产品名>`。
2. 确认后去掉 `--dry-run` 执行。导入不复制密钥：引用指向桌面已存在的钥匙串条目或私钥文件路径。
3. 按输出的 TODO 逐项补齐（问用户：机器的 provider / 费用 / 到期、数据库在哪台机器、项目是否混了多个产品）。
4. validate 通过后 commit `migrate(archive): import desktop configuration`。

## 换电脑
1. 克隆档案库，`devopspadcli archive use <目录>`。
2. `devopspadcli secrets check`：列出本机缺失的密钥；让用户逐个 `devopspadcli secrets set keychain:<account>`，私钥文件从备份恢复到 `file:` 指定的路径。
3. 生成桌面连接：让用户在 DevOpsPad 桌面 Archive 页面点 Sync connections；或先关闭桌面应用，再 `devopspadcli connect sync --dry-run` 预览、`devopspadcli connect sync` 执行（桌面开着时它会用内存里的旧列表覆盖文件）。
