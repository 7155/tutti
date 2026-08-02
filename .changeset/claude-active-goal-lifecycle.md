---
"@tutti-os/agent-gui": patch
"@tutti-os/claude-sdk-sidecar": patch
---

Consume Claude SDK `active_goal` lifecycle messages, stop inferring Goal completion from ordinary Turn settlement, and settle exact Goal control operations through the Host-owned lifecycle lane instead of transient session runtime context.
