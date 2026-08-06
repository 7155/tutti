import { useCallback, useMemo, useState } from "react";
import {
  useAgentGUIRuntime,
  type AgentGUIRuntime
} from "../../../agentActivityRuntime";
import type { AgentGUIConversationSummary } from "../model/agentGuiConversationTypes";
import {
  createAgentGUIConversationActivityActivation,
  projectAgentGUIConversationActivity,
  reconcileAgentGUIConversationActivityActivation,
  type AgentGUIConversationActivityActivation
} from "../model/agentGuiConversationActivityView";
import type { AgentGUIConversationActivityRootFact } from "./useAgentGUIConversationRailQuery";

interface AgentGUIConversationActivityViewState {
  activation: AgentGUIConversationActivityActivation | null;
  enabled: boolean;
  identityKey: string | null;
  runtime: AgentGUIRuntime | null;
  scopeKey: string | null;
  conversationCache: ReadonlyMap<string, AgentGUIConversationSummary>;
}

const EMPTY_ACTIVITY_CONVERSATION_CACHE: ReadonlyMap<
  string,
  AgentGUIConversationSummary
> = new Map();

const DISABLED_ACTIVITY_VIEW_STATE: AgentGUIConversationActivityViewState = {
  activation: null,
  conversationCache: EMPTY_ACTIVITY_CONVERSATION_CACHE,
  enabled: false,
  identityKey: null,
  runtime: null,
  scopeKey: null
};

export interface AgentGUIConversationActivityViewController {
  available: boolean;
  conversationsById: ReadonlyMap<string, AgentGUIConversationSummary>;
  enabled: boolean;
  needsAttention: boolean;
  presentationActive: boolean;
  projection: ReturnType<typeof projectAgentGUIConversationActivity> | null;
  toggle: () => void;
}

const EMPTY_ACTIVITY_CONVERSATIONS: readonly AgentGUIConversationSummary[] = [];
const EMPTY_ACTIVITY_CONVERSATIONS_BY_ID: ReadonlyMap<
  string,
  AgentGUIConversationSummary
> = new Map();
const EMPTY_DELETED_SESSION_IDS: Readonly<Record<string, true>> = {};

export function useAgentGUIConversationActivityView({
  conversations,
  hasConversationQuery,
  identityKey = "",
  deletedSessionIds = EMPTY_DELETED_SESSION_IDS,
  rootFacts,
  scopeKey
}: {
  conversations: readonly AgentGUIConversationSummary[];
  hasConversationQuery: boolean;
  identityKey?: string;
  deletedSessionIds?: Readonly<Record<string, true>>;
  rootFacts: ReadonlyMap<string, AgentGUIConversationActivityRootFact>;
  scopeKey: string;
}): AgentGUIConversationActivityViewController {
  const runtime = useAgentGUIRuntime();
  const available = runtime.conversationActivityViewEnabled === true;
  const activityConversations = useMemo(
    () =>
      available
        ? conversations.map((conversation) => {
            const fact = rootFacts.get(conversation.id);
            if (
              !fact ||
              (fact.needsUserAction === Boolean(conversation.needsUserAction) &&
                fact.status === conversation.status)
            ) {
              return conversation;
            }
            return {
              ...conversation,
              needsUserAction: fact.needsUserAction,
              status: fact.status
            };
          })
        : EMPTY_ACTIVITY_CONVERSATIONS,
    [available, conversations, rootFacts]
  );
  const [storedState, setStoredState] =
    useState<AgentGUIConversationActivityViewState>(
      DISABLED_ACTIVITY_VIEW_STATE
    );

  let state = storedState;
  let shouldStoreState = false;
  let conversationCache = state.conversationCache;
  const activityContextChanged =
    state.enabled &&
    (state.identityKey !== identityKey ||
      state.runtime !== runtime ||
      state.scopeKey !== scopeKey);
  if (!available || !state.enabled || activityContextChanged) {
    conversationCache = EMPTY_ACTIVITY_CONVERSATION_CACHE;
  }
  if (!available && state !== DISABLED_ACTIVITY_VIEW_STATE) {
    state = DISABLED_ACTIVITY_VIEW_STATE;
    shouldStoreState = true;
  } else if (available && state.enabled) {
    const sameIdentity =
      state.identityKey === identityKey && state.runtime === runtime;
    const activation =
      sameIdentity && state.scopeKey === scopeKey && state.activation
        ? reconcileAgentGUIConversationActivityActivation(
            state.activation,
            activityConversations,
            deletedSessionIds
          )
        : createAgentGUIConversationActivityActivation(
            activityConversations,
            Date.now(),
            sameIdentity
              ? state.activation?.priorityRetentionRecencyById
              : undefined
          );
    if (
      activation !== state.activation ||
      state.identityKey !== identityKey ||
      state.runtime !== runtime ||
      state.scopeKey !== scopeKey
    ) {
      state = {
        activation,
        conversationCache,
        enabled: true,
        identityKey,
        runtime,
        scopeKey
      };
      shouldStoreState = true;
    }
  }

  const enabled = available && state.enabled;
  if (enabled) {
    const nextConversationCache = new Map(conversationCache);
    for (const deletedId of Object.keys(deletedSessionIds)) {
      nextConversationCache.delete(deletedId);
    }
    for (const conversation of activityConversations) {
      if (deletedSessionIds[conversation.id]) continue;
      nextConversationCache.set(conversation.id, conversation);
    }
    if (
      !activityConversationMapsEqual(conversationCache, nextConversationCache)
    ) {
      conversationCache = nextConversationCache;
    }
  } else {
    conversationCache = EMPTY_ACTIVITY_CONVERSATION_CACHE;
  }
  if (conversationCache !== state.conversationCache) {
    state = { ...state, conversationCache };
    shouldStoreState = true;
  }
  if (shouldStoreState) {
    setStoredState(state);
  }
  const currentConversationsById = useMemo(
    () =>
      available
        ? new Map(
            activityConversations.flatMap((conversation) =>
              deletedSessionIds[conversation.id]
                ? []
                : [[conversation.id, conversation]]
            )
          )
        : EMPTY_ACTIVITY_CONVERSATIONS_BY_ID,
    [activityConversations, available, deletedSessionIds]
  );
  const conversationsById = useMemo(() => {
    if (!available) return EMPTY_ACTIVITY_CONVERSATIONS_BY_ID;
    if (!enabled || !state.activation) return currentConversationsById;
    const result = new Map(currentConversationsById);
    for (const member of [
      ...state.activation.priority,
      ...state.activation.recent
    ]) {
      if (result.has(member.id)) continue;
      const cached = conversationCache.get(member.id);
      if (cached) result.set(member.id, cached);
    }
    return result;
  }, [
    available,
    conversationCache,
    currentConversationsById,
    enabled,
    state.activation
  ]);
  const needsAttention = useMemo(
    () =>
      available &&
      activityConversations.some(
        (conversation) =>
          !deletedSessionIds[conversation.id] &&
          (conversation.needsUserAction || conversation.hasUnreadCompletion)
      ),
    [activityConversations, available, deletedSessionIds]
  );
  const projection = useMemo(
    () =>
      state.activation
        ? projectAgentGUIConversationActivity(state.activation)
        : null,
    [state.activation]
  );
  const toggle = useCallback(() => {
    if (enabled) {
      setStoredState(DISABLED_ACTIVITY_VIEW_STATE);
      return;
    }
    setStoredState({
      activation: createAgentGUIConversationActivityActivation(
        activityConversations.filter(
          (conversation) => !deletedSessionIds[conversation.id]
        ),
        Date.now()
      ),
      conversationCache: EMPTY_ACTIVITY_CONVERSATION_CACHE,
      enabled: true,
      identityKey,
      runtime,
      scopeKey
    });
  }, [
    activityConversations,
    deletedSessionIds,
    enabled,
    identityKey,
    runtime,
    scopeKey
  ]);
  return useMemo(
    () => ({
      available,
      conversationsById,
      enabled,
      needsAttention,
      presentationActive: enabled && !hasConversationQuery,
      projection,
      toggle
    }),
    [
      available,
      conversationsById,
      enabled,
      hasConversationQuery,
      needsAttention,
      projection,
      toggle
    ]
  );
}

function activityConversationMapsEqual(
  left: ReadonlyMap<string, AgentGUIConversationSummary>,
  right: ReadonlyMap<string, AgentGUIConversationSummary>
): boolean {
  return (
    left.size === right.size &&
    [...left].every(([id, conversation]) => {
      return right.get(id) === conversation;
    })
  );
}
