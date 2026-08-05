# Tutti Windows 沙箱权限修复验收

日期：2026-08-04  适用 worktree：`C:/Work/tutti-os/tutti-windows-e2e-acceptance-20260804`

## 结论

问题是 Windows Codex 沙箱的提权辅助进程启动被取消，不是 Tutti 登录、auth 文件写入或 refresh token 导致的。

原始错误为：

```text
windows sandbox: orchestrator_helper_launch_canceled:
ShellExecuteExW failed to launch setup helper: 1223
```

Windows 错误码 1223 表示操作被取消。该路径会调用 Codex 的 elevated sandbox setup helper；它需要通过 Windows 安全/UAC 辅助进程完成系统级设置。Tutti 的 app-server 不能替用户点击安全桌面上的确认框，因此在弹窗未显示、被取消或无法交互时，命令会在真正执行前失败。

## 最小修复

修复只作用于 Tutti 为单次 Agent 会话复制出的 `CODEX_HOME/config.toml`：Windows 下将 `[windows] sandbox` 固定为 `"unelevated"`。没有修改用户全局 `C:/Users/15514/.codex/config.toml`，也没有关闭 Tutti 的会话权限模式或审批策略。

代码：

- `packages/agent/runtimeprep/codex.go`：会话配置准备阶段写入非提权 Windows 沙箱。
- `packages/agent/runtimeprep/codex_windows_sandbox_test.go`：覆盖替换、追加和幂等行为。

这样仍然使用 Codex 的受限沙箱，只避免每个普通命令都进入需要 UAC 的 elevated helper 路径。

## 证据

### 修复前

- 失败会话的配置：`state/agent/runs/89a6ed16-4230-4a43-88d8-9ab7f6296f35/codex-home/config.toml` 中为 `[windows] sandbox = "elevated"`。
- Tutti 日志中多次出现 `orchestrator_helper_launch_canceled`、`ShellExecuteExW failed`、`1223`。
- 失败发生在 PowerShell 命令执行前；同一条只读命令在重试时可以成功，说明不是命令本身或 auth 文件读写失败。
- 失败期间顶层进程为 `PickerHost.exe`，标题为“Windows 安全中心”，说明确实进入了 Windows 安全/提权相关路径；本次未自动点击该安全窗口。

### 修复后真实链路

- 新建会话 ID：`1a43f514-6166-46e1-b465-2f3ea135ef8d`。
- 实际会话配置：`state/agent/runs/1a43f514-6166-46e1-b465-2f3ea135ef8d/codex-home/config.toml` 中为 `[windows] sandbox = "unelevated"`。
- 实际执行命令：`cmd /c echo tutti-sandbox-e2e-ok`。
- 实际输出：`tutti-sandbox-e2e-ok`。
- Turn 状态：`settled` / `completed`，无错误。
- 19:15 这次验证窗口内没有出现 `1223`、`orchestrator_helper_launch_canceled` 或 `ShellExecuteExW failed`。
- Codex sandbox 日志记录该命令为 `START` 后 `SUCCESS`，未出现 setup helper 失败。

## 自动化验证

```text
go test ./packages/agent/runtimeprep -run '^TestCodexConfigWithTuttiWindowsSandbox' -count=1 -v
PASS: 3 tests
```

直接调用 Codex 的非提权参数也通过：

```text
codex.exe -c windows.sandbox=unelevated sandbox cmd /c echo tutti-windows-sandbox-unelevated-ok
```

结果为退出码 0，输出 `tutti-windows-sandbox-unelevated-ok`，无 UAC 辅助进程错误。

## 备注

日志中早期的 refresh token 401/model cache 错误属于另一条 Codex 登录/模型目录问题；本次 Agent 命令的 1223 失败发生在 Windows 沙箱提权路径，二者不是同一个根因。
