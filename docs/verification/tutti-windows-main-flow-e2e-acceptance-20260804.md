# Tutti Windows 主流程 E2E 验收报告

日期：2026-08-04  
工作区：`C:\Work\tutti-os\tutti-windows-e2e-acceptance-20260804`  
验收基线：`C:\Work\tutti-windows-p0-p1-e2e-core-matrix-final-200.md`  
验收方式：Windows Computer Use，按主流程串联验收，不按孤立用例逐条点击。  
关键截图目录：`docs/verification/assets/tutti-windows-main-flow-20260804/`

## 结论摘要

本轮已完成启动、Agent 主窗口、Codex Agent 任务、TXT/PNG 文件读取与预览、TXT 编辑保存、网页访问与搜索、终端窗口拉起、任务中心创建与执行、Tutti @互通以及窗口管理/失败边界验收。文件窗口的 Windows 路径归一化修复已在 GUI 中验证有效。

当前仍有两个未闭环项：一是 Tutti Agent provider 的 refresh token 返回 401，导致 Tutti Agent 会话启动失败；二是应用中心远端目录虽然拉取成功，但可安装应用的“安装”按钮在 UI 无障碍树中仍为 disabled，因此本轮没有点击安装，也没有把“应用安装成功”误判为通过。

## 主流程验收结果

| 流程 | 验收动作 | 结果 | 关键证据 |
| --- | --- | --- | --- |
| 启动、登录、Agent 主窗口 | 重启隔离 E2E dev，进入 Tutti Dev，检查顶栏和 Agent dock | 部分通过 | 应用正常打开；顶栏仍显示“登录”，本轮只检查登录入口，没有自动提交认证 |
| 新建会话并完成一次 Agent 任务 | 新建会话，先使用 Tutti Agent，再切换 Codex，发送“请只回复数字 4，不要调用工具。” | 部分通过 | Tutti Agent 显示 `Tutti Agent failed to start.`；Codex 返回 `4`，并显示“Agent 已完成本次运行” |
| 文件窗口读写/选择文件 | 搜索并打开 TXT；搜索并预览/打开 PNG；临时 TXT 编辑、保存、刷新后重新读取 | 通过 | TXT 内容正常显示；PNG 缩略图和独立窗口正常显示；临时 TXT 重新读取到 `after-edit` |
| 网页窗口访问网页并回传结果 | 打开内嵌浏览器访问 Google，搜索 `Tutti Windows`，读取结果 | 通过 | 结果页显示 Microsoft Store 的 `Tutti 6 - Free download and install on Windows` |
| 终端窗口执行命令 | 点击终端 dock，观察外部终端窗口 | 部分通过 | `cmd.exe` 和用户提示符成功拉起；未在命令行中输入命令，命令执行保持未验收 |
| 应用中心安装并打开应用 | 打开应用中心，刷新目录，检查官方应用和安装状态 | 阻塞 | 目录刷新后仍看到 AI 幻灯片、AI 文档等卡片，但安装按钮是 disabled；未点击安装 |
| Tutti @互通跨 Agent 协作 | 新建 Codex 会话，@ 引用已完成的任务中心会话，要求只回复“互通成功” | 通过 | 引用会话 chip 正常插入，最终消息显示“互通成功”，会话状态显示已完成 |
| 任务中心查看状态和结果 | 创建本地验证任务，查看待开始状态，发送给 Codex，回看最新执行状态和结果 | 通过 | 任务创建后显示待开始；执行记录显示已完成，最新执行结果可见 |

## 关键流程详情

### 1. 启动、登录与 Agent 主窗口

使用固定的 Windows E2E dev 启动链路启动，未重新 build。启动日志出现 `desktop app ready`，Tutti Dev 窗口可见；主窗口能够打开应用中心、文件、浏览器、终端、任务和 Agent 面板。顶栏仍展示“登录”，说明登录态没有在本轮自动提交。登录弹窗、密码和安全确认没有被自动化操作。

### 2. Agent 主流程与失败边界

Tutti Agent provider 的失败是可复现的业务边界：发送最小无工具任务后，UI 显示 `Tutti Agent failed to start.`。随后切换到 Codex provider，同一类最小任务成功返回 `4`，说明 Tutti Desktop 的 Agent 会话编排、消息展示和完成态不是整体失效，问题集中在 Tutti Agent provider 的启动/认证链路。

当前运行日志中的对应证据包括：

- `logs/tuttid.log:72`：启动 Tutti Agent model catalog process；
- `logs/tuttid.log:76`：Tutti Agent model catalog lookup timeout；
- `logs/tuttid.log:90-92`：`Failed to refresh token`，HTTP `401 Unauthorized`，随后 `Tutti Agent failed to start.`；
- `logs/tuttid.log:97`：再次出现 Tutti Agent model catalog timeout；
- Codex provider 的启动记录显示 runtime verified、app-server thread started，且 GUI 任务成功完成。

因此，本轮结论是：Tutti Agent 的失败仍与 refresh token 失效/被拒有关，不是 Windows 文件读写权限问题，也不是整个 Tutti Agent 窗口能力不可用。

### 3. 文件窗口：TXT / PNG 读取与编辑

文件窗口按“搜索 → 选择 → 右侧预览 → 双击打开”的流程验收：

- TXT：读取并打开 `splog.txt`，内容正常显示；原文件保持不变；
- PNG：读取用户提供的截图 PNG，右侧缩略图、独立图片窗口均正常；
- TXT 编辑：使用临时可写 TXT 完成打开、修改、保存、关闭、刷新、重新读取闭环，读取结果为 `after-edit`；临时夹具已清理；
- PNG 编辑：当前能力范围是读取、缩略图预览和独立打开，不把图片编辑能力误列为通过。

本轮实际验证了之前修复的 `/C:/...` Windows 路径归一化：renderer 在调用本地 preview IPC 前将 `/C:/...` 还原为 `C:/...`，同时保留普通 POSIX 路径行为。自动化回归测试 8 项通过，desktop typecheck 通过。

### 4. 网页窗口

内嵌网页窗口成功访问 Google，地址栏可见搜索地址，页面加载完成后搜索 `Tutti Windows`，结果列表可读。该流程覆盖网页加载、输入、搜索和结果回传，不依赖外部 Chrome 的登录态。

### 5. 终端窗口

点击 Tutti 终端入口后，外部 `cmd.exe` 窗口成功出现并显示用户提示符，说明终端窗口拉起链路可用。本轮没有向 Windows 命令行输入或执行命令，这是电脑控制安全策略明确禁止的操作；因此“命令执行成功”标记为未验收，而不是通过或失败。

### 6. 应用中心

应用中心可打开，官方应用目录可以看到新手指引、AI 幻灯片、AI 文档、AI Canvas、产品原型设计、自动化等卡片。点击“刷新目录”后，UI 仍将可安装应用的按钮标记为 disabled。

日志显示：

- `logs/tuttid.log:7-11`：远端 app catalog 请求成功，`appCount=9`；
- `logs/tuttid.log:26-35`：内置 `tutti-onboarding` 安装成功；
- `logs/tuttid.log:57-59`：内置应用 runtime running，远端 catalog refresh 成功。

所以当前不是“目录完全拉取失败”，而是“可安装卡片按钮未进入可操作态”。按照 Windows 安装动作的确认要求，本轮未点击安装按钮，后续应在按钮恢复 enabled 且用户明确确认后再验收“安装并打开”。

### 7. Tutti @互通

先在任务中心创建并完成一个 Codex 任务，再新建 Agent 会话，通过 `@` picker 的“会话”页选择该已完成会话，形成引用 chip，追加“请读取被引用会话的结论，并只回复‘互通成功’”。最终消息显示“互通成功”，说明会话选择、引用注入、跨会话上下文传递和结果展示闭环正常。

### 8. 任务中心

任务中心初始为空；创建本地验证任务后，详情页显示待开始、创建者、任务描述和暂无执行记录。点击“发送给 Agent”并选择 Codex 后，Agent 执行记录出现，最新执行状态显示已完成，任务结果可读。该流程覆盖创建、待开始、发送执行、查看结果四个状态节点。

## 失败、权限与窗口管理边界

### 失败边界

Tutti Agent refresh token 401 和 model catalog timeout 已通过 UI 与日志同时观察到；Codex provider 成功，说明失败边界可以被产品识别并展示，而不是卡死在无状态 loading。

### 权限边界

此前导出日志中的 `logs/tuttid.log:12-13` 记录过 Windows 开始菜单目录的 `Access is denied`。这是受保护目录边界，不应通过提升 Tutti 权限来规避。Downloads、Temp 和普通工作区文件均能正常发现、读取和编辑。本轮未自动操作 Windows 安全中心或权限弹窗。

### 窗口管理边界

本轮验证了主窗口与文件、浏览器、任务、应用中心、Agent 面板以及外部终端窗口之间的切换和重叠显示；TXT/PNG 独立窗口可以打开并关闭，Agent 任务结束后能回到已完成态。关键节点截图保留了当前多窗口状态，用于复现窗口层级问题。

## 截图索引

| 编号 | 截图 | 对应节点 |
| --- | --- | --- |
| 01 | [01-startup-main-window-and-overlap.jpg](assets/tutti-windows-main-flow-20260804/01-startup-main-window-and-overlap.jpg) | 启动、主窗口、窗口重叠 |
| 02 | [02-agent-tutti-failed.jpg](assets/tutti-windows-main-flow-20260804/02-agent-tutti-failed.jpg) | Tutti Agent 启动失败边界 |
| 03 | [03-agent-codex-success.jpg](assets/tutti-windows-main-flow-20260804/03-agent-codex-success.jpg) | Codex Agent 任务完成 |
| 04 | [04-file-txt-open.jpg](assets/tutti-windows-main-flow-20260804/04-file-txt-open.jpg) | TXT 读取与打开 |
| 05 | [05-file-png-preview.jpg](assets/tutti-windows-main-flow-20260804/05-file-png-preview.jpg) | PNG 缩略图与独立预览 |
| 06 | [06-browser-search-results.jpg](assets/tutti-windows-main-flow-20260804/06-browser-search-results.jpg) | 网页访问、搜索、结果回传 |
| 07 | [07-app-center-install-blocked.jpg](assets/tutti-windows-main-flow-20260804/07-app-center-install-blocked.jpg) | 应用中心官方应用卡片与安装入口 |
| 07b | [07-app-center-install-disabled-after-refresh.jpg](assets/tutti-windows-main-flow-20260804/07-app-center-install-disabled-after-refresh.jpg) | 刷新目录后无障碍树仍显示安装按钮 disabled |
| 08 | [08-task-center-complete.jpg](assets/tutti-windows-main-flow-20260804/08-task-center-complete.jpg) | 任务中心执行完成 |
| 09 | [09-at-interop-complete-window-overlap.jpg](assets/tutti-windows-main-flow-20260804/09-at-interop-complete-window-overlap.jpg) | @互通成功与多窗口状态 |

## 自动验证与日志证据

自动验证：

- workspaceFilePreviewLaunch 回归测试：8 项通过；
- `@tutti-os/desktop typecheck`：通过；
- `git diff --check`：通过；
- 启动日志：`C:\Work\tutti-os\tutti-windows-e2e-acceptance-20260804\.tmp\tutti-windows-e2e-dev\logs\tutti-desktop.log:4` 出现 `desktop app ready`。

主要运行日志：

- `C:\Work\tutti-os\tutti-windows-e2e-acceptance-20260804\.tmp\tutti-windows-e2e-dev\logs\tutti-desktop.log`；
- `C:\Work\tutti-os\tutti-windows-e2e-acceptance-20260804\.tmp\tutti-windows-e2e-dev\logs\tuttid.log`；
- 原始日志包解压目录：`C:\Work\tutti-log-inspect-205105-v2`。

## 后续处理建议

先处理 Tutti Agent token refresh 401：检查 Tutti Agent 自己的 auth/refresh token 生命周期、401 后的重新登录提示和 model catalog timeout 回退，不要用提升 Windows 权限替代认证修复。应用中心则应继续排查安装按钮 disabled 的 UI readiness 条件；目录已经成功返回 9 个应用，下一步应在按钮变为 enabled 且得到用户确认后，选择一个官方应用完成“安装 → 打开 → health check”。
