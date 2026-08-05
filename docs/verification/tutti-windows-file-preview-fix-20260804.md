# Tutti Windows 文件窗口问题分析与修复验收

日期：2026-08-04  
工作区：`C:\Work\tutti-os\tutti-windows-e2e-acceptance-20260804`  
日志包：`C:\Users\15514\Downloads\tutti-logs-last-10-minutes-with-sessions-20260804-205105.zip`

## 结论

本次 TXT/PNG 双击打开失败的主要原因是 Windows 文件路径在 Tutti daemon 与 Electron 本地文件 IPC 之间的格式不一致，不是登录、auth 文件、refresh token 或普通文件权限导致的。

daemon 的工作区文件 API 使用 POSIX 形态承载 Windows 路径，例如：

```text
/C:/Users/15514/Downloads/example.txt
```

文件列表和右侧预览已经经过主进程路径解析，可以把它还原成 `C:/...`；但独立预览窗口的 renderer 节点此前把 `/C:/...` 原样传给 `readLocalPreviewFile`。在 Windows 上，这会让本地 `readFile` 读取失败，因此 TXT 与 PNG 会同时出现“出了点问题”。

## 日志依据

日志包已解压到：`C:\Work\tutti-log-inspect-205105-v2`。

- `runtime-context.json`：确认运行平台为 `win32`，Electron `43.2.0`，状态目录和日志目录都是本次隔离的 Windows E2E state。
- `export-summary.json:4-10`：导出时间为 `2026-08-04T12:51:05.017Z`，覆盖最近 10 分钟；包含 app-center snapshot，但没有 agent session 文件。
- `logs/tuttid.log:1-11,14-26`：C 盘、Downloads、Snipaste 目录以及 Temp 目录均能成功列举和搜索，说明 daemon 的文件发现能力是正常的。
- `logs/tuttid.log:12-13`：仅有一次访问 Windows“开始”菜单目录的 `Access is denied`。这是受 Windows 保护的目录边界场景，不是用户 Downloads/Temp 文件打不开的原因。
- `logs/tutti-desktop.log:2`：有一次文件目录请求返回 `502`，发生在早期打开文件窗口阶段；之后 daemon 继续完成目录列举和搜索，属于启动/请求时序问题，不能解释 TXT/PNG 双击预览统一失败。
- `app-center-snapshot.json:5-6,475-497`：安装 jobs 为空，`tutti-onboarding` 为 active，`lastError` 为 null；应用中心不是本次文件预览根因。

日志包没有记录到独立预览节点的本地 IPC 读取事件，这也是为什么需要结合 renderer 代码与实际 GUI 操作定位路径格式问题。

## 最小修复

修改了以下位置：

1. `apps/desktop/src/renderer/src/features/workspace-workbench/services/workspaceFilePreviewLaunch.ts`

   新增 `normalizeWorkspaceFilePreviewLocalPath()`：

   - `/C:/...` 转为 `C:/...`；
   - `C:\\...` 统一为 `C:/...`；
   - 普通 POSIX 路径保持不变；
   - 不修改权限策略、不修改 auth、不修改 refresh token、不放宽文件访问范围。

2. `apps/desktop/src/renderer/src/features/workspace-workbench/services/internal/workspaceFilePreviewNodeController.ts:140-142`

   仅在调用 `readLocalPreviewFile` 前做路径归一化。工作区文件仍使用原有 Tutti API；因此改动范围只覆盖 Windows 本地文件预览读取链路。

3. `apps/desktop/src/renderer/src/features/workspace-workbench/services/workspaceFilePreviewLaunch.test.ts`

   增加 `/C:/...`、Windows 反斜杠路径和普通 POSIX 路径的回归测试。

## 重启与自动验证

使用隔离 state 的固定启动命令重新启动，没有重新 build：

```powershell
corepack pnpm@10.11.0 run dev:windows:e2e:clean
```

验证结果：

- `tutti-desktop.log:4`：出现 `desktop app ready`；
- `tuttid.log:35`：内置 `tutti-onboarding` 安装成功；
- 文件目录连续列举成功：当前运行日志 `tuttid.log:61,66,68`；
- 文件路径归一化回归测试：8 个测试全部通过；
- `@tutti-os/desktop typecheck`：通过；
- `git diff --check`：通过。

## GUI 验收结果

通过 Computer Use 在重启后的 Tutti Dev 窗口中按“定位 → 右侧预览 → 双击打开”的流程验收：

| 类型 | 验收动作 | 结果 |
| --- | --- | --- |
| TXT（原文件） | 搜索 `splog.txt`，选择并查看右侧文本预览，双击打开 | 通过；内容正常显示。该文件本身显示为只读，因此未修改用户原文件 |
| PNG（用户截图） | 搜索 `codex-clipboard-da11566a-b1d5-4717-8374-0c2b9be6d242.png`，查看缩略图，双击打开 | 通过；右侧缩略图和独立图片窗口均正常显示 |
| TXT（可写夹具） | 创建临时 TXT，打开后修改内容、保存、关闭、刷新并重新读取 | 通过；保存后重新读取到 `after-edit` |

可写 TXT 夹具已在验证完成后清理，没有保留到 Downloads，也没有修改原始 `splog.txt` 或 PNG 文件。

PNG 的内容编辑不属于文件窗口当前能力范围；本次验证覆盖了 PNG 的读取、缩略图预览和独立打开。TXT 已覆盖读取、编辑、保存和重新读取闭环。

## 边界说明

不建议通过给 Tutti 提升权限或关闭 Windows 安全策略来规避本问题。受保护的“开始”菜单目录仍应保留 `Access is denied`，应用应继续允许用户访问正常的 Downloads、Temp 和普通工作区文件。

