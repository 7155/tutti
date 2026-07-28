import type { AgentSessionEngine } from "@tutti-os/agent-activity-core";
import { AgentGUIConversationRailQueryController as InternalAgentGUIConversationRailQueryController } from "./agent-gui/agentGuiNode/controller/AgentGUIConversationRailQueryController.ts";
import type { AgentGUIConversationRailQuerySnapshot } from "./agent-gui/agentGuiNode/controller/agentConversationRailQuerySnapshot.ts";
import type { ConversationRailQueryScope } from "./agent-gui/agentGuiNode/controller/agentGuiConversationRailQueryTypes.ts";
import type { AgentConversationRailRuntimePort } from "./agentConversationRailContracts.ts";
import {
  createWorkspaceQueryCache,
  type WorkspaceQueryCache
} from "./shared/query/workspaceQueryCache.ts";

export type {
  AgentGUIConversationRailQuerySnapshot,
  ConversationRailQueryScope
};

export type ConversationRailQueryRuntime = Pick<
  AgentConversationRailRuntimePort,
  | "listPinnedSessionsPage"
  | "listSessionSectionPage"
  | "listSessionSections"
  | "listSessionsPage"
  | "reportDiagnostic"
>;

export interface AgentGUIConversationRailQueryControllerInput {
  engine: AgentSessionEngine;
  getActiveConversationId(): string | null;
  runtime: ConversationRailQueryRuntime;
  scheduler?: {
    schedule(
      delayMs: number,
      task: () => void
    ): {
      cancel(): void;
    };
  };
  sectionPageSize?: number;
  sectionRefreshLimitMax?: number;
  workspaceId: string;
}

export class AgentGUIConversationRailQueryController {
  private readonly controller: InternalAgentGUIConversationRailQueryController;

  constructor(input: AgentGUIConversationRailQueryControllerInput) {
    this.controller = new InternalAgentGUIConversationRailQueryController(
      input
    );
  }

  attach(): () => void {
    return this.controller.attach();
  }

  configure(scope: ConversationRailQueryScope): void {
    this.controller.configure(scope);
  }

  getSnapshot(): AgentGUIConversationRailQuerySnapshot {
    return this.controller.getSnapshot();
  }

  isInteractionLocked(): boolean {
    return this.controller.isInteractionLocked();
  }

  loadMoreSearchResults(): void {
    this.controller.loadMoreSearchResults();
  }

  loadMoreSectionConversations(section: { id: string }): void {
    this.controller.loadMoreSectionConversations(section);
  }

  refresh(): Promise<void> {
    return this.controller.refresh();
  }

  retrySearchResults(): void {
    this.controller.retrySearchResults();
  }

  setSearchQuery(value: string): void {
    this.controller.setSearchQuery(value);
  }

  subscribe(
    listener: (snapshot: AgentGUIConversationRailQuerySnapshot) => void
  ): () => void {
    return this.controller.subscribe(listener);
  }
}

const AGENT_CONVERSATION_BATCH_DELETION_RUNTIME_METHODS = [
  "deleteSessionsBatch",
  "listSessionSectionDeletionCandidates"
] as const satisfies ReadonlyArray<keyof AgentConversationRailRuntimePort>;

const AGENT_CONVERSATION_RAIL_SOURCE_METHODS = [
  "deleteSessionsBatch",
  "listPinnedSessionsPage",
  "listSessionSectionDeletionCandidates",
  "listSessionSectionPage",
  "listSessionSections",
  "listSessionsPage"
] as const satisfies ReadonlyArray<keyof AgentConversationRailRuntimePort>;

export const AGENT_CONVERSATION_RAIL_RUNTIME_METHODS = [
  "getSessionSectionsQueryCache",
  ...AGENT_CONVERSATION_RAIL_SOURCE_METHODS
] as const satisfies ReadonlyArray<keyof AgentConversationRailRuntimePort>;

type AgentConversationRailSourceMethod =
  (typeof AGENT_CONVERSATION_RAIL_SOURCE_METHODS)[number];
type AgentConversationBatchDeletionRuntimeMethod =
  (typeof AGENT_CONVERSATION_BATCH_DELETION_RUNTIME_METHODS)[number];

export type AgentConversationRailRuntime = Required<
  Pick<
    AgentConversationRailRuntimePort,
    (typeof AGENT_CONVERSATION_RAIL_RUNTIME_METHODS)[number]
  >
>;

export type AgentConversationRailRuntimeSource = Required<
  Pick<AgentConversationRailRuntimePort, AgentConversationRailSourceMethod>
>;

export interface AgentConversationBatchDeletionCapability {
  available: boolean;
  missingMethods: AgentConversationBatchDeletionRuntimeMethod[];
  partial: boolean;
}

export function createAgentConversationRailRuntime(
  source: AgentConversationRailRuntimeSource
): AgentConversationRailRuntime {
  const sessionSectionsQueryCaches = new Map<
    string,
    WorkspaceQueryCache<unknown>
  >();

  return {
    deleteSessionsBatch: (input) => source.deleteSessionsBatch(input),
    getSessionSectionsQueryCache(workspaceId) {
      const key = workspaceId.trim();
      const current = sessionSectionsQueryCaches.get(key);
      if (current) return current;
      const created = createWorkspaceQueryCache<unknown>();
      sessionSectionsQueryCaches.set(key, created);
      return created;
    },
    listPinnedSessionsPage: (input) => source.listPinnedSessionsPage(input),
    listSessionSectionDeletionCandidates: (input) =>
      source.listSessionSectionDeletionCandidates(input),
    listSessionSectionPage: (input) => source.listSessionSectionPage(input),
    listSessionSections: (input) => source.listSessionSections(input),
    listSessionsPage: (input) => source.listSessionsPage(input)
  };
}

export function inspectAgentConversationBatchDeletionCapability(
  runtime: Partial<
    Pick<
      AgentConversationRailRuntimePort,
      AgentConversationBatchDeletionRuntimeMethod
    >
  >
): AgentConversationBatchDeletionCapability {
  const missingMethods =
    AGENT_CONVERSATION_BATCH_DELETION_RUNTIME_METHODS.filter(
      (method) => typeof runtime[method] !== "function"
    );
  return {
    available: missingMethods.length === 0,
    missingMethods: [...missingMethods],
    partial:
      missingMethods.length > 0 &&
      missingMethods.length <
        AGENT_CONVERSATION_BATCH_DELETION_RUNTIME_METHODS.length
  };
}
