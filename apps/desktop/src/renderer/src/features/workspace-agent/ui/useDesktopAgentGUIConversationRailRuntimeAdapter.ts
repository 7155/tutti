import { useMemo } from "react";
import { createDesktopAgentGUIConversationRailRuntimeAdapter } from "../services/desktopAgentGUIConversationRailRuntime.ts";

export function useDesktopAgentGUIConversationRailRuntimeAdapter(
  nodeId: string
) {
  return useMemo(
    () => createDesktopAgentGUIConversationRailRuntimeAdapter(nodeId),
    [nodeId]
  );
}
