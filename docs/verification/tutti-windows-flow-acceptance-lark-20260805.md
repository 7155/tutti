# Tutti Windows 主流程 E2E 验收文档

验收日期：2026-08-05  
验收工作区：`C:\Work\tutti-os\tutti-windows-e2e-acceptance-20260804`  
验收基线：`C:\Work\tutti-os\tutti-windows-p0-p1-e2e-core-matrix-final-200.md`

## 验收方式与当前结论

本次按用户主流程串联验收，不把 200 条用例拆成互不关联的单点点击。每个场景记录入口、关键操作、结果、失败证据和截图；未执行的场景标记为 `BLOCKED`，不标记为通过。

当前复验在 FLOW-01 启动/登录阶段被 Windows 顶层 `PickerHost.exe` 防火墙权限窗口阻断，弹窗内容为“是否允许公共网络和专用网络访问此应用？Electron（GitHub, Inc.）”。因此本次不能把后续 Agent 消息、@互通和任务结果标记为通过。历史截图和此前已执行的场景证据仍保留在本报告中，供回归对照。

## 主流程场景矩阵

| 场景 | 串联路径 | 结果 | 截图证据 |
| --- | --- | --- | --- |
| FLOW-01 启动、登录、Agent 主窗口 | 启动 dev → 主窗口 → 登录入口 → Agent 面板 | BLOCKED：顶层 Windows 防火墙权限窗口拦截桌面交互 | 01、02 |
| FLOW-02 Agent 任务 | 新建会话 → 提交只读任务 → 等待回复 → 检查会话结果 | FAIL：历史验收出现 `Tutti Agent failed to start.`；当前复验未越过 FLOW-01 | 03 |
| FLOW-03 文件读取与选择 | 文件窗口 → 选择测试文件 → 读取 TXT → 回传路径/内容 | PASS（历史验收） | 04、05、06 |
| FLOW-04 图片读取 | 文件窗口 → 搜索 PNG → 选择 → 右侧预览 | PASS（历史验收） | 07 |
| FLOW-05 文件打开/编辑 | TXT/PNG → 打开 → 编辑或保存 → 回读 | FAIL（历史验收）：打开子窗口提示“出了点问题，请稍后再试。” | 06、08 |
| FLOW-06 网页窗口 | 打开网页窗口 → 访问 `https://example.com/` → 读取标题/正文 → 回传 Agent | PASS（历史验收：页面读取）；Agent 回传因 FLOW-02 未闭环 | 09 |
| FLOW-07 终端窗口 | 打开内置终端 → 执行 `echo`/`ver` → 检查原文回显和退出码 | FAIL（历史验收）：输入桥接未按原文执行 | 10、11 |
| FLOW-08 应用中心 | 查看目录 → 安装应用 → 健康检查 → 打开应用 | PARTIAL：目录和已安装新手指引可打开；可安装应用按钮 disabled | 12、13 |
| FLOW-09 Tutti @互通 | 主会话 @ 目标 Agent → 提交小任务 → 等待结果 → 回传主会话 | BLOCKED：Agent 主链路未成功，未伪造协作结果 | 14 |
| FLOW-10 任务中心 | 打开任务中心 → 查看待开始/执行中/完成/失败 → 查看结果详情 | PARTIAL：页面和空状态可用；没有可验收的成功执行结果 | 15 |
| FLOW-11 窗口管理 | Agent/文件/网页/终端/应用/任务窗口叠加 → 最小化 → dock 恢复 | PASS（历史验收） | 16、17 |
| FLOW-12 Provider 边界 | Cursor/OpenCode 状态 → 安装/连接失败 → 查看错误反馈 | FAIL（历史验收，错误可识别）；Kimi 已完成运行时和 ACP 检测，登录/消息仍待复验 | 18、19 |
| FLOW-13 权限与安全边界 | 遇到防火墙/UAC/凭据弹窗 → 停止高风险操作 → 记录阻断 | BLOCKED：当前仍存在 Windows 防火墙权限弹窗 | 当前桌面弹窗未保存为本地文件 |

## Agent Provider 专项链路

| Agent | 安装/配置证据 | 连接证据 | 消息验收 |
| --- | --- | --- | --- |
| Cursor | 官方 Windows 安装命令已纳入代码链路；历史日志曾因 MSYS 环境失败 | 待防火墙弹窗解除后刷新状态 | 未完成 |
| OpenCode | CLI、模型目录和自定义 Anthropic 配置已验证 | ACP `initialize`、`session/new` 已返回；真实模型请求曾被网络权限阻断 | 未完成 |
| Hermes | 已生成 provider 配置；运行时安装需要继续走 Tutti 安装链路 | 未完成 | 未完成 |
| Kimi | managed runtime 安装成功；`kimi doctor` 通过；ACP 检测通过 | OAuth 登录未完成；配置文件已改为合法 TOML | 未完成 |

消息验收标准：每个 Agent 必须在 Tutti 中完成一次最小消息，例如“只回复 OK”，并记录发送前、发送中、回复后的截图；只有收到 Agent 回复才算连接成功。

## 失败与修复证据

此前日志/代码分析确认过的关键问题包括：Windows 下 POSIX 单引号命令导致 `.cmd` 启动失败、Hermes/uv Windows `.exe` 路径未归一化、Cursor/OpenCode 安装器运行环境不完整、Tutti Agent 权限模式和 ACP 探测超时。相关修复已保留在验收工作区，使用增量构建启动，未重复执行完整单测。

本次最新阻断不是 Tutti 业务代码失败，而是 Electron 首次联网触发的 Windows 防火墙权限窗口；在该窗口解除前，真实模型消息、应用安装下载和网页回传都不能作为端到端通过证据。

## 截图索引

截图目录：`C:\Work\tutti-os\tutti-windows-e2e-acceptance-20260804\docs\verification\assets\tutti-windows-main-flow-20260804`

1. `flow-01-restart-main-window.png`：重启后的主窗口。
2. `flow-01-start-agent-window.png`：Agent 主窗口入口。
3. `flow-03-agent-task-complete.png`：Tutti Agent 任务失败。
4. `flow-04-file-select-posm.png`：选择测试文件。
5. `flow-04-file-read-txt.png`：读取 TXT 内容。
6. `flow-04-file-open-txt.png`：TXT 打开失败。
7. `flow-04-file-read-img.png`：PNG 预览。
8. `flow-04-file-open-img.png`：PNG 打开失败。
9. `flow-05-web-page-loaded.png`：网页页面加载完成。
10. `flow-06-terminal-command.png`：终端命令执行失败。
11. `flow-06-terminal-command-retry.png`：终端输入复核。
12. `flow-07-app-center.png`：应用中心安装按钮 disabled。
13. `flow-07-app-open-result.png`：内置新手指引打开结果。
14. `flow-08-agent-interaction-entry.png`：Agent 互动和 @ 入口。
15. `flow-09-task-center.png`：任务中心空状态。
16. `flow-10-window-minimize.png`：窗口最小化。
17. `flow-10-window-restore.png`：窗口恢复。
18. `flow-11-provider-cursor-result.png`：Cursor 连接失败状态。
19. `flow-11-provider-opencode-result.png`：OpenCode 连接失败状态。

## 后续验收门槛

解除防火墙权限窗口后，严格按 FLOW-01 → FLOW-02 → FLOW-03 → FLOW-04 → FLOW-05 → FLOW-06 → FLOW-07 → FLOW-08 → FLOW-09 → FLOW-10 → FLOW-11 → FLOW-12 → FLOW-13 继续。若 FLOW-02、FLOW-05、FLOW-07、FLOW-08 或任一 Agent 消息链路失败，先收集 `tuttid.log`、`tutti-desktop.log` 和对应代码链路，修复并重启后再进入下一场景。
