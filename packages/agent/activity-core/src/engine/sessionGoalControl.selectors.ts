import { isPendingActivationViable } from "./pendingIntents.types.ts";
import { selectLatestActivationForSession } from "./pendingIntents.selectors.ts";
import {
  projectSessionGoalControl,
  sessionGoalsEqual
} from "./sessionGoalControl.projection.ts";
import type {
  SessionGoalControlOperation,
  SessionGoalControlPresentation,
  SessionGoalControlPublicState,
  SessionGoalControlSettlement
} from "./sessionGoalControl.types.ts";
import type { RootAgentSessionEngineState } from "./rootReducer.types.ts";
import type { AgentSessionEngineState } from "./types.ts";

export function selectInternalSessionGoalControlOperation(
  state: RootAgentSessionEngineState,
  agentSessionId: string | null | undefined
): SessionGoalControlOperation | null {
  const id = agentSessionId?.trim() ?? "";
  return state.goalControl.operationsBySessionId[id] ?? null;
}

export function selectSessionGoalControlSettlement(
  state: AgentSessionEngineState,
  agentSessionId: string | null | undefined
): SessionGoalControlSettlement | null {
  const id = agentSessionId?.trim() ?? "";
  return state.goalControl.settlementsBySessionId[id] ?? null;
}

export function selectSessionGoalControlPresentation(
  state: AgentSessionEngineState,
  agentSessionId: string | null | undefined
): SessionGoalControlPresentation {
  const id = agentSessionId?.trim() ?? "";
  return (
    state.goalControl.presentationsBySessionId[id] ?? {
      agentSessionId: id || null,
      goal: null,
      optimistic: false,
      status: "idle"
    }
  );
}

export function projectPublicSessionGoalControlState(
  state: RootAgentSessionEngineState,
  previous?: SessionGoalControlPublicState
): SessionGoalControlPublicState {
  const agentSessionIds = new Set([
    ...Object.keys(state.sessionLifecycle.sessionsById),
    ...Object.keys(state.goalControl.operationsBySessionId),
    ...Object.values(state.pendingIntents.activationsByRequestId)
      .filter((activation) => activation.initialGoalControl)
      .map((activation) => activation.agentSessionId)
  ]);
  const presentationsBySessionId: Record<
    string,
    SessionGoalControlPresentation
  > = {};
  const settlementsBySessionId: Record<string, SessionGoalControlSettlement> =
    {};
  for (const agentSessionId of agentSessionIds) {
    const presentation = projectSessionGoalControlPresentation(
      state,
      agentSessionId
    );
    const previousPresentation =
      previous?.presentationsBySessionId[agentSessionId];
    presentationsBySessionId[agentSessionId] =
      previousPresentation &&
      sessionGoalControlPresentationsEqual(previousPresentation, presentation)
        ? previousPresentation
        : presentation;
  }
  for (const operation of Object.values(
    state.goalControl.operationsBySessionId
  )) {
    const settlement = projectSessionGoalControlSettlement(operation);
    const previousSettlement =
      previous?.settlementsBySessionId[operation.agentSessionId];
    settlementsBySessionId[operation.agentSessionId] =
      previousSettlement &&
      sessionGoalControlSettlementsEqual(previousSettlement, settlement)
        ? previousSettlement
        : settlement;
  }
  if (
    previous &&
    recordValuesEqual(
      previous.presentationsBySessionId,
      presentationsBySessionId
    ) &&
    recordValuesEqual(previous.settlementsBySessionId, settlementsBySessionId)
  ) {
    return previous;
  }
  return { presentationsBySessionId, settlementsBySessionId };
}

function projectSessionGoalControlPresentation(
  state: RootAgentSessionEngineState,
  id: string
): SessionGoalControlPresentation {
  const canonicalGoal = state.sessionLifecycle.sessionsById[id]?.goal ?? null;
  const operation = state.goalControl.operationsBySessionId[id] ?? null;
  if (
    operation &&
    (operation.status === "pending" ||
      operation.status === "accepted" ||
      operation.status === "unknown")
  ) {
    return {
      agentSessionId: id || null,
      goal: operation.optimisticGoal,
      optimistic:
        operation.status === "pending" ||
        (operation.status === "unknown" && operation.resultState === null),
      status: operation.status
    };
  }
  const activation = selectLatestActivationForSession(state, id);
  if (
    activation?.mode === "new" &&
    activation.initialGoalControl &&
    isPendingActivationViable(activation)
  ) {
    const activationGoal = projectSessionGoalControl(
      canonicalGoal,
      activation.initialGoalControl.action,
      activation.initialGoalControl.objective
    );
    const canonicalSession =
      state.sessionLifecycle.sessionsById[activation.agentSessionId];
    const activationConfirmed = canonicalSession
      ? activationGoal === null
        ? canonicalGoal === null
        : canonicalGoal?.objective === activationGoal.objective
      : false;
    if (activationConfirmed) {
      return {
        agentSessionId: id || null,
        goal: canonicalGoal,
        optimistic: false,
        status: operation?.status ?? "idle"
      };
    }
    return {
      agentSessionId: id || null,
      goal: activationGoal,
      optimistic: true,
      status: "pending_create"
    };
  }
  return {
    agentSessionId: id || null,
    goal: canonicalGoal,
    optimistic: false,
    status: operation?.status ?? "idle"
  };
}

function projectSessionGoalControlSettlement(
  operation: SessionGoalControlOperation
): SessionGoalControlSettlement {
  return {
    action: operation.action,
    agentSessionId: operation.agentSessionId,
    clientSubmitId: operation.clientSubmitId,
    errorCode: operation.errorCode,
    errorMessage: operation.errorMessage,
    errorReason: operation.errorReason,
    status: operation.status
  };
}

export function sessionGoalControlPresentationsEqual(
  left: SessionGoalControlPresentation,
  right: SessionGoalControlPresentation
): boolean {
  return (
    left.agentSessionId === right.agentSessionId &&
    sessionGoalsEqual(left.goal, right.goal) &&
    left.optimistic === right.optimistic &&
    left.status === right.status
  );
}

function sessionGoalControlSettlementsEqual(
  left: SessionGoalControlSettlement,
  right: SessionGoalControlSettlement
): boolean {
  return (
    left.action === right.action &&
    left.agentSessionId === right.agentSessionId &&
    left.clientSubmitId === right.clientSubmitId &&
    left.errorCode === right.errorCode &&
    left.errorMessage === right.errorMessage &&
    left.errorReason === right.errorReason &&
    left.status === right.status
  );
}

function recordValuesEqual<T>(
  left: Readonly<Record<string, T>>,
  right: Readonly<Record<string, T>>
): boolean {
  const leftKeys = Object.keys(left);
  const rightKeys = Object.keys(right);
  return (
    leftKeys.length === rightKeys.length &&
    leftKeys.every((key) => left[key] === right[key])
  );
}
