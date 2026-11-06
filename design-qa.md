# Desktop reference design QA

Source: artifacts/reference-desktop-style.png (1920 × 1195, 1×).
Implementation: artifacts/desktop-reference-workbench-iteration-1.png (1920 × 1195 CSS pixels, deviceScaleFactor 1).
Scope: adapt the supplied OfficeDex desktop visual language to DevOpsPad, retaining its database and cluster workflows. Slides, hardware photos, Office ribbon commands, fake OS controls and the reference brand are not part of the requested product.

## Iteration 1
Paired source and rendered screenshot opened together in the comparison tool input. Browser rendering verified through the project's existing Chrome/Playwright workflow; no in-app Browser capability was exposed in the session tools.

- [P2] Left task panel too narrow at the reference viewport (365px versus approximately 426px). Increase it to 420px at large widths while retaining narrower variants for 1480px and 1000px.
- [P2] Assistant text too small relative to the supplied screenshot (13px body, 17px heading). Increase large-screen body to 15px, heading to 19px, and give the prompt and file card more breathing room.
- [P2] Residual green text in table headings, totals and assistant header. Replace it with neutral gray tokens; keep green only for semantic status.
- Behavior regression found and fixed: assistant file card reused the saved-query button name. Rename it to Open current query so the actions remain distinguishable.

## Iteration 2
- The first-pass typography, width and color changes are visible in the paired 1920 × 1195 comparison.
- [P1] At 1480 × 940 and 1000 × 680, the implicit CSS grid row grows to its min-content height, clipping the composer, rail controls and status bar. Evidence: artifacts/desktop-reference-1480-iteration-2.png and artifacts/desktop-reference-1000-iteration-2.png.
- Fix: explicit minmax(0, 1fr) row; min-height: 0 for direct panels and scrolling assistant content; prevent composer shrinking. Add viewport-bound assertions for Send message, Settings and the status bar.
- [P2] Connection inputs still inherit more-specific green borders from the old form styles. Replace at matching specificity and neutralize the preview notice.

## Final comparison

Source and final rendered screenshots were opened together for visual comparison, followed by focused panel inspection. Reference-size rendering uses 1920 × 1195 CSS pixels at deviceScaleFactor 1.

| Surface | Evidence and result |
| --- | --- |
| Layout | [Workbench](artifacts/desktop-reference-workbench.png): narrow rail, 420px left assistant, Explorer and right workspace reproduce the reference's panel hierarchy. Query tabs sit at the top. |
| Typography | [Assistant](artifacts/desktop-reference-assistant.png): system sans-serif, larger task heading and readable prompt/body spacing; compact labels retained in the data workspace. |
| Color and surfaces | Neutral gray task panel, white editor, thin borders and charcoal primary actions; semantic status colors retained. [Connection dialog](artifacts/desktop-reference-dialog.png) now uses matching neutral controls. |
| Icons and assets | Consistent thin outline icons; DevOpsPad identity retained. No decorative bitmap assets are needed for this infrastructure workspace. |
| Density and detail | [Editor and results](artifacts/desktop-reference-editor.png): flat document tabs, compact toolbars, aligned table cells and persistent status bars. Office slide imagery is intentionally replaced by working query/data views. |

Responsive evidence: [1480 × 940](artifacts/desktop-reference-1480.png), [1000 × 680](artifacts/desktop-reference-1000.png), and [Connections](artifacts/desktop-reference-connections.png). The composer, Run query, Settings and status bar remain within the viewport. Assistant content scrolls independently; narrow tables scroll horizontally inside the results region. Long Explorer labels ellipsize.

Validation: 23 Playwright tests passed after the layout redesign; 11 relevant layout/workspace tests passed after the viewport fix; all 3 layout tests passed again after the final narrow-width adjustment. TypeScript unused-code checks and the final Wails desktop build passed. The resulting macOS app remained running during a 3-second startup smoke check. Local Vite preview returned HTTP 200 when checked directly without the environment's proxy.

Remaining minor polish: [P3] the Connections add card and demo CTA retain a small legacy green accent. No blocking visual or interaction findings remain for this reference-style adaptation. This is not a pixel-identical Office editor clone or a formal accessibility audit. Windows/Linux and native WebView visual comparison remain outside this review.

final result: passed

---

## 2026-10-23 — 本地应用服务界面

本节记录本次新增应用服务的 QA，前面的数据库工作台记录保留。

**Source visual truth**

- 用户 Image #3：`/var/folders/sy/hk33q0lx62j_gz1l5wcx_5lm0000gn/T/codex-clipboard-ZW2Nn6.png`，2048×934，工作负载列表。
- 用户 Image #4：`/var/folders/sy/hk33q0lx62j_gz1l5wcx_5lm0000gn/T/codex-clipboard-PLpe8v.png`，2048×1102，展开的 Pod 分组。
- 用户 Image #2：`/var/folders/sy/hk33q0lx62j_gz1l5wcx_5lm0000gn/T/codex-clipboard-7iDehv.png`，发布记录的信息层级与状态配色参考；内容和数据行数不作为复制目标。

**Browser evidence**

- `artifacts/ops-deployments-2048.png`：2048×934 CSS/pixel，deviceScaleFactor 1，Deployment 页面，两个健康负载。
- `artifacts/ops-pods-2048.png`：2048×1102 CSS/pixel，deviceScaleFactor 1，两个展开的负载组。
- `artifacts/ops-artifacts-1480.png`、`artifacts/ops-artifacts-1000.png`：1480×940 和 1000×680，制品列表。
- `artifacts/ops-releases.png`、`artifacts/ops-release-plan.png`：1480×940，发布列表和待确认的计划。
- 部署/Pod 的源图与实际截图以同一工具输入打开比较，像素尺寸一致，不缩放归一化。发布列表以布局语言比较，源图和实现视口不同，不声称逐像素匹配。
- 顶部环境选择、表格表头、副本状态与 Pod 端口在 2048 像素原图中均可读；对这些区域逐项检查。发布弹窗另有独立状态截图，用于检查长镜像换行与确认按钮，无需从页面截图二次裁切。

**Findings and comparison history**

1. [P2, fixed] 旧数据库表格的全局 `td` 样式把新表格文字变为偏绿的 11px，并带来多余的竖线。增加 `.ops-console .ops-table` 作用域覆盖，恢复中性色的 12–13px 字体和横向细分隔线。重新捕获上述 Deployment、Pod 和 1000px 制品截图，确认已修复。
2. [P2, fixed] Deployment / Pod 页重复标题造成额外 65px 顶部空间，和参考图紧凑的筛选栏不一致。将可访问标题保留为视觉隐藏标题，把刷新放入筛选栏；表格从筛选栏下方直接开始。2048px 后续截图确认修复。
3. [P1, fixed] 发布弹窗标题继承旧暗色界面的浅色文字，在白底上不清晰。显式设置 `.ops-console .modal-head h2` 为 `#303034`，重新捕获 `ops-release-plan.png` 并打开，确认标题清晰。

**Required fidelity surfaces**

- 字体：沿用已安装的 Geist 与系统中文字体，标题 18px、导航 14px、表格 12–13px；代码和摘要使用等宽字体。长镜像在弹窗内换行，列表中截断并可查看完整标题。
- 间距：208–216px 侧栏，54px 顶栏，内容区 10px 外边距；保留参考图的白色主面板、紧凑筛选栏、展开分组和大量留白。1000px 窗口可操作主按钮；宽 Pod 表格在自身容器横向滚动。
- 配色：浅灰页面、白面板、细灰分隔线、蓝紫导航与主要操作、绿色健康状态。已隔离旧工作台暗色/绿色样式。
- 图像和图标：此界面沿用 DevOpsPad 文字品牌，使用现有 Lucide 图标。未复制参考产品的人员头像或品牌；本地版无人员账号体系，不使用虚构头像占位。
- 内容：菜单按用户要求实现制品、发布、Deployment、Pod、监控、日志、事件、配置与共享。演示模式标识明确；未添加无法执行的构建、终端或删除按钮。发布确认会显示完整目标和当前镜像；失败与成功有独立状态。

**Interactions verified**

制品摘要校验、登记、生成计划、演示发布、发布详情、刷新后持久化、环境与应用隔离、Pod 日志跳转、容器与行数选择、日志过滤、监控、事件、YAML 错误、配置冲突保留草稿、共享页演示限制、数据库草稿保留、桌面桥接确认前不执行以及失败结果显示。新页面测试监听了浏览器 pageerror，未发现未捕获异常。

**Accepted scope differences / follow-up**

- 新入口与已有数据库工作台共存；未复制参考站点的企业账号、私有部署、旧发布及其他未请求菜单。
- 数据来自本地示例或所选集群，负载数量、镜像、时间、操作者展示和参考产品不同。
- 本次为本地功能实现与风格对齐；不是原产品逐像素复制。历史监控图、镜像构建、日志流与 CRDT 同步属于后续功能。
- 原生 Wails 目录选择器和真实集群 rollout 的环境验收不由浏览器截图证明。

**Implementation checklist**

- [x] 作用域样式，避免旧工作台影响新界面。
- [x] Deployment / Pod 主布局与参考图对照。
- [x] 制品和发布的主要操作可执行。
- [x] 小窗口、空状态、错误、确认与完成状态覆盖。
- [x] 修复 P1/P2 后重新捕获并打开截图。

final result: passed

## 2026-10-23 — 原工作台风格与 Deployment 操作

用户要求继续实现功能并保持之前风格。本轮以仓库原有 Orca 工作台为配色、字体、按钮和边框依据，沿用应用服务的导航与资源表格结构。默认深色，右上角保留浅色切换。

已在浏览器打开并验证：制品列表 → Deployment 扩缩容（包括 0 副本）→ 重启 → 成功发布记录 → 回退计划 → 执行结果。切换环境、刷新页面后验证记录与副本状态隔离；模拟桌面桥接验证确认前无 apply 调用、失败结果可见。原数据库查询草稿在切换入口后保留。

视觉比对已打开：

- 原工作台 `artifacts/desktop-reference-1480.png` 与新制品库 `artifacts/ops-orca-artifacts.png`（1480×940）。
- 扩缩容确认弹窗 `artifacts/ops-orca-scale.png`（1480×940），标题、输入、提示及主操作均清晰。
- Deployment `artifacts/ops-orca-deployments.png`（1480×940），新增行内操作无溢出。
- Pod `artifacts/ops-orca-pods-1000.png`（1000×680），页面没有横向溢出，长资源表格在自身容器内滚动。
- 浅色截图继续由原应用服务测试捕获，主题选择重载后保留。

已处理：硬编码浅色改为带浅色默认值的语义变量；深色配色复用 Orca tokens，所有表格、表单、状态、提示和弹窗使用同一主题。操作记录区分发布、扩缩容、滚动重启与版本回退，副本操作显示前后数量。

新增功能不包含实时 CRDT、镜像构建/上传、Deployment 创建/删除或历史指标存储。真集群 rollout 仍需目标环境验收。

final result: passed

本轮验证：`go test -race -timeout 90s ./...`、`go vet ./...`、前端生产构建及 macOS arm64 桌面打包通过。完整浏览器回归 35 条一次通过，另 1 条受 Wails 运行时文件刷新触发的 Vite 重载影响；打包结束后该用例单独重跑通过，共 36 条完成验证。

## 2026-10-23 — K3s 接入与 AI 执行记录

沿用原 Orca 深色样式。新增入口“接入 K3s”和“AI 执行记录”，其余制品、发布和工作负载表格继续复用现有布局。

已打开并检查 1480×940 的 `ops-k3s-connect.png`、`ops-ai-runs.png`、`ops-ai-run-detail.png`，与原工作台及上一轮 Orca 页面比对。接入弹窗内部可滚动，底部操作始终可见；执行详情中的源码、会话、镜像、步骤和时间可读。修复了源码字段显示字面量换行转义的问题，状态标签改为贴合文字宽度，失败事件使用错误图标。1000×680 下接入按钮可见，页面没有横向溢出。

37 条浏览器用例全部通过，包括原工作台回归、真实桥接形状的 K3s 接入、环境/应用映射、外部记录刷新、构建到制品的关联。界面测试使用模拟桥接数据；真实 K3s 的只读读取和本地项目接入另由同一 Go 客户端路径验证，不将模拟构建声称为生产执行。真实集群未执行发布、扩缩容或重启。

JSON 事件有严格字段、生命周期、摘要和关联校验；Go 测试覆盖幂等重放、终态保护、失败构建拦截、日志路径约束、配置并发、Git 两副本同步以及文档示例。CLI 不替代执行端构建，也不会自动捕获未使用接入接口的操作。

final result: passed


## 2026-10-23 — Deployment 加载性能

Deployment 与 Pod 按需分别读取，三种负载并行查询。按应用查看时使用不嵌入对象的 Table 索引筛选候选，再读取完整对象并校验标签。保留元数据或模板标签匹配语义；不支持索引、列缺失、范围过宽及 list-only 权限会回退到完整列表。候选对象与索引共享 HTTP/TLS 连接。

页面内复用进行中的请求与已读取结果，15 秒内切回不再重复请求，过期结果先显示并后台刷新。手动刷新和操作完成后使旧结果失效；实际连接配置未变时，刷新本地项目不重复查询集群。初次等待有明确加载状态，离开集群页面不会把加载状态留在制品页。

heartie-pro/default 的只读实测：旧完整接口为 9.75、6.40 秒；仅分拆与并行后为 7.69、8.96、10.79 秒；最终索引路径（app=askuy）为 3.25、3.68、6.29 秒。网络有波动，这不是稳定延迟承诺，也没有用缓存时间冒充首次网络查询时间。完整 Deployment 响应约 103 KB，不含对象的索引约 4.5 KB。未执行集群写操作。

验证：38 条浏览器回归全部通过；Go 全量竞态测试、go vet、前端生产构建及 macOS arm64 打包通过。新增测试覆盖并发读取、跳过 Pod、请求复用、缓存过期、手动刷新、应用隔离、错误恢复，以及索引中未知选择器、元数据单独匹配、权限降级、对象删除/变化、分页提示和无效名称。已查看更新后的 ops-orca-deployments.png，保持原 Orca 风格，底部读取时间与数量显示正常。

final result: passed
