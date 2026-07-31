---
"@tutti-os/agent-gui": major
---

Make `AgentGUIRuntime` the sole AgentGUI host contract and remove the legacy
`AgentActivityRuntime` interface, Provider, hooks, and test overrides. Desktop
and TSH now use the narrow contract without duplicating lifecycle callbacks
already owned by `AgentSessionEngine`; Mobile was audited and has no dependency
on the removed contract.
