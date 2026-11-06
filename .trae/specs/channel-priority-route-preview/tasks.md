# 渠道优先级调整的路由变化预览确认 - 实施计划

## Task 1: 后端路由分档预览模型与计算
- **Status**: `pending`
- **Priority**: high
- **Depends On**: None
- **Description**:
  - 在 `model/channel_priority.go` 中：
    - `ChannelPriorityMove` 增加请求字段 `Routes bool `json:"routes"``。
    - 新增类型 `ChannelPriorityRouteTier{Priority *int64; Channels []int}`、`ChannelPriorityRouteChange{Group, Model string; Before, After []ChannelPriorityRouteTier}`。
    - `ChannelPriorityOrder` 增加 `Routes []ChannelPriorityRouteChange `json:"routes,omitempty"`` 与 `ChannelNames map[int]string `json:"channel_names,omitempty"``。
    - 在 `ReorderChannelPriority` 事务内、`planChannelPriorityMove` 得到 changes 之后，仅当 `move.Preview && move.Routes` 时，用同一 `tx`：
      1. 单次查询启用路由行（`abilities` 中 `enabled = true` 的 `group, model, channel_id, priority`，跨三库兼容的 GORM 写法，`group` 列经既有保留字处理方式或直接结构体字段映射）；
      2. 以当前 priority 构建 before 分档，以 changes 覆盖后构建 after 分档（nil 按有效值 0 归档）；
      3. 逐（group, model）比较有序档成员序列（档数不同或任一档排序后成员集合不同即变化），只收集变化组合，档位 priority 降序、档内 id 升序；
      4. 查询这些组合中出现的全部渠道 id 的 `name`（`Select("id","name")`，单次查询），填充 `ChannelNames`。
    - preview 不写库、不加锁的既有行为不变；非 preview 或未请求 routes 时响应不含新字段（保证现有 `assert.Equal(preview, applied)` 类断言语义不破坏）。
  - 控制器 `controller/channel_priority.go` 无需结构改动（字段随模型透传），确认错误映射不变。
- **Acceptance Criteria Addressed**: AC-2, AC-3, AC-8, NFR-2
- **Test Requirements**:
  - `rule` TR-1.1: 新增纯分档判定的表驱动测试（跨档变化列入；纯空隙重编号且成员/顺序不变不列入；禁用渠道不参与；档合并/分裂、首选/备用互换列入；档内 id 升序、档位 priority 降序；NULL priority 与 0 同档），证据：`go test ./model/ -run ChannelPriority` 输出。
  - `rule` TR-1.2: 在 `TestChannelPriorityDatabaseMatrix` 增加 routes 预览断言（多分组多模型，只受影响组合返回；`channel_names` 含页外渠道；preview 不持久化；SQLite 必跑，MySQL/PG 设置 DSN 时跑），证据：三库矩阵测试输出与版本日志。
  - `rule` TR-1.3: 未携带 routes 的 preview/commit 响应不含 routes/channel_names 字段且既有 matrix 断言保持通过，证据：同一测试运行。
- **Notes**: 不使用方言 SQL；布尔参数化；不改 schema/迁移；JSON 序列化标签仅作类型引用，响应仍由控制器 `c.JSON` 输出，与现状一致。

## Task 2: 前端 API 类型与优先级 Hook 确认流程
- **Status**: `pending`
- **Priority**: high
- **Depends On**: Task 1
- **Description**:
  - `web/src/features/channels/lib/channel-priority-order.ts`：为 `PriorityOrder` 增加可选 `routes?: PriorityRouteChange[]`、`channelNames?: Record<string, string>`；新增 `PriorityRouteTier`/`PriorityRouteChange` 类型；`PriorityMove` 增加可选 `routes?: boolean`。
  - `web/src/features/channels/hooks/use-channel-priority-order.ts`：
    - 将松手/键盘后的 `commit()` 拆为"请求确认"与"确认保存"：
      - `requestConfirm()`：取手势快照 revision，调用 `reorderChannelPriority({...drop, revision, preview:true, routes:true})`；成功后设置 `pending = { drop, order }` 打开弹窗；失败经 `handleServerError` 提示并 `cancel()`（不保存）。
      - `confirmSave()`：用手势开始时快照 revision 发非 preview 提交（复用现有 `save()`，成功后失效列表查询、toast、设置 undo、关闭弹窗）；提交失败保留/刷新数据并提示原因。
      - `decline()`：清除 pending 并执行现有 `cancel()`，无写请求。
    - 暴露 `pending`、`preparing`（路由预览请求中）、`requestConfirm`、`confirmSave`、`decline`；`saving`/`undo`/`restore` 保持。
  - 拖拽接线：`use-channel-priority-drag.ts` 的 `commit` 回调改接 `requestConfirm`（指针落点动画完成后与键盘路径一致）；`channels-table.tsx` 传入新回调并渲染确认弹窗；悬停预览（不带 routes）与状态条逻辑保持不变。
  - 空结果处理：`pending.order.routes` 为空（或 changes 为空）时弹窗只显示空状态与"关闭"，不调用保存。
- **Acceptance Criteria Addressed**: AC-1, AC-7, AC-8, NFR-1
- **Test Requirements**:
  - `rule` TR-2.1: 松手/方向键后首个 reorder 请求携带 `preview:true` 与 `routes:true`，确认前/取消时均无非 preview 写请求；确认后写请求体 revision 等于手势开始时 GET 快照 revision，证据：hook/组件测试请求断言。
  - `rule` TR-2.2: 预览失败与提交冲突分别 toast 服务端原因（冲突含现有英文键文案），均无写入、列表查询刷新；取消后 undo 不产生、列表保持原序，证据：组件测试。
  - `rule` TR-2.3: `bun run check` 对改动 TS/TSX 零类型错误，证据：命令输出。

## Task 3: 路由变化确认弹窗组件
- **Status**: `pending`
- **Priority**: high
- **Depends On**: Task 2
- **Description**:
  - 新增 `web/src/features/channels/components/channel-priority-confirm-dialog.tsx`：
    - 复用 `@/components/dialog`（带滚动 body，宽幅 `sm:max-w-3xl` 左右）、`Button`、`Badge`、`GroupBadge`（`label` 传掩码）、`EmptyState`/`Spinner`。
    - props：`order: ReturnType<typeof useChannelPriorityOrder>`（消费 `pending`/`saving`/`confirmSave`/`decline`）；从 `useChannels()` 读 `sensitiveVisible`。
    - 内容：
      - 顶部汇总受影响（分组, 模型）组合数量；加载中显示 Spinner 与等待文案。
      - 按 group 分区（`GroupBadge`，隐藏时 `label='••••'`）；每组下按 model 分行（模型名等宽展示，不掩码）；每行 before/after 两列：逐档渲染，rank 0 标签"首选"，rank≥1 标签"备用 {{n}}"，同档渠道在一个成组容器内以徽标展示（渠道名取 `order.channelNames[id]`，缺失回退本地列表再回退 `#id`；隐藏时显示 `••••` 且不显示 id）。
      - 高亮 before/after 中 rank 发生变化的渠道徽标（如 warning/primary 描边）。
      - `routes` 为空时显示 `EmptyState`（图标 + "无路由变化"标题与说明），footer 仅"关闭"。
      - 有变化时 footer："取消"与"保存变更"（保存中 loading/disabled）；ESC/遮镜/取消→`decline`。
    - 挂载于 `channels-table.tsx` 拖拽区域附近（不依赖 portal 容器，Dialog 自带 portal）。
  - 移除/替换教学与状态文案：`channel-priority-guide.tsx` 底部 "Changes save on drop. You can undo the last move." 改为新英文键（如 "Review routing changes and confirm to save. You can undo the last move."），并从 7 个 locale 删除旧键、加入新键；其余拖拽教学内容不变。
- **Acceptance Criteria Addressed**: AC-4, AC-5, AC-6, AC-9, AC-10
- **Test Requirements**:
  - `rule` TR-3.1: 弹窗按分组→模型→before/after 档渲染；rank 0/备用标签正确；同档渠道成组；跨档渠道徽标带高亮标识；页外渠道名取自 `channelNames`，证据：新增/扩展的弹窗组件测试。
  - `rule` TR-3.2: 空 routes 时仅有关闭动作且无保存请求，证据：组件测试。
  - `rule` TR-3.3: 隐藏态下分组名与渠道名均为 `••••`、真实名称与 id 不出现、模型名可见，证据：组件测试。
  - `rule` TR-4.1（键盘/可访问性，本任务实现）：弹窗可 Tab 到取消/保存、ESC 关闭且不保存；方向键触发路径与松手一致，证据：组件测试。
  - `rubric` TR-3.4: 信息架构质量（AC-9 同维），1-5，锚点同 AC-9，阈值 >=4，证据：浏览器实际走查。
- **Notes**: 不新造基础控件；不引入依赖；测试文件放入该组件/功能既有 `__tests__/` 目录，优先扩展 `priority-order.test.tsx`，仅在确有必要时新增一个弹窗专属测试文件。

## Task 4: i18n 文案补全（7 个 locale）
- **Status**: `pending`
- **Priority**: high
- **Depends On**: Task 3
- **Description**:
  - 在 `web/src/i18n/locales/` 的 en/zh/zh-TW/fr/ru/ja/vi 全部 7 个文件中加入弹窗与新文案键，包括但不限于：确认框标题/说明、加载中文案、首选、备用档序号、调整前/调整后、受影响组合数量、无路由变化标题与说明、保存变更按钮、新教学底部文案；删除被替换的旧键 "Changes save on drop. You can undo the last move."。
  - 在 `web/` 执行 `bun run i18n:sync`（若项目脚本会按 en 校准键集合），核对无缺失/冗余键。
- **Acceptance Criteria Addressed**: AC-9, AC-10, NFR-3
- **Test Requirements**:
  - `rule` TR-4.1: 7 个 locale 文件中新键集合一致、旧键不再被代码引用（grep 验证），证据：脚本/ grep 输出。
  - `rule` TR-4.2: 语言切换为 zh 时弹窗标题/按钮/标签显示中文翻译，证据：组件测试或浏览器验证。

## Task 5: 前端测试更新与整体回归
- **Status**: `pending`
- **Priority**: medium
- **Depends On**: Task 2, Task 3, Task 4
- **Description**:
  - 扩展 `web/src/features/channels/components/__tests__/priority-order.test.tsx`（及必要时一个弹窗测试文件）：确认流（AC-1/AC-7）、空状态（AC-5）、掩码（AC-6）、结构与高亮（AC-4）、键盘路径（AC-10）；更新旧用例中"松手即保存"的断言为新流程；保持悬停预览、撤销恢复、教学弹窗相关测试通过。
  - 后端：运行 `gofmt`、`go build ./...`、`go test ./model/ ./controller/ ./router/`；如有本地 MySQL/PG（TEST_*_DSN）则跑三库矩阵，否则记录未运行环境与 SQL 兼容性分析。
  - 前端：`bun run check`、对改动文件 `bun run lint`、受影响测试文件 `bun run test`（vitest）、`bun run build`。
- **Acceptance Criteria Addressed**: AC-1~AC-8, AC-11
- **Test Requirements**:
  - `rule` TR-5.1: 全部新增/扩展用例通过，且无被跳过的关键路径，证据：vitest 输出。
  - `rule` TR-5.2: 后端 build/test 与前端 check/lint/build 全绿；三库矩阵结果或缺失环境的明确阻塞记录，证据：命令输出归档到完成证据。

## Task 6: 浏览器端到端走查
- **Status**: `pending`
- **Priority**: medium
- **Depends On**: Task 5
- **Description**:
  - 本地启动前端（必要时后端）渠道页，构造同渠道多分组多模型、同档多渠道、跨页渠道数据，实际验证：拖放与键盘移动→弹窗数据正确、无变化空态、掩码开关、取消回退、确认保存与 Undo、冲突提示（可通过双窗口模拟）。
- **Acceptance Criteria Addressed**: AC-9, AC-10
- **Test Requirements**:
  - `rubric` TR-6.1: 交互一致性与可读性（AC-10 同维）1-5，阈值 >=4，证据：走查记录（现象、截图或 DOM 断言摘要）。
  - `rule` TR-6.2: 真机浏览器中确认前无写请求、确认后路由缓存生效（日志或网络面板），证据：走查记录。
