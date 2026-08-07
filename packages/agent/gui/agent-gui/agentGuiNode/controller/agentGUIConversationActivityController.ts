import {
  createAgentGUIConversationActivityActivation,
  reconcileAgentGUIConversationActivityActivation,
  resolveAgentGUIConversationActivityPriorityReason,
  type AgentGUIConversationActivityActivation
} from "../model/agentGuiConversationActivityView";
import type { AgentGUIConversationSummary } from "../model/agentGuiConversationTypes";
import {
  emitConversationActivityDiagnostic,
  type ConversationRailDiagnosticLogger
} from "./agentGuiConversationRailDiagnostics";

const EMPTY_DELETED_SESSION_IDS: Readonly<Record<string, true>> = {};

export interface AgentGUIConversationActivityControllerInput {
  available: boolean;
  conversations: readonly AgentGUIConversationSummary[];
  deletedSessionIds?: Readonly<Record<string, true>>;
  identityKey: string;
  scopeKey: string;
}

export interface AgentGUIConversationActivityControllerSnapshot {
  available: boolean;
  activation: AgentGUIConversationActivityActivation | null;
  conversationCache: ReadonlyMap<string, AgentGUIConversationSummary>;
  enabled: boolean;
  identityKey: string | null;
  scopeKey: string | null;
}

export interface AgentGUIConversationActivityController {
  getSnapshot: () => AgentGUIConversationActivityControllerSnapshot;
  subscribe: (listener: () => void) => () => void;
  configure: (input: AgentGUIConversationActivityControllerInput) => void;
  toggle: () => void;
}

const DISABLED_SNAPSHOT: AgentGUIConversationActivityControllerSnapshot = {
  available: false,
  activation: null,
  conversationCache: new Map(),
  enabled: false,
  identityKey: null,
  scopeKey: null
};
const AVAILABLE_OFF_SNAPSHOT: AgentGUIConversationActivityControllerSnapshot = {
  ...DISABLED_SNAPSHOT,
  available: true
};

export function createAgentGUIConversationActivityController(
  options: {
    diagnosticLogger?: ConversationRailDiagnosticLogger;
    workspaceId?: string;
  } = {}
): AgentGUIConversationActivityController {
  let snapshot = DISABLED_SNAPSHOT;
  let latestInput: AgentGUIConversationActivityControllerInput | null = null;
  const listeners = new Set<() => void>();
  const diagnosticLogger = options.diagnosticLogger;
  const workspaceId = options.workspaceId ?? "";

  const publish = (
    next: AgentGUIConversationActivityControllerSnapshot
  ): void => {
    if (next === snapshot) return;
    snapshot = next;
    for (const listener of listeners) listener();
  };

  const configure = (
    input: AgentGUIConversationActivityControllerInput
  ): void => {
    const previousSnapshot = snapshot;
    latestInput = input;
    if (!input.available) {
      if (snapshot !== DISABLED_SNAPSHOT) publish(DISABLED_SNAPSHOT);
      reportActivityDiagnostic({
        candidates: [],
        deletedSessionIds: input.deletedSessionIds,
        input,
        lateIdleIgnoredCount: 0,
        nextSnapshot: snapshot,
        operation: "configure",
        previousSnapshot,
        sameContext: false
      });
      return;
    }
    if (!snapshot.enabled) {
      if (snapshot !== AVAILABLE_OFF_SNAPSHOT) {
        publish(AVAILABLE_OFF_SNAPSHOT);
      }
      reportActivityDiagnostic({
        candidates: input.conversations.filter(
          (conversation) => !input.deletedSessionIds?.[conversation.id]
        ),
        deletedSessionIds: input.deletedSessionIds,
        input,
        lateIdleIgnoredCount: 0,
        nextSnapshot: snapshot,
        operation: "configure",
        previousSnapshot,
        sameContext: false
      });
      return;
    }

    const sameContext =
      snapshot.identityKey === input.identityKey &&
      snapshot.scopeKey === input.scopeKey;
    const deletedSessionIds =
      input.deletedSessionIds ?? EMPTY_DELETED_SESSION_IDS;
    const candidates = input.conversations.filter(
      (conversation) => !deletedSessionIds[conversation.id]
    );
    const activation =
      sameContext && snapshot.activation
        ? reconcileAgentGUIConversationActivityActivation(
            snapshot.activation,
            candidates,
            deletedSessionIds
          )
        : createAgentGUIConversationActivityActivation(candidates, Date.now());
    const conversationCache = mergeConversationCache(
      sameContext ? snapshot.conversationCache : new Map(),
      candidates,
      deletedSessionIds
    );
    const lateIdleIgnoredCount = countLateIdleCandidates(
      candidates,
      sameContext ? snapshot.activation : null
    );
    if (
      sameContext &&
      activation === snapshot.activation &&
      conversationCache === snapshot.conversationCache
    ) {
      reportActivityDiagnostic({
        candidates,
        deletedSessionIds,
        input,
        lateIdleIgnoredCount,
        nextSnapshot: snapshot,
        operation: "configure",
        previousSnapshot,
        sameContext
      });
      return;
    }
    publish({
      available: true,
      activation,
      conversationCache,
      enabled: true,
      identityKey: input.identityKey,
      scopeKey: input.scopeKey
    });
    reportActivityDiagnostic({
      candidates,
      deletedSessionIds,
      input,
      lateIdleIgnoredCount,
      nextSnapshot: snapshot,
      operation: "configure",
      previousSnapshot,
      sameContext
    });
  };

  const toggle = (): void => {
    const input = latestInput;
    if (!input) return;
    if (!input.available) return;
    const previousSnapshot = snapshot;
    if (snapshot.enabled) {
      publish(AVAILABLE_OFF_SNAPSHOT);
      reportActivityDiagnostic({
        candidates: [],
        deletedSessionIds: input.deletedSessionIds,
        input,
        lateIdleIgnoredCount: 0,
        nextSnapshot: snapshot,
        operation: "toggle_off",
        previousSnapshot,
        sameContext: true
      });
      return;
    }
    const deletedSessionIds =
      input.deletedSessionIds ?? EMPTY_DELETED_SESSION_IDS;
    const candidates = input.conversations.filter(
      (conversation) => !deletedSessionIds[conversation.id]
    );
    publish({
      available: true,
      activation: createAgentGUIConversationActivityActivation(
        candidates,
        Date.now()
      ),
      conversationCache: mergeConversationCache(
        new Map(),
        candidates,
        deletedSessionIds
      ),
      enabled: true,
      identityKey: input.identityKey,
      scopeKey: input.scopeKey
    });
    reportActivityDiagnostic({
      candidates,
      deletedSessionIds,
      input,
      lateIdleIgnoredCount: 0,
      nextSnapshot: snapshot,
      operation: "toggle_on",
      previousSnapshot,
      sameContext: false
    });
  };

  const reportActivityDiagnostic = (input: {
    candidates: readonly AgentGUIConversationSummary[];
    deletedSessionIds?: Readonly<Record<string, true>>;
    input: AgentGUIConversationActivityControllerInput;
    lateIdleIgnoredCount: number;
    nextSnapshot: AgentGUIConversationActivityControllerSnapshot;
    operation: "configure" | "toggle_on" | "toggle_off";
    previousSnapshot: AgentGUIConversationActivityControllerSnapshot;
    sameContext: boolean;
  }): void => {
    if (!diagnosticLogger) return;
    const previousPriorityIds = new Set(
      input.previousSnapshot.activation?.priority.map((member) => member.id)
    );
    const nextPriorityIds = new Set(
      input.nextSnapshot.activation?.priority.map((member) => member.id)
    );
    const priorityAddedCount = [...nextPriorityIds].filter(
      (id) => !previousPriorityIds.has(id)
    ).length;
    const priorityRemovedCount = [...previousPriorityIds].filter(
      (id) => !nextPriorityIds.has(id)
    ).length;
    const priorityRetainedCount = [...nextPriorityIds].filter((id) =>
      previousPriorityIds.has(id)
    ).length;
    const deletedSessionIds = input.deletedSessionIds ?? {};
    const deletedCandidateCount = input.input.conversations.filter(
      (conversation) => deletedSessionIds[conversation.id]
    ).length;
    const candidateStats = summarizeActivityCandidates(input.candidates);
    emitConversationActivityDiagnostic({
      diagnosticLogger,
      payload: {
        activeCandidateCount: candidateStats.activeCandidateCount,
        available: input.nextSnapshot.available,
        candidateCount: input.candidates.length,
        deletedCandidateCount,
        enabled: input.nextSnapshot.enabled,
        idleCandidateCount: candidateStats.idleCandidateCount,
        lateIdleIgnoredCount: input.lateIdleIgnoredCount,
        operation: input.operation,
        priorityAddedCount,
        priorityAfterCount: nextPriorityIds.size,
        priorityBeforeCount: previousPriorityIds.size,
        priorityRemovedCount,
        priorityRetainedCount,
        recentAfterCount: input.nextSnapshot.activation?.recent.length ?? 0,
        recentBeforeCount:
          input.previousSnapshot.activation?.recent.length ?? 0,
        sameContext: input.sameContext,
        unreadCandidateCount: candidateStats.unreadCandidateCount,
        waitingCandidateCount: candidateStats.waitingCandidateCount,
        workspaceId
      }
    });
  };

  return {
    getSnapshot: () => snapshot,
    subscribe: (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    configure,
    toggle
  };
}

function summarizeActivityCandidates(
  conversations: readonly AgentGUIConversationSummary[]
): {
  activeCandidateCount: number;
  idleCandidateCount: number;
  unreadCandidateCount: number;
  waitingCandidateCount: number;
} {
  let activeCandidateCount = 0;
  let idleCandidateCount = 0;
  let unreadCandidateCount = 0;
  let waitingCandidateCount = 0;
  for (const conversation of conversations) {
    const reason =
      resolveAgentGUIConversationActivityPriorityReason(conversation);
    if (reason === "active") activeCandidateCount += 1;
    else if (reason === "unread") unreadCandidateCount += 1;
    else if (reason === "waiting") waitingCandidateCount += 1;
    else idleCandidateCount += 1;
  }
  return {
    activeCandidateCount,
    idleCandidateCount,
    unreadCandidateCount,
    waitingCandidateCount
  };
}

function countLateIdleCandidates(
  conversations: readonly AgentGUIConversationSummary[],
  activation: AgentGUIConversationActivityActivation | null
): number {
  if (!activation) return 0;
  const existingIds = new Set([
    ...activation.priority.map((member) => member.id),
    ...activation.recent.map((member) => member.id)
  ]);
  return conversations.filter(
    (conversation) =>
      !existingIds.has(conversation.id) &&
      resolveAgentGUIConversationActivityPriorityReason(conversation) === null
  ).length;
}

function mergeConversationCache(
  previous: ReadonlyMap<string, AgentGUIConversationSummary>,
  conversations: readonly AgentGUIConversationSummary[],
  deletedSessionIds: Readonly<Record<string, true>>
): ReadonlyMap<string, AgentGUIConversationSummary> {
  const next = new Map(previous);
  for (const deletedId of Object.keys(deletedSessionIds)) {
    next.delete(deletedId);
  }
  for (const conversation of conversations) {
    next.set(conversation.id, conversation);
  }
  if (conversationMapsEqual(previous, next)) return previous;
  return next;
}

function conversationMapsEqual(
  left: ReadonlyMap<string, AgentGUIConversationSummary>,
  right: ReadonlyMap<string, AgentGUIConversationSummary>
): boolean {
  return (
    left.size === right.size &&
    [...left].every(([id, conversation]) => right.get(id) === conversation)
  );
}
