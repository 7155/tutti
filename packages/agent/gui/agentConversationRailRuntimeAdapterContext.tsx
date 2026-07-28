import { createContext, useContext, type ReactNode } from "react";
import type {
  AgentGUIConversationRailQueryRuntimeAdapter,
  ConversationRailQueryRuntime
} from "./agentConversationRailController";

const identityConversationRailRuntime = (
  runtime: ConversationRailQueryRuntime
): ConversationRailQueryRuntime => runtime;

const AgentGUIConversationRailRuntimeAdapterContext =
  createContext<AgentGUIConversationRailQueryRuntimeAdapter>(
    identityConversationRailRuntime
  );

export function AgentGUIConversationRailRuntimeAdapterProvider({
  adapter,
  children
}: {
  adapter?: AgentGUIConversationRailQueryRuntimeAdapter;
  children: ReactNode;
}): React.JSX.Element {
  return (
    <AgentGUIConversationRailRuntimeAdapterContext.Provider
      value={adapter ?? identityConversationRailRuntime}
    >
      {children}
    </AgentGUIConversationRailRuntimeAdapterContext.Provider>
  );
}

export function useAgentGUIConversationRailRuntimeAdapter(): AgentGUIConversationRailQueryRuntimeAdapter {
  return useContext(AgentGUIConversationRailRuntimeAdapterContext);
}
