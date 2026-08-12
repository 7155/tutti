# 飞书多维表格四项 AgentGUI 问题修复说明

日期：2026-08-12

## 范围与结论

四条现象位于同一 `tutti-os/tutti` monorepo，但属于四条独立数据链路：

1. 删除项目只删除了项目记录，没有按产品文案级联删除该项目的会话；置顶会话因此残留，取消置顶后没有可展示分区。
2. Claude WebFetch 授权事件带有 URL，但 daemon 归一化丢弃 `toolCall.input`，授权卡片读取不到 URL。
3. Claude SDK 的“本次会话允许”只更新当前 Query；Tutti 每个 settled Turn 后创建 fresh Query，权限没有跨 Query 延续。
4. Codex 计划完成后的会话内提示曾未进入 Message Center；当前 `origin/main` 已由 `a95fa07a1` 修复，本次只做完整链路验证。

## 1. 删除项目后取消置顶导致会话消失

### 调用链与根因

项目删除确认文案承诺“删除项目及其全部会话”，但旧调用链仅执行
`userProjects.remove(path)`。项目分区已有的批量删除候选接口又默认使用
`excludePinned: true`，因此不能直接复用来完成项目删除。仍置顶的 Session 继续由
Pinned 投影展示，掩盖了项目记录已不存在的事实；取消置顶后，它重新按持久化的旧
`rail_section_key` 投影，而对应项目模板已经删除，最终无处展示。

直接原因是 Remove project 没有调用 Session 删除链路，并且普通“清空分区”语义会
排除 pinned Session。系统性原因是 UI 文案、项目元数据删除与 canonical Session
生命周期之间没有形成一个 fail-closed 的级联删除编排。

### 修复

用户确认 Remove project 后，编排器才按精确 `sectionKey` 请求权威删除候选快照，明确使用
`excludePinned: false`，且不传 `agentTargetId`，从而覆盖该项目下所有 provider/agent
target 的普通与 pinned Session。确认后将完整快照交给 canonical batch Session
删除；只有删除成功才调用 `userProjects.remove(path)`。确认提交期间使用同步 in-flight
guard 防止重复请求。候选查询或批量删除失败时均
保留项目，供用户重试。

不会把 Session 迁移到 Chats，也不会直接改写 rail 归属。会话生命周期仍由现有批量
删除协调器负责，包括运行时关闭、子会话闭包、保护中执行拒绝、事务删除和资源清理。

### 影响与验证

- 影响：该项目精确 canonical section 下的全部 Session，包括 pinned、未加载分页及不同 agent target 的 Session。
- 不影响：其他项目和 Chats；现有 Session 删除保护、子会话闭包及资源清理语义保持不变。
- 验证：断言候选请求为 `excludePinned: false` 且无 target filter；断言 batch 输入同时包含普通与 pinned Session；断言成功顺序为“删 Sessions → 删项目”，失败时项目不删除。

## 2. Claude WebFetch 授权卡片缺少 URL

### 调用链与根因

Claude SDK `canUseTool(WebFetch, input)` → sidecar `approval_requested.payload.toolCall.input`
→ daemon `normalizedApprovalInput` → canonical Interaction → AgentGUI 授权卡片。

sidecar 已同时发送根级 `input` 和 `toolCall.input`。旧 daemon 只复制 toolCall 的
ID、名称、状态等 identity 字段，明确丢弃了嵌套 input；同时顶层字段 fallback 列表
也没有 `url`/`uri`。GUI 展示模型读取 canonical `toolCall.input`，因此 URL 缺失。

直接原因是归一化字段白名单不完整。系统性原因是 producer 与 consumer 对授权展示
模型的契约不一致，缺少 WebFetch 形状的边界测试。

### 修复

daemon 将清洗后的 display input 同时放入 canonical 根 input 和
`toolCall.input`，继续排除 raw/provider debug/content 等非展示字段；并把 `url`、`uri`
加入旧 provider 顶层形状的 fallback。

### 影响与验证

- 影响：Claude、Codex、ACP 共用的 approval canonical projection；属于加法字段兼容。
- 风险控制：只复制清洗后的展示 input，不重新引入 provider debug 大对象。
- 验证：Claude WebFetch 标准嵌套 input、顶层 URL fallback、原有 edit diff 归一化测试。

## 3. “本次会话允许”未跨 fresh Query 延续

### 调用链与根因

用户点击 `allow_always` → sidecar 返回 SDK `updatedPermissions=suggestions` → 当前 Claude
Query 内存权限更新 → Turn settled → `SessionRuntime` 关闭 Query → 下一次用户消息创建
fresh Query。

Claude SDK 的 `updatedPermissions` 只对收到返回值的 Query 生效。SessionRuntime 会跨
Turn 复用，但旧实现没有保存 SDK suggestion，所以 fresh Query 再次请求同一权限。

直接原因是权限更新生命周期绑定 Query。系统性原因是 UI 的“会话”语义与 SDK 的
Query 生命周期没有适配层。

### 修复

在 `SessionRuntime` 生命周期持有的 `InteractiveCoordinator` 内增加有界内存 ledger：

- 只记录 SDK 原样建议且全部 `destination=session` 的批次；
- 使用稳定 canonical JSON 做精确匹配，不解析或扩大 rule/path/domain 语义；
- fresh Query 再次提出完全相同的 suggestion 时，自动返回该 Query 的
  `updatedPermissions`，不再发授权卡片；
- `allow_once`、持久化 destination、混合 destination、空 suggestion 或不完全匹配仍然询问；
- ledger 不落盘，SessionRuntime 销毁即失效，并设置容量上限。

### 影响与验证

- 影响：仅 Claude SDK sidecar 的普通 tool permission。
- 不影响：AskUserQuestion、ExitPlanMode、bypassPermissions 和其他 provider。
- 验证：ledger 精确匹配/拒绝持久 scope、allow-once 不记忆，以及两个连续 Turn 使用两个 fresh Query 的集成测试。

## 4. Codex 计划提示未进入 Message Center

### 当前基线链路

当前 `origin/main` 已具备完整链路：

`codex_appserver_event_items.go` 将 completed plan item 投影为
`messageKind=plan` → canonical Session/Turn/Message 进入 Activity Engine →
共享 `consumerAwaitingPlanImplementation` 判断已完成 plan 是否待实施 →
`workspaceAgentMessageCenterEngineModel` 生成 `kind=plan-implementation` attention target →
Message Center 与右侧浮层消费同一模型。

修复来自提交 `a95fa07a1`，本分支不重复改写该模型。

### 影响与验证

- 影响：支持 `planImplementation` capability 的 provider；Codex app-server 已声明该能力。
- 验证：Codex completed plan event 生成 tagged plan message、共享 awaiting selector、Message Center canonical target、非当前会话保持 waiting 的三组测试。

## 风险与回滚边界

- 数据迁移发生在项目删除事务中，失败会阻止项目删除，不会留下半完成状态。
- approval projection 是加法变化；旧 consumer 忽略新嵌套字段仍可工作。
- Claude ledger 只驻留内存且精确匹配，主要剩余风险是未来 SDK 改变 suggestion 结构后回退为再次询问，而不是误授权。
- 第 4 条没有本分支运行时代码变化，风险来自基线能力本身，由现有回归覆盖。
