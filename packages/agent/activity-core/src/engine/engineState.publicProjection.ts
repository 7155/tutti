import type { RootAgentSessionEngineState } from "./rootReducer.types.ts";
import { projectPublicSessionGoalControlState } from "./sessionGoalControl.selectors.ts";
import type { AgentSessionEngineState } from "./types.ts";

/**
 * Projects the host-observable Engine state. Keeping this as a real object
 * projection, rather than a type assertion, prevents private reducer ledgers
 * from leaking through getSnapshot() or subscription callbacks at runtime.
 */
export function projectPublicAgentSessionEngineState(
  state: RootAgentSessionEngineState,
  previous?: AgentSessionEngineState,
  previousRoot?: RootAgentSessionEngineState
): AgentSessionEngineState {
  const goalControlUnchanged =
    previous !== undefined &&
    previousRoot !== undefined &&
    state.goalControl === previousRoot.goalControl &&
    state.pendingIntents === previousRoot.pendingIntents &&
    state.sessionLifecycle === previousRoot.sessionLifecycle;
  const projected: AgentSessionEngineState = {
    attentionReadState: state.attentionReadState,
    composerOptions: state.composerOptions,
    editRetry: state.editRetry,
    engineRuntime: state.engineRuntime,
    goalControl: goalControlUnchanged
      ? previous.goalControl
      : projectPublicSessionGoalControlState(state, previous?.goalControl),
    pendingIntents: state.pendingIntents,
    planDecisions: state.planDecisions,
    promptQueue: state.promptQueue,
    sessionCommands: state.sessionCommands,
    sessionLifecycle: state.sessionLifecycle,
    sessionMessages: state.sessionMessages,
    sessionMutations: state.sessionMutations,
    sessionReconcile: state.sessionReconcile,
    tuttiModeActivation: state.tuttiModeActivation
  };
  if (
    previous &&
    Object.keys(projected).every(
      (key) =>
        previous[key as keyof AgentSessionEngineState] ===
        projected[key as keyof AgentSessionEngineState]
    )
  ) {
    return previous;
  }
  return projected;
}
