# Tutti Windows 主流程 E2E 验收报告

> 验收日期：2026-08-04  
> 验收工作区：`C:\Work\tutti-os\tutti-windows-e2e-acceptance-20260804`  
> 验收基线：`C:\Work\tutti-windows-p0-p1-e2e-core-matrix-final-200.md`  
> 验收方式：使用 Windows Computer Use 按主流程串联操作；不把 200 条用例拆成互不关联的单点点击。  
> 启动方式：`tools/scripts/start-windows-e2e-dev.ps1`，本轮未重新 build。

## 一、结论

本轮完成了 Tutti Windows 的启动、窗口管理、文件窗口、网页窗口、终端窗口、应用中心、Agent 互动入口、任务中心及 Agent Provider 边界验收。

结论不是“全部通过”，而是：

- 主窗口和多窗口工作台可启动，文件检索、文件选择、TXT 预览、PNG 预览、网页访问、内置新手指引应用打开、任务中心空状态展示、窗口最小化/恢复均可复现。
- Tutti Agent 新建会话后执行最小任务仍显示 `Tutti Agent failed to start.`，因此 Agent 任务主链路、基于 Agent 的网页结果回传、@互通实际协作、任务执行结果无法闭环验收。
- 文件窗口的 TXT/PNG “打开”动作均出现“出了点问题，请稍后再试。”；本轮只确认了读/预览和选择，不把打开/编辑误判为通过。
- Tutti 终端窗口可以拉起，但通过 Computer Use 输入命令后，`echo`/`ver` 未按原文执行，终端显示无效输入错误；命令执行能力判定为失败。
- 应用中心目录可展示，内置“新手指引”可打开；可安装应用的“安装”按钮全部为 disabled，应用安装链路被 UI readiness 阻塞，没有强行点击或绕过确认。
- Cursor 与 OpenCode 当前均为未连接/未安装状态。日志分别记录 Cursor 官方安装脚本识别为 `MSYS_NT` 不支持，以及 OpenCode 安装脚本缺少 `tr` 命令；这属于 Provider 安装运行环境问题，不是本轮文件读写问题。

## 二、主流程验收矩阵

| 流程 | 串联场景 | 结果 | 关键证据 |
| --- | --- | --- | --- |
| FLOW-01 | 启动 → Agent 主窗口 → 登录入口 | 部分通过 | Tutti Dev 可启动，顶部存在“登录”入口；本轮未自动操作账号/安全确认，登录态未作为通过条件 |
| FLOW-02 | 新建 Agent 会话 → 提交最小任务 → 等待结果 | 失败 | Tutti Agent 显示 `Tutti Agent failed to start.`；见 `flow-03-agent-task-complete.png` |
| FLOW-03 | 文件窗口 → 选择 POSM → 搜索 TXT → 读取预览 | 通过 | POSM 可选中；TXT 内容和路径可见；见 `flow-04-file-select-posm.png`、`flow-04-file-read-txt.png` |
| FLOW-04 | 文件窗口 → 搜索 PNG → 读取/预览 → 打开 | 部分通过 | PNG 可选中且右侧显示图形预览；打开动作报错；见 `flow-04-file-read-img.png`、`flow-04-file-open-img.png` |
| FLOW-05 | TXT/PNG → 打开/编辑 | 失败 | TXT、PNG 打开子窗口均出现“出了点问题，请稍后再试。”；见 `flow-04-file-open-txt.png`、`flow-04-file-open-img.png` |
| FLOW-06 | 网页窗口 → 访问网页 → 读取页面结果 | 通过 | `https://example.com/` 加载成功，页面标题和正文可读；见 `flow-05-web-page-loaded.png` |
| FLOW-07 | 终端窗口 → 执行命令 → 检查输出 | 失败 | `cmd.exe` 窗口可拉起，但输入被转义/未按原文执行；见 `flow-06-terminal-command.png`、`flow-06-terminal-command-retry.png` |
| FLOW-08 | 应用中心 → 刷新/查看目录 → 安装应用 → 打开 | 部分通过/安装阻塞 | 目录可见；安装按钮 disabled；已安装的新手指引可打开；见 `flow-07-app-center.png`、`flow-07-app-open-result.png` |
| FLOW-09 | Agent 互动入口 → 跨 Agent 协作 | 入口通过、协作阻塞 | “Agent 互动”入口和 Big @ 入口可见，但 Tutti Agent 未启动，未形成实际跨 Agent 结果；见 `flow-08-agent-interaction-entry.png` |
| FLOW-10 | 任务中心 → 查看状态和结果 | 空状态通过、执行结果阻塞 | 任务中心可打开并显示“还没有任务”；因 Agent 任务失败没有可查看的执行结果；见 `flow-09-task-center.png` |
| FLOW-11 | 多窗口 → 最小化 → 从 dock 恢复 | 通过 | Agent、文件、网页、终端、应用、任务窗口同时存在；任务窗口可最小化并恢复；见 `flow-10-window-minimize.png`、`flow-10-window-restore.png` |
| FLOW-12 | Provider 失败边界：Cursor/OpenCode | 失败可识别 | UI 显示需要先连接；日志给出安装脚本失败原因；见 `flow-11-provider-cursor-result.png`、`flow-11-provider-opencode-result.png` |
| FLOW-13 | 权限/安全边界 | 未执行高风险操作 | 本轮未自动操作 Windows 安全中心、UAC 或凭据弹窗；未发现权限弹窗阻塞本轮已允许的文件预览/网页/窗口操作 |

## 三、关键流程证据

### 1. 启动、Agent 主窗口和失败状态

启动脚本输出 `Tutti Windows E2E dev is ready`，应用主窗口可以显示 Agent、文件、网页、终端、应用和任务入口。当前顶部仍有“登录”入口，本轮没有代替用户提交登录凭据或确认安全弹窗。

![启动后的 Tutti 主窗口](C:/Work/tutti-os/tutti-windows-e2e-acceptance-20260804/docs/verification/assets/tutti-windows-main-flow-20260804/flow-01-restart-main-window.png)

提交最小任务 `请只回复：TUTTI-E2E-AGENT-OK` 后，界面明确显示 `Tutti Agent failed to start.`。这是本轮 FLOW-02 的失败证据，不是网络页面或文件窗口造成的假失败。

![Tutti Agent 任务启动失败](C:/Work/tutti-os/tutti-windows-e2e-acceptance-20260804/docs/verification/assets/tutti-windows-main-flow-20260804/flow-03-agent-task-complete.png)

日志证据：

- `C:\Work\tutti-os\tutti-windows-e2e-acceptance-20260804\.tmp\tutti-windows-e2e-dev\logs\tutti-desktop.log` 中，Tutti Agent 的 create 请求返回 HTTP 502，`errorReason=provider_error`。
- 同一日志上下文有 `agentTargetId=local:tutti-agent`，说明失败发生在 Tutti Agent Provider 启动链路，不是 Agent GUI 的发送按钮不可用。
- `tuttid.log` 中可见 Tutti Agent 的 model catalog/认证 bootstrap 相关记录；此前导出日志还出现 refresh token 401，后续应继续单独修复 token 生命周期和 catalog 超时，不应通过提升 Windows 权限替代认证修复。

### 2. 文件窗口：TXT/PNG 选择、读取、打开

文件窗口可以定位并选中用户文件，也可以搜索本轮 fixture：

- `C:\Users\15514\Downloads\tutti-e2e-sample.txt`
- `C:\Users\15514\Downloads\tutti-e2e-sample.png`

TXT 预览能展示 `TUTTI-WINDOWS-E2E-TXT-READ-WRITE-OK` 和第二行内容；PNG 右侧能展示图形预览。这两项判定为读取/选择通过。

![选择 POSM 文件](C:/Work/tutti-os/tutti-windows-e2e-acceptance-20260804/docs/verification/assets/tutti-windows-main-flow-20260804/flow-04-file-select-posm.png)

![读取 TXT 预览](C:/Work/tutti-os/tutti-windows-e2e-acceptance-20260804/docs/verification/assets/tutti-windows-main-flow-20260804/flow-04-file-read-txt.png)

![读取 PNG 预览](C:/Work/tutti-os/tutti-windows-e2e-acceptance-20260804/docs/verification/assets/tutti-windows-main-flow-20260804/flow-04-file-read-img.png)

但继续执行“打开”时，TXT 和 PNG 都进入同一种错误子窗口：`出了点问题，请稍后再试。`。因此本轮没有把编辑、保存、重新读取判为通过，也没有修改用户原文件。

![TXT 打开失败](C:/Work/tutti-os/tutti-windows-e2e-acceptance-20260804/docs/verification/assets/tutti-windows-main-flow-20260804/flow-04-file-open-txt.png)

![PNG 打开失败](C:/Work/tutti-os/tutti-windows-e2e-acceptance-20260804/docs/verification/assets/tutti-windows-main-flow-20260804/flow-04-file-open-img.png)

### 3. 网页窗口

内嵌浏览器地址栏输入 `https://example.com/` 后页面正常加载，`Example Domain` 标题、正文和 `Learn more` 链接均出现在可访问树中。网页读取链路通过；由于 Tutti Agent 同时不可用，本轮只验证了页面结果读取，没有伪造“已回传给 Agent”的通过结论。

![网页窗口加载 Example Domain](C:/Work/tutti-os/tutti-windows-e2e-acceptance-20260804/docs/verification/assets/tutti-windows-main-flow-20260804/flow-05-web-page-loaded.png)

### 4. 终端窗口

Tutti 的终端入口成功拉起 `cmd.exe`，显示 `C:\Users\15514>` 提示符。通过 UI 输入 `echo TUTTI-E2E-TERMINAL-OK` 和 `ver` 时，实际回显为无效输入/命令未识别，未得到期望输出。因此判定为“窗口启动通过，命令执行失败”，需要后续排查终端输入桥接，而不是只验证窗口是否出现。

![终端窗口启动](C:/Work/tutti-os/tutti-windows-e2e-acceptance-20260804/docs/verification/assets/tutti-windows-main-flow-20260804/flow-06-terminal-window-open.png)

![终端命令执行结果](C:/Work/tutti-os/tutti-windows-e2e-acceptance-20260804/docs/verification/assets/tutti-windows-main-flow-20260804/flow-06-terminal-command.png)

### 5. 应用中心：目录、安装和打开

应用中心目录能够展示官方应用；但 AI 幻灯片、AI 文档、AI Canvas、产品原型设计、自动化等可安装应用的“安装”按钮均为 disabled，本轮没有绕过 UI readiness，也没有在未出现用户确认时强行安装。

![应用中心目录与 disabled 安装按钮](C:/Work/tutti-os/tutti-windows-e2e-acceptance-20260804/docs/verification/assets/tutti-windows-main-flow-20260804/flow-07-app-center.png)

已存在的内置“新手指引”可以从应用中心打开，页面加载完成，说明应用打开能力和应用安装能力应拆开看：前者通过，后者阻塞。

![内置新手指引打开完成](C:/Work/tutti-os/tutti-windows-e2e-acceptance-20260804/docs/verification/assets/tutti-windows-main-flow-20260804/flow-07-app-open-result.png)

日志证据：`tuttid.log` 中 remote app catalog fetch completed，返回 `appCount=9`；内置 `tutti-onboarding` 的 materialize/install 记录成功。因此当前更像“远端目录可用，但可安装卡片未进入 enabled 状态”，不是简单的目录请求失败。

### 6. Tutti @互通和任务中心

Agent 互动入口、Big @ 协作说明以及“@ 另一个 Agent，共享会话上下文”入口均可见，但 Tutti Agent 会话创建失败，不能安全地构造一次真实的跨 Agent 协作结果。

![Agent 互动入口](C:/Work/tutti-os/tutti-windows-e2e-acceptance-20260804/docs/verification/assets/tutti-windows-main-flow-20260804/flow-08-agent-interaction-entry.png)

任务中心可以打开并展示全部/待开始/执行中/待验收/已完成/失败等筛选项，当前为空并显示“还没有任务”。这证明任务中心页面可用，但由于 Agent 主流程失败，本轮没有可验收的任务执行结果。

![任务中心空状态](C:/Work/tutti-os/tutti-windows-e2e-acceptance-20260804/docs/verification/assets/tutti-windows-main-flow-20260804/flow-09-task-center.png)

### 7. 失败、权限与窗口管理边界

- 失败边界：Tutti Agent 的错误会显示在会话区；Cursor/OpenCode 的连接状态会显示“需要先连接”，没有卡在无限 loading。
- 权限边界：本轮未自动化 Windows 安全中心、UAC、凭据或权限提升弹窗；未以管理员权限重跑 Tutti。Downloads、Temp 和普通工作区文件的搜索/读取不因权限弹窗而阻塞。
- 窗口管理：Agent、文件、浏览器、终端、应用、任务窗口可以叠加存在；任务窗口可最小化，再通过 dock 恢复。

![多窗口与任务窗口最小化](C:/Work/tutti-os/tutti-windows-e2e-acceptance-20260804/docs/verification/assets/tutti-windows-main-flow-20260804/flow-10-window-minimize.png)

![任务窗口恢复](C:/Work/tutti-os/tutti-windows-e2e-acceptance-20260804/docs/verification/assets/tutti-windows-main-flow-20260804/flow-10-window-restore.png)

## 四、Cursor/OpenCode 连接边界证据

### Cursor

UI 可切换到 Cursor Provider，但状态为“需要先连接 Cursor”。对应 `tuttid.log` 记录：

```text
provider=cursor availability=not_installed reasonCode=cli_not_found
provider=cursor target=cli installerKind=official_script
Unsupported operating system: MSYS_NT-10.0-26200
bash.exe: warning: could not find /tmp, please create!
```

![Cursor 未连接状态](C:/Work/tutti-os/tutti-windows-e2e-acceptance-20260804/docs/verification/assets/tutti-windows-main-flow-20260804/flow-11-provider-cursor-result.png)

### OpenCode

UI 可切换到 OpenCode Provider，但状态为“需要先连接 OpenCode”。对应 `tuttid.log` 记录：

```text
provider=opencode availability=not_installed reasonCode=cli_not_found
provider=opencode target=cli installerKind=official_script
line 80: tr: command not found
bash.exe: warning: could not find /tmp, please create!
```

![OpenCode 未连接状态](C:/Work/tutti-os/tutti-windows-e2e-acceptance-20260804/docs/verification/assets/tutti-windows-main-flow-20260804/flow-11-provider-opencode-result.png)

这两个问题与 Tutti Agent 的 refresh token 问题、文件窗口预览问题是不同故障域，建议后续按 Provider 运行时/安装器、Tutti Agent 认证、文件打开 IPC 三条链路分别跟踪。

## 五、截图索引

截图目录：`C:\Work\tutti-os\tutti-windows-e2e-acceptance-20260804\docs\verification\assets\tutti-windows-main-flow-20260804`

| 截图 | 说明 |
| --- | --- |
| `flow-01-restart-main-window.png` | 重启后的主窗口 |
| `flow-02-tutti-agent-provider.png` | Tutti Agent Provider |
| `flow-03-agent-task-complete.png` | Agent 任务提交后失败 |
| `flow-04-file-select-posm.png` | 选择 POSM 文件 |
| `flow-04-file-search-txt.png` | 搜索 TXT |
| `flow-04-file-read-txt.png` | 读取 TXT 预览 |
| `flow-04-file-open-txt.png` | TXT 打开失败 |
| `flow-04-file-search-img.png` | 搜索 PNG |
| `flow-04-file-read-img.png` | 读取 PNG 预览 |
| `flow-04-file-open-img.png` | PNG 打开失败 |
| `flow-05-web-window-open.png` | 网页窗口打开 |
| `flow-05-web-page-loaded.png` | Example Domain 加载成功 |
| `flow-06-terminal-window-open.png` | 终端窗口启动 |
| `flow-06-terminal-command.png` | 终端命令执行失败 |
| `flow-06-terminal-command-retry.png` | 终端输入复核 |
| `flow-07-app-center.png` | 应用中心与 disabled 安装按钮 |
| `flow-07-app-open-result.png` | 内置新手指引打开完成 |
| `flow-08-agent-interaction-entry.png` | Agent 互动入口 |
| `flow-09-task-center.png` | 任务中心空状态 |
| `flow-10-window-minimize.png` | 窗口最小化 |
| `flow-10-window-restore.png` | 窗口恢复 |
| `flow-11-provider-cursor-result.png` | Cursor 未连接 |
| `flow-11-provider-opencode-result.png` | OpenCode 未连接 |

## 六、后续修复优先级

1. P0：修复 Tutti Agent refresh token/认证 bootstrap 与 model catalog 超时，恢复新建会话和 Agent 任务主链路。
2. P0：修复文件窗口 TXT/PNG “打开”共用错误路径，补充 Windows 路径归一化后的打开/编辑/保存回归。
3. P1：修复终端窗口输入桥接，至少保证 `echo TUTTI-E2E-TERMINAL-OK` 和 `ver` 可执行并回显。
4. P1：修复应用中心安装按钮 disabled 的 readiness 条件，安装后再验证打开和 runtime health check。
5. P1：为 Cursor/OpenCode 提供 Windows 原生安装器或可靠的 Git Bash/MSYS 依赖检查；不要把 `MSYS_NT` 当成 Linux，也不要假设 `tr`、`/tmp` 始终存在。
6. P1：上述链路恢复后，重新串联 FLOW-02 → FLOW-06 → FLOW-08 → FLOW-10，补齐 Agent 结果回传、@互通和任务执行结果截图。

