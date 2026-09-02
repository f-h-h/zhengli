# 前端优化方案（web/）

> 项目：家庭收纳位置管理系统
> 前端技术栈：Vue 3（Composition API）+ Vite 6 + three.js r170
> 评估日期：2026-09-02
> 范围：仅 `web/` 目录（不含 Go 后端）

本文档从**性能**与**审美**两个维度记录前端现状评估结论、优化方案与执行状态。
执行状态图例：✅ 已完成 / 🚧 进行中 / ⬜ 待实施

---

## 一、性能维度

### P1. three.js 全量打进主包，首屏 JS 646KB（✅ 已执行）

**现状**

- [Scene3D.vue](web/src/components/Scene3D.vue) 顶层静态 `import * as THREE`，three.js（r170 gzip 后约 160KB）几乎占满整个 bundle。
- `web/dist/assets/index-CBpPs0ob.js` 体积 **645.9KB**，其中 90%+ 来自 three.js。
- 用户打开页面无论是否使用 3D 视图，都必须下载并解析该体积。

**方案**

- 将 `Scene3D` 改为 `defineAsyncComponent(() => import(...))` 动态加载，把 three.js 拆分为独立 chunk。
- 主包降至仅包含 UI 框架代码（约 40-60KB），首屏渲染优先。

### P2. rAF 渲染循环常驻，无按需渲染（✅ 已执行）

**现状**

- `animate()` 每帧无条件执行 `orbit.update() + renderer.render()`。
- 桌面端 `Scene3D` 常驻（`App.vue` 非移动端分支），相机静止时仍满帧渲染，移动端额外耗电。

**方案**

- 引入 `renderDirty` 标记：`OrbitControls` 的 `change`、`TransformControls` 的 `objectChange`、重建/选中/闪烁等场景变更时置脏。
- rAF 循环保留（`orbit.update()` 支撑阻尼动画），但仅当 `renderDirty || flash` 时才调用 `renderer.render`。
- 借助 `enableDamping` 在阻尼过程中持续派发 `change`，动画结束后自动停止渲染。

### P3. 热路径反复 O(n) 线性查找（✅ 已执行）

**现状**

- `store.locations` 是普通数组；`depthOf`/`chainOf`/`selectedLoc` 以及 3D 拖拽的碰撞、吸附、支撑检测每帧调用 `store.locations.find(l => l.id === x)`。
- 拖拽热路径上 `find` 成 O(n)，且随容器数量增长。

**方案**

- `store.js` 维护 `locById: Map(id → location)`，`refreshIndex()` 在 locations 变更后重建。
- `depthOf`/`chainOf`/`selectedLoc`/`currentRoom` 及 `Scene3D` 热路径统一改用 `locById.get(id)` O(1) 查找。

### P4. 重建时 GPU 资源未释放（✅ 已执行）

**现状**

- `rebuild()`/`rebuildRoom()` 每次 `group.clear()` 旧网格，新建 Geometry/Material 未 `dispose()`。
- 改房间、改尺寸、缩放拖拽都会触发重建，长会话中 GPU 内存累积。

**方案**

- 新增 `disposeGroup()`：`traverse` 递归释放 geometry 与 material。
- 在 `rebuild()`、`rebuildRoom()` 清空 group 前调用。

### P5. 图片无压缩、列表加载原图（⬜ 待实施，需前后端配合）

**现状**

- 上传仅前端限制 10MB，未压缩；40px 缩略图与详情大图直接加载原图 URL。

**方案（建议）**

- 前端 canvas 压缩后上传（`DetailPanel.onImageFile`）。
- 或后端生成缩略图规格，列表使用缩略图 URL。

### P6. 数据请求：无缓存、无去抖、无取消（⬜ 待实施）

**现状**

- 切房间/增删改全量重拉；搜索需手动回车。

**方案（建议）**

- `searchItems` 加 300ms 防抖。
- 请求支持 `AbortController`，组件卸载后取消。
- 数据量较小时可加内存缓存。

---

## 二、审美维度

### A1. 顶部栏信息过载、emoji 混用（🚧 部分执行）

**现状**

- `RoomBar` 硬编码 emoji 🏠 + 4 按钮 + 2 个 ref 对话框挤一行；`ImportExport` 用原生 `<details>` 弹出无指示的菜单。
- 全项目 emoji 充当图标，各平台渲染不一。

**方案**

- ✅ 品牌区替换为内联 SVG 收纳箱标识（低风险）。
- ⬜ 低频操作（待购/回收站/导入导出）收进右侧分组或图标按钮（涉及布局重构，另行评估）。

### A2. 默认灰按钮过多，视觉焦点分散（⬜ 待实施）

**现状**

- 全局仅 `primary`（蓝）/`danger`（红）两种强调，其余全灰；DetailPanel 行内操作 8 个灰按钮并排。

**方案（建议）**

- 提炼语义色阶：次要操作用浅底描边、行内高频操作（用完/补货）图标化。

### A3. 详情面板过宽 + 信息层级弱（⬜ 待实施）

**现状**

- 桌面右栏固定 360px，物品表格 8 按钮 + 3 输入框，窄屏变形；类型/深度/备注/几何共用同一种弱灰。

**方案（建议）**

- 调整列宽策略、增强层级区分（例如几何区块弱化、操作区分组）。

### A4. 色彩风格割裂：功能蓝 + 高饱和类型色（⬜ 待实施）

**现状**

- 3D 场景 15 种类型色高饱和，与浅色背景 + 白地板同框偏"玩具感"；UI 单色灰蓝，两者风格割裂。

**方案（建议）**

- 类型色向低饱和莫兰迪方向收敛，贴近"家"的质感（需与后端色值约定同步）。

### A5. 移动端视图切换无过渡（✅ 已执行）

**现状**

- `App.vue` 移动端 搜索/树/3D/详情 硬切换，无动画；DetailPanel 无抽屉滑入感。

**方案**

- 用 `<Transition mode="out-in">` + 位移淡入淡出。

### A6. 反馈形式粗糙：confirm/prompt/toast（⬜ 待实施）

**现状**

- 8 处危险操作用浏览器原生 `confirm()`；库存多选用 `prompt()`；toast 固定 3200ms、无图标、无层级。

**方案（建议）**

- 自定义确认弹窗；toast 加 icon + 类型配色 + 更精致的出入场。

### A7. 悬停/按下动效缺失（⬜ 待实施）

**现状**

- 按钮仅 `background .15s`，无 hover 抬升/按下反馈；卡片无 active 态。

**方案（建议）**

- 统一 hover/active 动效规范，加微位移与阴影过渡。

### A8. 3D 场景氛围感弱（⬜ 待实施）

**现状**

- 背景纯色 `0xe9edf3`，网格线偏冷偏粗；无空态引导。

**方案（建议）**

- 背景加环境渐变/淡雾；空房间加引导文案或示意。

### A9. 无品牌、无 favicon、PWA 图标为空（✅ 已执行）

**现状**

- `index.html` 无 favicon；`manifest.webmanifest` 的 `icons` 为空；无品牌命名。

**方案**

- ✅ 新增 `web/public/favicon.svg` 并接入 `index.html`。
- ✅ manifest 补充 SVG 图标条目。
- ⬜ 完整 PWA（Service Worker + 离线缓存 + PNG 多尺寸图标）另行实施。

### A10. 引用不存在的 CSS 变量（✅ 已执行）

**现状**

- `MoveItemDialog.vue` 使用了 `--bg-hover`、`--accent-soft`、`--danger, #c0392b`，这些变量在 `style.css` 中未定义，hover/选中样式实际不生效。

**方案**

- 统一替换为 `style.css` 中已定义的 `--bg3`、`--accent-weak`、`--danger`。

---

## 三、执行记录

| 编号 | 优化项 | 维度 | 状态 |
|------|--------|------|------|
| P1 | Scene3D 动态 import 代码分割 | 性能 | ✅ |
| P2 | 3D 按需渲染（dirty 标记） | 性能 | ✅ |
| P3 | locations Map 索引 O(1) 查找 | 性能 | ✅ |
| P4 | rebuild 资源 dispose | 性能 | ✅ |
| A1 | 品牌区 SVG 标识 | 审美 | 🚧 部分 |
| A5 | 移动端视图过渡动画 | 审美 | ✅ |
| A9 | favicon + PWA 图标 | 审美 | ✅ |
| A10 | 修复不存在的 CSS 变量 | 审美 | ✅ |

### 待后续实施（需评审或前后端配合）

- P5 图片压缩 / 缩略图规格（需后端）。
- P6 请求去抖 / 取消 / 缓存。
- A1 顶部栏分组重构、A2 语义按钮色阶、A3 面板层级、A4 类型色收敛。
- A6 自定义 confirm/prompt + toast 增强、A7 动效规范、A8 3D 氛围。
- A9 完整 PWA（SW + 离线 + 多尺寸图标）。
