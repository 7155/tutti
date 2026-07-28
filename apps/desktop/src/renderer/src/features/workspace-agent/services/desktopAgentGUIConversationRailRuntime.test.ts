import assert from "node:assert/strict";
import test from "node:test";
import { createDesktopAgentGUIConversationRailRuntimeAdapter } from "./desktopAgentGUIConversationRailRuntime.ts";

test("desktop conversation rail adapter adds node identity to diagnostics", () => {
  const diagnostics: unknown[] = [];
  const adaptRuntime =
    createDesktopAgentGUIConversationRailRuntimeAdapter(" desktop-node-1 ");
  const runtime = adaptRuntime({
    reportDiagnostic(input) {
      diagnostics.push(input);
    }
  });

  runtime.reportDiagnostic?.({
    details: {
      phase: "first_pages"
    },
    event: "agent_gui.conversation_rail.first_pages_failed",
    level: "error",
    workspaceId: "workspace-1"
  });

  assert.deepEqual(diagnostics, [
    {
      details: {
        nodeId: "desktop-node-1",
        phase: "first_pages"
      },
      event: "agent_gui.conversation_rail.first_pages_failed",
      level: "error",
      workspaceId: "workspace-1"
    }
  ]);
});
