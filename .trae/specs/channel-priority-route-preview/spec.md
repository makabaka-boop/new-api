# 渠道优先级调整的路由变化预览确认 - 产品需求文档

## Overview

- **Summary**：将渠道页"拖拽模式"中**松手即保存**的优先级调整，改为**先预览、再确认后保存**。预览由后端基于全量启用渠道计算，按"分组 → 模型"展示调整前/后的首选档与备用档构成，同档渠道聚合显示；只列出分档成员或档位顺序确实改变的组合，并沿用页面的敏感信息隐藏设置。
- **Purpose**：优先级（priority）是渠道级全局值，一个渠道在 `abilities` 表中对应多个（分组, 模型）组合；拖入新档或引起整段档位平移时，现有提示只给新优先级数字与渠道数量，用户无法看到同一渠道其他模型、以及被连带挪档渠道的路由变化，容易在未察觉时改变故障切换链。
- **Target Users**：在渠道页使用拖拽模式（或键盘方向键）调整渠道优先级、准备切换上游的管理员/具备渠道编辑权限的用户。

## Goals

- 拖放松手或键盘移动渠道后，**不直接保存**，先弹出路由变化确认框。
- 预览按**分组、模型**组织，分别展示调整前后的**首选档（rank 0）与备用档（rank ≥ 1）**，同一档的渠道成组展示。
- 计算覆盖**所有启用渠道**（基于 `abilities.enabled = true` 全量行），不受当前分页、筛选影响。
- 仅列出**渠道分档或档位顺序确实改变**的（分组, 模型）组合；纯空隙重编号（每档成员与相对顺序不变）不列出；没有任何变化时明确提示且不保存。
- 预览沿用页面的敏感信息隐藏开关：隐藏时分组名、渠道名显示为 `••••`，模型名仍可见。
- **确认后才保存**；取消或预览失败时原配置不变，失败时展示服务端返回的原因。
- 沿用现有渠道编辑权限（`authz.ChannelWrite`）、revision 保存冲突检查、restore 撤销操作。

## Non-Goals

- 不改变行内数字编辑（优先级数字输入框走 `PUT /api/channel/`）、按 tag 批量编辑（`PUT /api/channel/tag`）的行为，这两条路径不增加预览。
- 不为"撤销（Undo/restore）"增加确认框：撤销恢复的是上一次已确认的状态，保持现有一键撤销行为。
- 不改变权重（weight）、重试次数（RetryTimes）、模型别名匹配等既有路由语义；不改数据库表结构与迁移。
- 不改变拖拽过程中的悬停预览（仍为轻量的新优先级数字提示）。

## Background & Context

- 前端入口：`web/src/features/channels/components/channels-table.tsx` 中 `reorderActive` 时启用 `useChannelPriorityDrag` 与 `useChannelPriorityOrder`；手柄组件 `ChannelPriorityHandle`，状态条 `ChannelPriorityStatus`。
- 现状流程：手势开始 `GET /api/channel/priority/order` 取 `{revision, items}`；悬停 200ms 防抖调 `POST /api/channel/priority/reorder`（`preview:true`）只拿数字结果；松手/方向键后 `commit()` 立即用同一接口非 preview 提交并 `InitChannelCache()`，toast `Channel priorities saved`，保存后可通过 `restore` 数组撤销。
- 后端：`model/channel_priority.go` 的 `ReorderChannelPriority`（事务 + revision 快照校验 + `lockForUpdate` 行锁，preview 不写库、不加锁）与 `planChannelPriorityMove`（等优先级=同档；开新槽时整体平移较短连续档位，可能连带修改其他渠道 priority）。
- 路由语义：每个（分组, 模型）下渠道按 priority 降序分档，`retry=0` 取最高档（首选），重试逐档下沉（备用）；同档内按 weight 加权随机，档内顺序不是路由顺序。见 `model/channel_cache.go`、`model/ability.go`。
- `abilities` 行的 `enabled` 随渠道启停同步（`AddAbilities`/`UpdateAbilities`/`UpdateAbilityStatus`），以 `enabled = true` 表达"启用渠道 × 分组 × 模型"。
- 敏感信息隐藏开关：`ChannelsProvider` 的 `sensitiveVisible`（默认 true，页面级状态），关闭时渠道名、ID、分组徽标等统一显示 `SENSITIVE_MASK = '••••'`（见 `channels-columns.tsx`、`channel-card.tsx`）。
- 可复用组件：`@/components/dialog`（可滚动弹窗布局）、`@/components/ui/button`、`@/components/ui/badge`、`@/components/group-badge`（`GroupBadge`，支持 `label` 覆盖为掩码）、`@/components/empty-state`、`@/components/ui/spinner`。

## Functional Requirements

- **FR-1：确认前不保存。** 拖放落点有效并松手（含落点动画结束）或键盘方向键移动后，前端调用一次带 `preview: true` 与新增路由明细开关的预览请求，弹出确认框；在用户点击确认前，不发起任何非 preview 的写请求，表格数据保持原优先级顺序。
- **FR-2：后端路由分档预览。** `POST /api/channel/priority/reorder` 在 `preview:true` 且请求携带路由明细开关时，响应在现有 `revision/items/changes` 之外增加：
  - 只包含**发生变化**的组合列表，每项：`group`、`model`、`before: Tier[]`、`after: Tier[]`；`Tier = { priority: number|null, channels: number[] }`，档位按 priority 降序（NULL 按有效值 0 归档，与 `GetPriority()`/内存缓存一致），档内渠道 id 升序（档内为 weight 加权随机，id 序仅用于稳定展示）。
  - 出现在任意变化组合中的渠道 **id → 名称**映射（用于当前页之外渠道的名称展示）。
- **FR-3：变化判定。** 对每个启用的（分组, 模型）组合，比较调整前/后的有序档成员序列：档数不同、或任一档位的渠道成员集合不同（含渠道跨档、档合并、档分裂、首选/备用互换）才列入；仅有 priority 数值变化但各档成员与相对顺序完全一致的组合不列入。被禁用渠道（`abilities.enabled = false`）不参与任何档位。
- **FR-4：计算范围与一致性。** 分档基于数据库中全量 `abilities` 启用行（单次查询），在移动计划所在事务内用同一 `tx` 读取，与 revision 快照一致；不写入任何数据。
- **FR-5：确认框展示。**
  - 按分组分区（分组名用 `GroupBadge` 风格展示），分组下按模型列出条目；每条目左右（或上下）对照"调整前 / 调整后"，逐档展示：rank 0 标注"首选"，rank ≥ 1 标注"备用 n"，同档渠道以成组徽标展示。
  - 高亮调整前后档位发生变化的渠道徽标。
  - 顶部给出受影响组合数量。
  - 无变化组合时展示明确空状态（标题+说明），底部仅提供"关闭"，不发起保存。
- **FR-6：确认 / 取消 / 失败。**
  - 确认：以手势开始时快照的 revision 发起非 preview 提交（语义同现状）；成功后刷新渠道列表查询、显示保存成功提示、保留撤销能力并关闭弹窗。
  - 取消（含遮罩/ESC/取消按钮）：结束本次手势，不保存，表格保持原顺序与优先级。
  - 预览请求失败：不打开可确认的弹窗、不保存，经 `handleServerError` 提示原因，界面回到原配置。
  - 确认提交失败（含 revision 冲突）：经 `handleServerError` 展示服务端原因（冲突时为现有 "Channel priorities changed. Refresh and try again." 文案），刷新列表查询，原配置未被修改。
- **FR-7：掩码沿用。** 弹窗中分组名与渠道名在 `sensitiveVisible = false` 时一律显示 `••••`（不泄露 id/名称）；模型名不受隐藏影响。
- **FR-8：键盘可达。** 键盘方向键触发的移动走同一预览/确认流程；弹窗焦点管理、ESC 取消、确认/取消按钮均可键盘操作。
- **FR-9：文案更新。** 拖拽教学弹窗底部与相关提示不再出现"松手即保存（Changes save on drop）"措辞，改为"先查看路由变化预览，确认后保存，可撤销"。
- **FR-10：权限与撤销不变。** 路由预览仍挂在现有 `POST /api/channel/priority/reorder`（`authz.ChannelWrite`）；不新增路由；revision 冲突、restore 撤销、成功后 `InitChannelCache()` 的行为保持不变。

## Non-Functional Requirements

- **NFR-1（性能）**：路由明细仅在需要渲染确认框时（drop 后一次）请求；悬停预览不携带该开关、不产生分档计算开销。分档计算为 abilities 单次查询 + 内存分组，典型规模（万级 abilities 行）下响应时间与现有 preview 同量级。
- **NFR-2（数据库兼容）**：仅新增只读查询，不使用方言 SQL、不改 schema；查询与布尔参数在 SQLite、MySQL ≥ 5.7.8、PostgreSQL ≥ 9.6 下行为一致。
- **NFR-3（前端规范）**：复用既有弹窗/徽标/空状态组件，不新增第三方依赖；TS 无 `any`，`bun run check`、lint、受影响测试通过；新增用户文案进入全部 7 个 locale 文件。
- **NFR-4（可观测行为）**：弹窗具备加载中（等待预览）、空（无路由变化）、错误（失败提示由全局 toast 承担）、正常列表四种状态；`role=status`/`aria-live` 与现有状态条风格一致。
- **NFR-5（测试）**：后端在现有 `model/channel_priority_test.go` 的三库矩阵中覆盖路由预览（SQLite 必跑，MySQL/PG 经 `TEST_MYSQL_DSN`/`TEST_POSTGRES_DSN` 启用），新增纯分档判定的表驱动单测；前端在现有 `priority-order.test.tsx` 中覆盖确认流、取消、无变化、掩码、失败与键盘路径，不新建多余测试文件。

## Constraints

- **Technical**：Go 根模块 JSON 经 `common.*` 包装（控制器仍用既有 `c.JSON(gin.H{...})` 响应模式，与现状一致）；GORM 优先、跨三库；前端 React 19 + Base UI + TanStack Query + i18next；复用 `@/components/dialog` 等现有组件。
- **Business**：不改动 new-api / QuantumNous 品牌信息；不新增 docs/ 下文档文件；错误文案沿用现有 i18n 键与前端翻译机制。
- **Dependencies**：不新增前后端依赖；`relaykit/`、`tokenkit/` 不涉及。

## Assumptions

- "首选档/备用档"即现有 priority 分层语义：rank 0 = 最高优先级档，rank n = 第 n 次重试下沉到的档；"档位顺序改变"通过有序档成员序列的差异判定（见 FR-3），而非 priority 数字本身。
- 撤销（restore）维持一键执行、不加确认框，符合需求中"沿用……撤销操作"的表述；撤销恢复的 before 状态即用户此前已确认过的状态。
- 弹窗名称数据以后端返回的 id→name 映射为准（保证含当前页之外渠道），本地列表仅作可选补充。
- 模型名不属于页面隐藏范围（现有掩码仅覆盖渠道名/ID/分组/余额等）。

## Acceptance Criteria

### AC-1：移动后先预览、确认前不写入
- **Type**: `rule`
- **Given**：具备 channel write 权限的用户在拖拽模式下，把渠道拖到有效落点并松手（或聚焦手柄按方向键）
- **When**：落点动画结束 / 方向键触发
- **Then**：前端发出的 reorder 请求为 `preview:true` 且携带路由明细开关；确认框出现前及点击"取消"前，不存在非 preview 的 `/api/channel/priority/reorder` 写请求，渠道列表优先级顺序不变
- **Pass Condition**：测试断言网络请求序列与列表状态满足上述条件
- **Evidence**：`priority-order.test.tsx` 回归用例（mock API 请求断言）

### AC-2：预览覆盖全量启用渠道（含当前页之外）
- **Type**: `rule`
- **Given**：存在未出现在当前列表页的启用渠道，且其（分组, 模型）档位受本次移动影响
- **When**：请求路由明细预览
- **Then**：返回的变化组合与渠道名称映射包含该页外渠道
- **Pass Condition**：后端测试（构造超出前端页大小的渠道集合）断言页外渠道出现在 routes 与名称映射中
- **Evidence**：`model/channel_priority_test.go` 新增用例

### AC-3：只返回分档/档序实际改变的组合；禁用渠道排除
- **Type**: `rule`
- **Given**：多个（分组, 模型）组合，其中：A 组合有渠道跨档；B 组合仅发生空隙重编号（各档成员与顺序不变）；C 组合含受影响渠道但该渠道在 C 中被禁用
- **When**：计算路由明细
- **Then**：routes 仅含 A；B、C 不出现；A 的 before/after 档数、每档 priority 与成员 id（升序）正确
- **Pass Condition**：表驱动单测对三种组合逐一断言
- **Evidence**：`model/channel_priority_test.go` 纯函数用例

### AC-4：首选/备用档与同档聚合展示，名称映射可渲染页外渠道
- **Type**: `rule`
- **Given**：确认框打开且存在变化组合
- **When**：渲染弹窗
- **Then**：按分组分区、其下按模型条目展示 before/after；rank 0 标注首选、rank≥1 标注备用序号；同档渠道在同一徽标组内；跨档变化的渠道徽标有可辨识高亮；页外渠道名称来自后端映射正常显示
- **Pass Condition**：组件测试断言分组/模型/档位结构、同档聚合与高亮、页外渠道名
- **Evidence**：确认弹窗组件测试

### AC-5：无变化时明确提示且不保存
- **Type**: `rule`
- **Given**：预览结果 `changes` 或 `routes` 为空（无效移动/纯路由中性移动）
- **When**：弹窗展示
- **Then**：出现明确的"无路由变化"空状态文案，底部没有"保存"动作（仅关闭）；关闭过程无任何写请求
- **Pass Condition**：测试断言空状态可见且写请求不发生
- **Evidence**：确认弹窗组件测试

### AC-6：掩码设置沿用
- **Type**: `rule`
- **Given**：页面敏感信息开关为隐藏（`sensitiveVisible=false`）
- **When**：打开确认框
- **Then**：分组名与所有渠道名显示为 `••••`，不出现真实名称/ID；模型名正常显示
- **Pass Condition**：测试在隐藏态断言掩码存在、真实渠道名与分组名不存在、模型名存在
- **Evidence**：确认弹窗/交互测试

### AC-7：确认保存、取消与失败处理
- **Type**: `rule`
- **Given**：确认框处于打开 / 预览请求失败 / 确认提交返回 revision 冲突三种情形
- **When**：分别点击确认、点击取消、模拟失败
- **Then**：确认→非 preview 提交（revision 为手势开始时快照值）、成功后列表查询失效刷新、出现保存成功提示、Undo 可用；取消→无写请求且列表保持原值；预览失败→toast 展示服务端原因、无写请求、无确认框残留；提交冲突→toast 展示 "Channel priorities changed. Refresh and try again." 对应翻译、列表刷新、数据未被修改
- **Pass Condition**：四条分支均有测试断言（请求、toast 文案、undo 按钮、列表状态）
- **Evidence**：`priority-order.test.tsx` 用例

### AC-8：权限、冲突检查与撤销沿用
- **Type**: `rule`
- **Given**：现有 reorder 接口的权限与并发控制
- **When**：本次改造完成
- **Then**：不新增路由；接口仍为 `authz.ChannelWrite`；revision 不匹配返回既有冲突错误；preview 不落库；restore 撤销请求行为与现在一致（成功后恢复原档位并刷新缓存）
- **Pass Condition**：路由测试与三库矩阵中 preview/conflict/restore 断言全部保持通过
- **Evidence**：`router/channel_router_test.go`、`model/channel_priority_test.go`

### AC-9：预览信息架构可读性
- **Type**: `rubric`
- **Dimension**：用户能否快速按"分组→模型→前/后首选备用档"理解路由影响并定位变化
- **Scale**: 1-5
- **Anchors**：1 = 仅罗列渠道与数字，无法看出分档变化；3 = 有分组/模型与前后对照，但同档聚合或变化高亮缺失；5 = 分组分区、模型条目、首选/备用标签、同档成组、变化高亮、受影响数量齐备且布局清晰
- **Pass Threshold**: >= 4
- **Evidence**：组件实现走查 + 浏览器实际操作截图/交互验证

### AC-10：交互一致性、状态完整与可访问性
- **Type**: `rubric`
- **Dimension**：与渠道页现有拖拽体验、视觉语言、键盘/读屏可达性的一致程度
- **Scale**: 1-5
- **Anchors**：1 = 打断拖拽流程且状态丢失；3 = 流程可用但加载/空/失败态不全或键盘操作受阻；5 = 拖拽与键盘路径一致、loading/empty 态完整、ESC/焦点/aria 完备、文案已去除"松手即保存"
- **Pass Threshold**: >= 4
- **Evidence**：浏览器键盘操作验证 + 组件测试 + 教学弹窗文案检查

### AC-11：实现质量与数据库兼容
- **Type**: `rubric`
- **Dimension**：代码直接可读、复用现有组件/查询模式、三库兼容且无多余抽象/依赖
- **Scale**: 1-5
- **Anchors**：1 = 引入方言 SQL/新依赖或重复实现控件；3 = 功能正确但有冗余抽象或未完成 lint/类型检查；5 = 单一只读查询、复用共享组件、Go/TS 检查与三库测试全绿、i18n 7 语言齐全
- **Pass Threshold**: >= 4
- **Evidence**：`go build`/`go test`、`bun run check`/lint/测试、三库矩阵结果与代码走查

## Open Questions

无（需求中"确认后才保存""沿用权限/冲突检查/撤销""沿用隐藏设置"等表述已明确；撤销不加确认框、行内编辑与 tag 批量编辑不在范围内，见 Non-Goals/Assumptions）。
