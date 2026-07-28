import type {
  AgentGUIConversationRailQueryRuntimeAdapter,
  ConversationRailQueryRuntime
} from "@tutti-os/agent-gui/conversation-rail-controller";

export function createDesktopAgentGUIConversationRailRuntimeAdapter(
  nodeId: string
): AgentGUIConversationRailQueryRuntimeAdapter {
  return (runtime) =>
    adaptDesktopAgentGUIConversationRailRuntime(runtime, nodeId);
}

function adaptDesktopAgentGUIConversationRailRuntime(
  runtime: ConversationRailQueryRuntime,
  nodeId: string
): ConversationRailQueryRuntime {
  const reportDiagnostic = runtime.reportDiagnostic;
  if (!reportDiagnostic) return runtime;
  const normalizedNodeId = nodeId.trim() || null;
  return {
    ...runtime,
    reportDiagnostic: (input) =>
      reportDiagnostic({
        ...input,
        details: {
          ...input.details,
          nodeId: normalizedNodeId
        }
      })
  };
}
