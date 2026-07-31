---
"@tutti-os/agent-gui": minor
---

Expose the narrower `AgentGUIRuntime` contract so hosts can provide AgentGUI
reads, subscriptions, diagnostics, files, and workspace Engine lookup without
duplicating lifecycle callbacks already owned by `AgentSessionEngine`.
Existing `AgentActivityRuntime` consumers remain compatible.
