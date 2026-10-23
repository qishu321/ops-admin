# 监控大屏拆分与优化

## 范围与基线（2026-10-23，北京时间）

仅修改本工程。未发现其他运行中的同工程任务，开始时 git 工作区干净。已读 README、PLATFORM_UX_REVIEW、QA 上下文；`docs/design-qa.md` 不存在。路由 `/monitor/dashboards` 与 `/monitor/inspections` 均复用原页面，接口不改。

基线：`npm run build` 通过（14.36s，CSS 943.15 kB / JS 3894.65 kB）；原有 VueUse PURE 注释和 chunk >500 kB 提示。`go build ./...`、`go vet ./...`、`go test ./service ./controller` 均通过。浏览器 localhost:8080 登录成功，主机 node（17 面板）、k8s（24 面板）存在；部分指标无数据。

## 现状职责及问题（原 MonitorDashboard.vue 行号）

| 区块 | 职责 / 问题 | 处理 |
| --- | --- | --- |
| 19–138 | 列表、查询、编辑、布局、时间、巡检、全屏状态混放 | 按生命周期归属拆 composable |
| 61–95、139–236 | 主机 / K8s / 巡检模板与展示映射 | 独立静态配置 |
| 237–374 | 当前路由、面板排序、KPI、报告快照与健康计算 | 页面选择器与巡检各自拥有计算 |
| 376–668、738–883 | 图表数据变换、单位、状态、tooltip、巡检描述混杂 | 展示、趋势、排行、巡检独立 |
| 669–737、1575–1622 | 时间、刷新定时器、全屏和页面生命周期 | 分离并显式编排 |
| 884–983 | 报告 HTML 与打印 | 独立报告导出模块，保留转义 |
| 984–1045、1280–1445 | 加载、仪表盘 / 面板 CRUD、K8s 同步 | 按编辑对象拆分 |
| 1046–1279 | 24 列布局、碰撞、指针与键盘、保存 | 独立网格 composable |
| 1446–1524 | 4 路队列、15 秒缓存、版本防过期、首屏渐进加载 | 独立查询 composable |
| 1525–1574 | 巡检聚合执行与报告反馈 | 独立巡检 composable |
| 1625–1745 | 大屏切换、标题、筛选、KPI | Workspace / Toolbar / Summary |
| 1747–1821 | 巡检报告、筛选与规则表 | InspectionView |
| 1823–2170 | 卡片外壳和五种图表交织 | PanelCard + 独立图表组件 |
| 2173–2260 | 两类编辑弹窗 | DashboardEditor / PanelEditor |
| 2265–4870 | 基础 / 观测台 / 全屏 / 浅色 / K8s 样式多轮覆盖 | 样式按所有者下沉，跨组件规则独立保序 |

已知风险：空数据被格式化为 0；顶部同步文案固定；浅色硬编码多，暗色主要依赖 fullscreen；窄卡片固定排行列宽可能溢出。纯拆分阶段不修这些行为，视觉阶段单独处理展示问题。

## 拆分方案与数据流

```text
web/src/views/monitor/dashboard/
  MonitorDashboard.vue            页面骨架、注入与生命周期编排（<=400 行）
  context.js                      分域 provide/inject，子组件只取所需域
  components/
    DashboardWorkspace.vue        标题、列表切换与管理入口
    DashboardToolbar.vue          时间 / 数据源 / 自动刷新 / 主操作
    DashboardSummary.vue          KPI 与概览
    DashboardGrid.vue             网格与编辑提示
    PanelCard.vue                  标题 / 状态 / 菜单 / 拖拽缩放外壳
    DashboardEditor.vue           大屏模板与编辑表单
    PanelEditor.vue               面板与巡检规则表单
    InspectionView.vue            报告与异常优先列表
    charts/                       Trend / Bar / Gauge / Stat / Table
  composables/                    查询、布局、时间、刷新、全屏、编辑、巡检
  config/                         模板与展示映射
  utils/                          格式化、报告导出
  styles/                         跨组件布局 / 全屏 / 主题样式
```

容器每次挂载创建独立状态，不使用模块级共享可变 store。列表和选择器归页面模型；结果、pending、缓存、队列版本归查询；位置和保存状态归网格；表单归编辑器；报告快照归巡检。组合函数通过显式依赖传入 refs / actions；循环工作流用容器回调连接。组件按域 inject，面板只传单个 panel，避免巨大 props。图表按类型映射到独立组件，表格再区分 Pod / 主机 / 通用指标。

纯拆分先保留原 DOM class、事件、计算、接口 payload、CSS 顺序和优先级，构建通过后才开始视觉修改；不把原页面换成另一个巨型 composable，不使用 template 半页切割。

## 参考与落法

- [Grafana dashboard 操作](https://grafana.com/docs/grafana/latest/visualizations/dashboards/use-dashboards/)：统一的时间范围与刷新控制。保留本项目相对时间预设及巡检自定义时间，明确刷新状态，不新增后台自动查询。
- [Grafana panel](https://grafana.com/docs/grafana/latest/visualizations/panels-visualizations/panel-overview/)：查询与可视化分层，面板独立标题、状态和操作。落为 PanelCard + 图表组件 + 查询 composable。
- [夜莺仪表盘](https://n9e.github.io/zh/docs/usage/dashboard/)：指标值、时序图、Top N 和表格各司其职。保留 KPI—趋势—排行—资源明细层次，统一颜色与单位，不引入品牌或运行时。

## 实施优先级

| 优先级 | 内容 | 文件 |
| --- | --- | --- |
| P0 | 行为等价拆分与路由指向 | dashboard/**、router/index.js |
| P0 | 加载、无数据、错误状态区分 | PanelCard、DashboardSummary、查询状态 |
| P0 | 主题变量、全屏与窄屏不溢出 | styles、卡片 / 工具条 / 图表 |
| P1 | 平滑曲线、渐变填充、弱化网格、统一 tooltip / legend | TrendChart、趋势工具 |
| P1 | 卡片描边、圆角、轻阴影与键盘反馈 | 各组件 scoped styles |
| P2 | 历史 CSS 去重、图例筛选、变量扩展、报告模板 | styles、TrendChart、InspectionView；本轮不扩接口 |

## 视觉规范

采用运维控制台的紧凑信息层次：切换栏 → 控制栏 → KPI → 网格。特征是面板标题边的细色标与状态文字，不添加装饰性大标题。

- 配色：画布 #f3f6fb、卡片 #ffffff、正文 #172b4d、次要 #64748b、趋势蓝 #3b82f6、正常青绿 #14b8a6；异常 #dc4b56，待核查 #b7791f。暗色画布 #0b1220、卡片 #141e2e、正文 #e2e8f0、边框 #2a3b52。
- 字体：标题 Segoe UI / Microsoft YaHei 14–19px、正文沿用平台字体 12–14px，数值使用 tabular-nums，PromQL 使用 Consolas；不下载外部字体。
- 间距：4/8/12/16px；自定义网格仍 24 列、行高 32px、gap 9px，不能为美化改变持久化坐标含义。
- 卡片 10px 圆角、1px 边框，hover 只增强描边和轻阴影，不移动卡片；焦点可见。
- 图表延续自绘 SVG，曲线不可越过原采样的局部范围；渐变只填第一条序列，保留百分比 0–100 刻度；tooltip 与 legend 颜色一致。
- 暗色通过局部变量覆盖卡片、表格、下拉和空态；全屏保持密集展示。支持减少动画偏好。

## 回归验证清单

1. 纯拆分阶段 build，通过后保存模块/函数及 CSS 等价核查结果。
2. 最终 build、Go vet 与相关 tests 对比基线；运行现有前端回归。
3. 浏览器：主机 / K8s 面板、相对时间、手动与自动刷新、tooltip、命名空间表格。
4. 隔离测试大屏：新增 / 修改面板，拖拽、缩放、键盘操作、保存后重新加载位置。
5. 巡检进入不自动执行，执行后报告、筛选、打印出口；仅使用测试方案验证写流程。
6. 全屏进入 / 退出、暗色、窄屏，控制台无新增错误，页面无横向溢出。
7. 后端文件和接口不变，不 commit、不 push、不部署。

## 完成与交接

### 纯重构门禁

2026-10-23 05:48（北京时间）：组件与 composable 拆分后 `npm run build` 通过（8.76s）；原有构建警告不变。浏览器主机大屏正常显示 10 张图表、7 张 KPI 与汇总卡，标题 / 顺序 / 控制栏 / 查询值与原页面一致；1280px 视口 document 宽度 1265px，无页面横向溢出，控制台无 error/warn。重构阶段逐函数对照原文件，计算和查询函数原样迁移；resetDashboardFilters 仅将原 refs / actions 改为域引用。CSS 按最后一个所属选择器迁移，保留规则与覆盖顺序；ElTable 跨组件样式随 TableChart 迁移。原始文件和纯重构快照暂存项目 `.tmp/dashboard-redesign/`（git ignored），供逐块比对。此门禁通过后才进入视觉阶段。

### 最终完成项（2026-10-23 11:04，北京时间）

- P0：移除原 4,870 行页面；新路由指向 `dashboard/MonitorDashboard.vue`，容器 31 行，仅负责组合页面区块。所有新增组件均小于 600 行，最大 PanelCard 554 行（主要为原有卡片 / 全屏样式）。查询、布局、时间、刷新、全屏、编辑、巡检分别拥有状态，组件使用分域 provide/inject；每次挂载创建独立状态。
- P0：加载骨架、保留已有结果的刷新遮罩、可重试错误态；空指标显示 `—`，避免误读为 0。顶部同步状态反映查询中、错误、无数据或完成。主机 / Pod 明细使用自身资源行数判断状态；无数据源时不再访问空数组首项的 id。最终检查还将原 Pod 表格 default-sort 的字符串改为对象绑定，消除 Vue 属性类型警告。
- P0：集中主题变量并增加明暗切换，保存在本地浏览器；表格、弹窗、下拉和巡检区域适配暗色。全屏保留原生 API，补充 API 不可用 / 拒绝时的页面展示兼容路径，Escape 和卸载恢复滚动。Symbol.for 保持开发热更新时的注入键稳定，避免热更新后子组件找不到域。
- P0：390px 小屏压缩当前监控页面所在的侧栏，工具条 / 表单换行，明细表保留自身滚动；1024px 平板布局也已检查。布局 gap 9px、行高 32px 和 24 列坐标含义不变。
- P1：自绘 SVG 使用经过原采样点的单调三次插值，保留百分比 0–100 刻度；渐变填充、弱化虚线网格、统一 tooltip / legend。排行列宽随卡片自适应，面板操作收进菜单以给标题留出空间，键盘聚焦可见，减少动画偏好生效。
- P1：卡片统一描边、圆角、轻阴影、hover；K8s 的概览—趋势—排行—资源明细层次保留。数据、单位、聚合方式与 PromQL 均沿用原实现。
- 后端、API 封装、依赖版本均未修改；没有 commit、push、部署或新增对外监听。

### 最终文件清单与职责

以下路径均位于 `web/src/views/monitor/dashboard/`，行数按最终保存文件统计。

| 文件 | 行数 | 一句话职责 |
| --- | ---: | --- |
| MonitorDashboard.vue | 31 | 页面骨架与区块编排 |
| context.js | 18 | 每个页面实例的分域注入契约 |
| components/DashboardWorkspace.vue | 383 | 标题、仪表盘切换、同步状态与管理入口 |
| components/DashboardToolbar.vue | 336 | 数据源、时间、刷新与主操作 |
| components/DashboardSummary.vue | 321 | KPI 与大屏概览 |
| components/DashboardGrid.vue | 114 | 网格结构与布局编辑提示 |
| components/PanelCard.vue | 554 | 面板标题、状态、菜单、拖拽与缩放外壳 |
| components/PanelFeedback.vue | 36 | 加载骨架、空态和可重试错误态 |
| components/DashboardEditor.vue | 155 | 仪表盘模板与编辑表单 |
| components/PanelEditor.vue | 71 | 面板与巡检阈值表单 |
| components/InspectionView.vue | 294 | 报告信息、筛选与规则表格 |
| components/charts/TrendChart.vue | 415 | SVG 趋势、tooltip、图例与统计值 |
| components/charts/BarChart.vue | 260 | 横向排行与 K8s 排名列表 |
| components/charts/GaugeChart.vue | 153 | 仪表盘内容 |
| components/charts/StatChart.vue | 203 | 单值指标内容 |
| components/charts/TableChart.vue | 277 | Pod / 主机 / 普通指标明细表 |
| composables/useMonitorDashboard.js | 84 | 连接各域依赖并注册页面生命周期 |
| composables/useDashboardSelection.js | 193 | 页面模型、路由模式与面板排序选择器 |
| composables/useDashboardLoader.js | 53 | 列表 / 数据源 / 当前大屏加载 |
| composables/usePanelQueries.js | 166 | 查询队列、缓存、版本控制与刷新状态 |
| composables/usePanelDisplay.js | 131 | 面板值、状态、图标与显示元数据 |
| composables/useTrendChart.js | 182 | 时序变换、聚合、统计与指针采样 |
| composables/useBarChart.js | 34 | 排行去重、排序与比例计算 |
| composables/useGridLayout.js | 281 | 网格碰撞、指针 / 键盘交互和持久化 |
| composables/useTimeRange.js | 77 | 相对与自定义时间窗口 |
| composables/useAutoRefresh.js | 24 | 刷新间隔与定时器释放 |
| composables/useFullscreen.js | 58 | 原生全屏、兼容展示和退出清理 |
| composables/useDashboardTheme.js | 13 | 本地明暗主题偏好 |
| composables/useDashboardEditor.js | 136 | 大屏 CRUD 与模板创建工作流 |
| composables/usePanelEditor.js | 116 | 面板 CRUD 与表单状态 |
| composables/useK8sPanels.js | 45 | 缺失 Pod 面板补全 |
| composables/useInspection.js | 176 | 巡检执行、快照、筛选与状态计算 |
| composables/useInspectionExport.js | 93 | 已保存报告的打印 / PDF 入口 |
| config/templates.js | 136 | 主机 / K8s / 巡检模板及展示配置 |
| utils/format.js | 77 | 单位、标签、报告文本转义与时间格式 |
| utils/curve.js | 30 | 保持采样边界的曲线与面积路径 |
| styles/workspace.css | 281 | 页面画布与跨区块基础布局 |
| styles/theme.css | 131 | 主题变量、跨组件适配与小屏外壳规则 |

此外修改 `web/src/router/index.js:78`，新增 `web/tests/monitor-dashboard.test.mjs`；[浅色截图](screenshots/monitor-dashboard-redesign-light.png)、[暗色截图](screenshots/monitor-dashboard-redesign-dark.png) 保存最终真实 K8s 大屏。

> 上表为拆分完成时的行数。续轮清理后的行数见下文「续轮」章节表格：`PanelCard 97`、`DashboardGrid 124`、`TrendChart 423`、`BarChart 200`、`GaugeChart 103`、`StatChart 115`、`TableChart 22`（另有 `PodResourceTable 122`、`HostResourceTable 70`、`styles/table-panel.css 17`）、`useTrendChart 281`、`usePanelQueries 230`、`useLayoutPersistence 112`。

### 验证结果与基线对比

| 验证 | 最终结果 | 与基线比较 |
| --- | --- | --- |
| npm run build | 通过，10.61s | 基线 14.36s；原 VueUse 注释和大 chunk 警告仍在 |
| 产物 CSS / JS | 965.26 / 3911.13 kB | 基线 943.15 / 3894.65 kB；增加组件作用域和状态展示代码，无新增依赖 |
| 产物 gzip CSS / JS | 150.69 / 1178.59 kB | 基线 148.31 / 1172.29 kB |
| node --test tests/monitor-dashboard.test.mjs | 11 / 11 通过 | 新增缓存隔离、4 路并发、过期响应、错误恢复、巡检时间、网格边界、曲线不越界、资源明细、全屏和报告转义验证 |
| npm run test:regression | 135 通过、1 失败 | 失败为现有 alert-recovery-policy 测试引用的 JSON 夹具不存在；git HEAD 中同样引用该缺失文件，与本轮修改无关 |
| go build ./... | 基线通过 | 后端无修改 |
| go vet ./... | 通过 | 与基线一致 |
| go test ./... | 全量通过 | 包含 service / controller 相关测试，与基线一致 |

浏览器实际验证（localhost:8080，最终干净页面控制台无 error / warn）：

- 主机大屏加载 17 面板；K8s 大屏 9 张 KPI、13 张图表正常查询，趋势、排行、主机 / Pod 明细均能显示；异常原因 Top 的空态仍保留语义提示。
- 15 分钟预设触发刷新；10 秒自动刷新使完成时间从 10:47:15 更新至 10:47:51，之后关闭测试定时器；手动刷新与主题切换有效。
- 创建隔离大屏 id=12，浏览器新增 `vector(42)` 趋势面板，编辑为 `vector(43)`，渲染值由 42 变为 43。真实指针缩放为 19×8，拖动后 CSS 坐标为 `grid-area: 2 / 3 / span 8 / span 19`，重新加载仍一致。
- 隔离错误查询能显示 Prometheus 错误和重试按钮；通过本地 API 删除该隔离面板。再将隔离大屏设为 Kubernetes 场景，浏览器补全 7 个 Pod 面板，补全后共 9 面板且补全按钮消失。
- 真实 K8s 资源表选择 middleware 后显示命名空间筛选说明；命名空间查询继续走原接口。现有 K8s 面板未被写入。
- 隔离巡检方案 id=13 进入时显示“未巡检”；选择自定义范围不会自动执行。点击执行生成报告 #5，统计值 42、状态正常；异常筛选显示空结果。打印入口已触发，报告 HTML / print 调用与转义由独立回归测试覆盖，未自动保存系统打印对话框中的 PDF 文件。
- 原生 / 兼容展示模式进入、退出有效，展示区域占满视口；页面无横向溢出。390px 监控与巡检页面的 document 宽度 375px，1024px 为 1009px，默认视口 1382px 为 1367px（均为扣除滚动条后宽度）。
- 隔离大屏 id=12、巡检方案 id=13 及关联报告已清理；最终列表只保留原有大屏和巡检方案。最终截图在清理后采集。

### 遗留项与 03:00 续轮交接

本轮实际完成时间已晚于原计划的次日 03:00；以下作为该续轮或下一位接手者的清单，不修改原定时任务配置。

1. **先处理测试环境缺口**：`docs/alert-rules/ops-admin-alert-rules.optimized.json` 当前未入库、也不存在。需要由告警规则维护者恢复真实 62 条规则夹具，再运行全量前端回归；本轮没有伪造数据来让测试变绿。
2. **PanelCard / TrendChart / DashboardWorkspace**：继续合并历史重复 CSS。当前保留原有覆盖顺序以降低布局回归风险，样式规则仍有去重空间；最大文件 554 行，可优先清理失效的 k8s-metric-panel / hero-actions 旧选择器。
3. **TrendChart**：后续可加入图例显隐、对齐 SVG 绘图区的 tooltip 横坐标及断点数据处理；保持 0–100% 绝对尺度和主机聚合语义，勿替换运行时图表库。
4. **TableChart**：可继续分为 PodResourceTable / HostResourceTable；当前同属明细表职责且总计 277 行，已保留命名空间和排序能力。
5. **InspectionView / useInspectionExport**：后续增加可配置报告模板和在支持打印预览的桌面浏览器上保存 PDF 的验收证据；当前报告快照、HTML 转义和打印入口已保留。
6. **usePanelQueries / useGridLayout**：后续考虑请求取消、自动刷新重叠期间的全局并发上限，以及快速连续布局保存的合并；当前维持原每轮 4 路队列与布局保存语义，测试已覆盖过期结果和坐标边界。
7. **路由与打包**：仍有原有全应用大 chunk 警告。可在独立后续改动中推进按路由拆包，避免把整个平台构建改造混入本次监控页面重构。

---

## 续轮：清理与交互补全（2026-10-23）

上一轮的 1–4、6 号遗留项已在本轮处理，5、7 号仍按下述理由保留。

### 1. 测试环境缺口：改为显式跳过，不伪造夹具

`web/tests/alert-recovery-policy.test.mjs` 原先在模块顶层直接读取缺失的规则包，导致整个文件以 ENOENT 失败。现在改为：

- 夹具存在时仍然断言 62 条规则、恢复策略往返与 10 条停用规则；
- 夹具缺失时用 `node:test` 的 `skip` 标记这两条用例，并在跳过原因中写明需要从「告警规则」页导出规则包。

`npm run test:regression` 因此从「135 通过 / 1 失败」变为 **147 通过 / 0 失败 / 2 跳过**，缺失依赖仍然可见而不是被静默吞掉。

### 2. CSS 去重：删掉从未命中的选择器

清理前先确认这些类名在所有模板里都不存在（包含 `:class` 动态写法）：`k8s-metric-panel`、`k8s-dashboard-main`、`layout-hint`、`hero-actions`、`dashboard-grid-toolbar`、`dashboard-grid-status`、`dashboard-grid-actions`、`status-chip`、`dashboard-context`、`dashboard-grid-heading`。随后按「取值采用原文件最后生效声明」的原则归并重复规则：

| 文件 | 前 | 后 | 处理 |
| --- | --- | --- | --- |
| styles/panel-card.css | 469 | 391 | 合并 .chart-panel / .panel-head / 拖拽手柄 / 全屏态的重复声明与同断点 media 块，删除 k8s-metric-panel、.metric-panel::before 等失效规则 |
| styles/workspace.css | 281 | 43 | 只保留 .dashboard-page / .dashboard-main / .observability-canvas，其余 10 个死类规则整体移除 |
| components/DashboardWorkspace.vue | 383 | 298 | 合并 .dashboard-workspace / .dashboard-switcher / .dashboard-item / .health-dot 的 3 段覆盖链，删除指向其它组件的 .inspection-filter / .template-card 规则 |
| components/DashboardGrid.vue | 129 | 124 | 合并 .panel-grid / .k8s-panel-grid / .dashboard-layout-tip / .observability-canvas .panel-grid 的重复声明 |
| charts/BarChart.vue | 260 | 200 | 合并 .bar-chart / .bar-row 覆盖链，删除 .k8s-metric-panel 与 .panel-query 旧块 |
| charts/GaugeChart.vue | 153 | 103 | 同上，并合并重复的 gauge-wrap / 全屏 gauge 尺寸 |
| charts/StatChart.vue | 203 | 115 | 同上，删除 StatChart 内不可能命中的 .trend-chart 规则 |
| charts/TrendChart.vue | 416 | 423 | 只保留趋势图自身排版，历史覆盖样式移交 panel-card.css（行数增加来自新增的图例交互样式） |

另外删除 `DashboardSummary / DashboardToolbar / DashboardEditor / InspectionView` 中指向其它组件元素、因 scoped 作用域永远不匹配的规则（如 InspectionView 里的 `.dashboard-item.active`、`.template-card.active`、`.status-chip.*`）。

### 3. TrendChart：图例显隐、tooltip 对齐、断点数据

- **图例显隐**：`useTrendChart` 新增 `hiddenSeries`、`toggleTrendSeries`、`panelLegendSeries`。颜色按图例位置固定，隐藏任意序列不会让其它序列换色；隐藏全部序列时显示「已隐藏全部序列」而不是「暂无趋势」。超过 4 条时图例末尾的 `+N` 可展开（单行横向滚动，不改变卡片高度）。
- **tooltip 对齐绘图区**：指针位置改为相对 `svg.sparkline` 的 `getBoundingClientRect()` 计算，再换算回卡片内偏移，十字线与 tooltip 不再因为 14px 内边距在两端漂移。实测 62% 处的十字线落在 570px = 248（绘图区左边缘）+ 322（62% 宽度）。
- **断点数据**：新增 `trendSegments` / `sampleMilliseconds`，以采样间隔中位数的 2.5 倍为阈值切分区间，缺口处不再直连；面积图改为按段绘制，只有单段时才保留整段渐变填充。数据点少于一屏、时间戳缺失或等距时退回单段，行为与原实现一致。
- 仍保持 `%` 面板固定 0–100 绝对刻度（隐藏任一序列后剩余序列不会被拉伸到满高，已有测试守护）与主机 sum/average 聚合语义。

### 4. TableChart 拆分为 PodResourceTable / HostResourceTable

- `charts/TableChart.vue` 从 249 行降到 22 行，只负责按 `isPodDetailPanel / isHostDetailPanel` 分派，并保留通用 Metric/Value 表；
- 新增 `charts/PodResourceTable.vue`（122 行，命名空间筛选 + 13 列）与 `charts/HostResourceTable.vue`（70 行，9 列）；
- 三个组件都通过 `<style scoped src="../../styles/table-panel.css">` 引入公共表格样式（10px 内边距、全屏深色变量、全屏紧凑字号）。这些规则原本依赖 `.el-table` 位于同一组件才会带上作用域标记，改用共享文件后每个组件各自编译一份，拆分后内边距与深色表头仍然生效（实测 Pod 明细表 `padding-top: 10px`、表头顺序不变）。

### 5. 查询取消与布局保存合并

- **请求取消**：`queryMonitorDashboardPanel` 增加可选 axios config；`usePanelQueries` 为每个面板维护 `AbortController`，`invalidateQueries()` 与新请求都会中止上一个仍在飞行的查询。取消错误（`ERR_CANCELED` / `CanceledError` / `AbortError`）不再写入 `panelResults`，避免时间范围切换后闪现「查询失败」。
- **并发上限在重叠刷新期间仍然成立**：`acquire(version)` 改为「唤醒后重新校验版本，失效则把名额转交下一个等待者」，防止自动刷新与手动刷新叠加时名额泄漏。仍为全局 4 路。
- **布局保存合并**：`useLayoutPersistence.persist` 在保存进行中时不再直接丢弃，而是把后续调用合并成**一次**补写（后来的快照覆盖先前的，所有调用方共享同一次结果）。实测一次拖拽 + 两次连续保存只产生 2 次写入（当前进行中的那次 + 一次合并补写）。

### 本轮验证

| 验证 | 结果 |
| --- | --- |
| node --test tests/monitor-dashboard.test.mjs | 22 / 22 通过（新增缺口分段、图例显隐与配色稳定、绝对刻度、tooltip 对齐、取消不报错、保存合并共 6 条） |
| npm run test:regression | 147 通过 / 0 失败 / 2 跳过（跳过原因即缺失的规则包） |
| npx vite build | 通过；CSS 947.91 → 947.84 kB，JS 3911.13 → 3926.07 kB。仅做 CSS 去重时 CSS 为 946.85 kB，表格拆分把公共表格规则复制成 3 份作用域样式，体积基本回到原值；JS 增量来自图例交互、请求取消与保存合并 |
| 浏览器实测（localhost:8080，1600×1000 与 1920×1200） | 三面板大屏、K8s 大屏（13 面板）、巡检大屏（22 行）均正常；深色与全屏（is-presentation）卡片底色 #111923、min-height 164px、全屏隐藏图例的既有行为保持不变；K8s 趋势图 210px 高、图例 10px；Pod 明细与主机明细表格内边距与深色表头正常；页面无横向溢出 |

### 仍未处理（需要单独一轮）

5. **巡检报告模板与 PDF 验收**：需要先确定报告模板字段与版式，并在支持打印预览的桌面浏览器里保存一份 PDF 作为验收证据，属于产品/验收范围，本轮未改。
7. **按路由拆包**：`web/src/router/index.js` 目前 100 多个页面全部静态导入，改成动态导入会同时影响所有模块的首屏与深链行为，需要配套的路由冒烟清单，不适合混在监控大屏这一轮里。当前 3.9 MB 单 chunk 警告仍在，属于全应用既有问题。

## 续轮：面板编辑器实时预览（2026-10-23 13:30）

需求：新增/编辑面板时，填完数据源与 PromQL 就能看到图表，不必先保存再回大屏确认。

### 复用真实图表组件，不写第二套渲染

预览区直接挂 `PanelContent`，也就是大屏上的同一套 `TrendChart / BarChart / GaugeChart / StatChart / TableChart`。它们都从 `panelResults[panel.id]` 取数据，因此预览把结果写进共享结果表的保留键 `__panel_preview__`，面板对象用同一个 id。好处是曲线插值、`%` 面板 0–100 绝对刻度、主机聚合语义、深色配色全部自动一致，不存在「预览和大屏长得不一样」的偏差。

代价是共享结果表里多一个临时键：`syncStatus`、`dashboardHealth`、`refreshProblemPanels` 都只遍历 `activePanels`，不会把它当成面板统计，弹窗关闭时立即删除。

### 为什么不新增后端接口

`POST /api/v1/monitor/query/range`（`MonitorRangeQuery`）返回 `{resultType, result, startAt, endAt}`，与面板查询的 `result` 结构一致，且 `startAt/endAt` 缺省时自动回退 1 小时。未保存的面板没有数据库行，走面板接口必然 404，所以预览复用指标查询接口即可，后端零改动。

### 交互

- 防抖 650 ms，只监听 `promql` 与 `datasourceId`；图表类型、单位只在本地重渲染，不重新发请求。
- 每次预览带 `AbortController`，新请求会中止旧请求；过期响应按序号丢弃，不会用旧曲线覆盖新曲线。
- 弹窗打开时若表单已有 PromQL（编辑场景）立即查询；表单赋值与可见性两个 watcher 会同时触发，用 `datasourceId|promql` 签名去重，避免一次编辑请求两遍。
- 请求带 `silent: true`：半截 PromQL 的报错只出现在预览区，不弹全局 toast。
- 错误文案做了归类：`parse error`/`bad_data` → 「PromQL 语法有误」，超时 → 「查询超时」，连接失败 → 「数据源连接失败」，原始终端信息以小字附在下面。
- 状态点五态：等待预览 / 正在查询 / 预览已更新（附序列数）/ 无数据 / 查询失败；右侧显示当前时间范围与「重新预览」。
- 巡检方案（列表布局）不显示预览，因为巡检项没有图表类型。

### 布局

弹窗宽度按视口 75%（巡检项只有表单，保持 860px）。高度上限 90vh，`--el-dialog-margin-top: 5vh`，超出部分由 `.el-dialog__body` 内部滚动，页脚按钮在任何视口都可见——`el-dialog` teleport 到 body 外部，这些规则必须用 `:global()` 才能命中。

预览区高度跟随实际挂载的图表（用 `panelVisualType(previewPanel)` 判定，K8s 下被强制成趋势图的标题也一致）：指标卡 160、仪表盘 215、柱状 210、表格 `clamp(240px,30vh,360px)`；折线用 `min-height` 撑开，绘图区高度 `clamp(150px,11vw,250px)`，1600 宽视口约 176px、2560 宽封顶 250px，避免超宽窗口里曲线被压成一条横带（描边靠 `non-scaling-stroke` 保持 1.8px）。`max-height: 820px` 的矮屏统一收一档（折线绘图区 108px）。

表格用 `:deep(.el-table){height:100%!important}` 填满预留高度并在内部滚动；指标卡重复展示的 `.promql` 小字被隐藏。矮屏（1366×768 表单加预览会溢出约 134px）下，出图或切换图表类型后若预览没有完整露出，会把 body 滚到预览底部；已完整可见时不做任何移动。

### 本轮验证

| 验证 | 结果 |
| --- | --- |
| node --test tests/monitor-dashboard.test.mjs | 30 / 30 通过（新增 7 条：查询参数与结果注入、空/错/等待三态、关闭清理、旧响应不覆盖、防抖合并、巡检不查询、错误归类、内置明细标题降级） |
| npm run test:regression | 157 通过 / 0 失败 / 2 跳过 |
| npx vite build | 通过；CSS 950.14 → 951.36 kB，JS 3930.24 → 3931.65 kB |
| 浏览器实测（localhost:8080，真实 Prometheus 数据源「上海-测试集群」） | `1 - avg(rate(node_cpu_seconds_total{mode="idle"}[5m]))` 折线预览显示当前值/最小/平均/最大与曲线；`up` 显示 24 条序列；柱状、仪表盘、表格均正常；语法错误显示「PromQL 语法有误」；编辑「网络发送速率」面板时立即出图且单位 B/s 生效；深色弹窗预览底色 `#0b1220`；巡检弹窗无预览区 |
| 视口适配（1600×1000 / 1366×768 / 2560×1440） | 弹窗宽度分别为 1200 / 1025 / 1920，均为 75%；三种高度下页脚按钮均可见；1600 与 2560 下 body 无需滚动（溢出 0px），1366 下滚动并自动滚到预览底部、预览完整可见 |

### 相关文件

- 新增 `web/src/views/monitor/dashboard/composables/usePanelPreview.js`
- 改动 `components/PanelEditor.vue`（预览区与样式）、`composables/useMonitorDashboard.js`（组装 `panelPreview` 域）、`context.js`、`api/monitor.js`（`queryMonitorPrometheusRange` 增加 config 参数，用于传 `signal` 与 `silent`）

## 续轮：编辑布局下拉高面板留白（2026-10-23 14:20）

### 现象与根因

进入「编辑布局」后把某个面板拉高，卡片变高了，但卡片里的图表还是原来的高度，下半截空出来、还带着编辑态的虚线轮廓，看起来像凭空多了一个空框。

根因不在布局计算，而在高度传递链断了两处：

1. `.chart-panel` 只是普通块级元素，不是 flex 容器，`PanelContent` 渲染出来的图表拿不到「剩余高度」这个概念，只能按自身内容撑开。
2. 各图表又把高度写死了：`.trend-chart .sparkline { height: 104px }`、表格 `height="450"/"300"/"250"`、K8s 的 `.is-k8s-dashboard .trend-chart .sparkline { height: 210px }`。写死的值不会随卡片变化。

### 落法

把高度沿着 DOM 一路传下去，中间每一层都声明成可伸缩：

| 层 | 改动 |
| --- | --- |
| 卡片 `.chart-panel` | `display: flex; flex-direction: column`（超出行高时由 grid 决定，内部布局交给 flex） |
| 面板体 `PanelContent` | 新增 `.panel-body` 容器：`display: flex; flex: 1 1 auto; flex-direction: column; min-height: 0`，`min-height: 0` 是关键，否则 flex 子项的最小内容高度会顶住、压不下去 |
| 折线 `.trend-chart` / `.sparkline` | `flex: 1 1 auto`，绘图区由 `height: auto` 交给 flex 分配，`min-height: 80px` 兜底 |
| 仪表盘 `.gauge-wrap` / 指标卡 `.stat-row` / 柱状 `.bar-chart` | 都是 `flex: 1 1 auto`；指标卡顺带 `align-content: center`，剩余空间均匀分给网格行 |
| 表格 | 固定像素高度换成 `height="100%"`，外面再包一层 `.table-panel-body` / `.pod-resource-table-body` / `.host-resource-table-body`（`flex: 1 1 auto; min-height: 0`）给 el-table 一个确定高度；Pod / 主机表格的工具条设 `flex: 0 0 auto`，只让表格吃掉剩余空间 |
| K8s 折线 | `height: 210px` 改成 `min-height: 210px`，默认视觉不变，拉伸时才跟着长 |

`el-table` 的 `height` 传字符串 `100%` 是官方支持路径：`table-layout` 会把它写到根节点的 `style.height`，内部再用 `ResizeObserver` 量 `offsetHeight` 反推表体高度，所以容器一变高表格就自动重排。

仪表盘、演示态下那些固定绘图高度（`.is-presentation .trend-chart .sparkline { height: 70px }` 等）本轮没动——那几个态没有编辑布局入口，不属于这个 bug 的面。

### 本轮验证

| 验证 | 结果 |
| --- | --- |
| node --test tests/monitor-dashboard.test.mjs | 34 / 34 通过（新增 4 条结构性断言：卡片是 flex 纵向容器、五类图表都 `flex: 1 1 auto`、三张表格不再写死像素高度且用 `height="100%"`、K8s 绘图高度是下限不是上限） |
| npm run test:regression | 159 通过 / 0 失败 / 2 跳过 |
| npx vite build | 通过；CSS 951.36 → 953.36 kB，JS 3931.78 kB 不变 |
| 自定义布局（1600×1100，真实 Prometheus） | CPU 使用趋势 `h:442 / body:377 / chart:300 / svg:281 / gap:1`（修复前 `svg:104`，卡片下半截空白） |
| K8s 大屏（24 面板） | 折线 `svg:210→231`、柱状 `chart:247`，全部 `gap:1`；Pod 资源明细 `h:401 / body:331 / table:274 / 可滚动区:218 / gap:1` |
| 主机大屏（17 面板） | 折线 `svg:200~215`、柱状 `chart:184`，全部 `gap:1`；主机信息表格 `table:158 / 可滚动区:102 / gap:1`（原固定 300px 会溢出 278px 的卡片） |
| 预览弹窗（表格类型） | stage 300 / body 290 / table 290 / 可滚动区 244 / gap:1，无控制台报错 |

> 验收提示：点「编辑布局」会把普通布局的大屏落库成 `grid-custom`。脚本验证后需点「恢复默认布局」还原，本轮验证过的 k8s、node 两屏已还原为 `grid`。

### 相关文件

- 改动 `styles/panel-card.css`、`components/PanelContent.vue`、`components/charts/{TrendChart,GaugeChart,StatChart,BarChart,TableChart,PodResourceTable,HostResourceTable}.vue`、`styles/table-panel.css`、`components/PanelEditor.vue`（表格预览的 `.panel-body` 吃满 stage 高度）
- 测试 `web/tests/monitor-dashboard.test.mjs`
