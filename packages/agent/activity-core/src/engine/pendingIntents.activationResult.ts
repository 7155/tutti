import type { AgentActivitySessionInput } from "../sessionNormalization.ts";
import type { AgentActivityTurn } from "../types.ts";
import type { AgentActivityEditRetryAvailability } from "./editRetry.types.ts";
import type {
  PendingActivationIntentRecord,
  PendingActivationStatus
} from "./pendingIntents.types.ts";
import type { EngineIntent } from "./types.ts";

export type ActivationCommandSettlement =
  | {
      errorCode: string | null;
      errorMessage: string | null;
      kind: "acknowledged";
      projectionIntent: EngineIntent | null;
    }
  | {
      errorCode: string | null;
      errorMessage: string | null;
      kind: "failed";
      projectionIntent: null;
    }
  | {
      errorCode: "invalid_command_result";
      errorMessage: null;
      kind: "invalid";
      projectionIntent: null;
    };

/**
 * Accepts legacy activation acknowledgements without a projection, while
 * validating and extracting the richer result required by typed hosts.
 */
export function validateActivationCommandResult(
  value: unknown,
  record: PendingActivationIntentRecord
): ActivationCommandSettlement {
  if (!value || typeof value !== "object") return invalid();
  const result = value as {
    activation?: { mode?: unknown; status?: unknown };
    detail?: unknown;
    error?: { code?: unknown; message?: unknown } | null;
    session?: unknown;
  };
  const status = result.activation?.status;
  if (typeof status !== "string") return invalid();
  const mode = result.activation?.mode;
  if (mode !== undefined && mode !== record.mode) return invalid();
  if (status === "failed") {
    return {
      errorCode: normalizedString(result.error?.code),
      errorMessage: normalizedString(result.error?.message),
      kind: "failed",
      projectionIntent: null
    };
  }

  const projectionIntent =
    record.mode === "new"
      ? newSessionProjection(result, record)
      : existingSessionProjection(result, record);
  if (projectionIntent === false) return invalid();
  return {
    errorCode: null,
    errorMessage: null,
    kind: "acknowledged",
    projectionIntent
  };
}

function newSessionProjection(
  result: {
    activation?: { mode?: unknown; status?: unknown };
    session?: unknown;
  },
  record: PendingActivationIntentRecord
): EngineIntent | null | false {
  if (
    result.activation?.mode === undefined &&
    result.activation?.status !== "attached"
  ) {
    return null;
  }
  if (result.activation?.status !== "attached") return false;
  if (result.session === undefined) {
    return result.activation?.mode === undefined ? null : false;
  }
  if (!sessionMatches(result.session, record)) return false;
  return {
    session: result.session,
    type: "session/upserted"
  };
}

function existingSessionProjection(
  result: {
    activation?: { mode?: unknown; status?: unknown };
    detail?: unknown;
    session?: unknown;
  },
  record: PendingActivationIntentRecord
): EngineIntent | null | false {
  if (
    result.activation?.mode === undefined &&
    result.activation?.status !== "already_attached"
  ) {
    return null;
  }
  if (result.activation?.status !== "already_attached") return false;
  if (result.detail === undefined) {
    return result.activation?.mode === undefined ? null : false;
  }
  if (
    result.activation?.mode !== undefined &&
    !sessionMatches(result.session, record)
  ) {
    return false;
  }
  if (!result.detail || typeof result.detail !== "object") return false;
  const detail = result.detail as {
    childSessions?: unknown;
    editRetry?: unknown;
    projection?: unknown;
    session?: unknown;
    turns?: unknown;
  };
  const entities =
    Array.isArray(detail.childSessions) && Array.isArray(detail.turns)
      ? scopedDetailEntities(detail.childSessions, detail.turns, record)
      : null;
  if (
    detail.projection !== "authoritative" ||
    !sessionMatches(detail.session, record) ||
    entities === null ||
    (detail.editRetry !== undefined &&
      !isEditRetryAvailability(detail.editRetry))
  ) {
    return false;
  }
  return {
    childSessions: entities.childSessions,
    ...(detail.editRetry === undefined ? {} : { editRetry: detail.editRetry }),
    session: detail.session,
    turns: entities.turns,
    type: "session/detailSnapshotReceived",
    workspaceId: record.workspaceId
  };
}

function sessionMatches(
  value: unknown,
  record: Pick<PendingActivationIntentRecord, "agentSessionId" | "workspaceId">
): value is AgentActivitySessionInput {
  if (!value || typeof value !== "object") return false;
  const session = value as {
    activeTurnId?: unknown;
    agentSessionId?: unknown;
    cwd?: unknown;
    latestTurnInteractions?: unknown;
    pendingInteractions?: unknown;
    provider?: unknown;
    title?: unknown;
    workspaceId?: unknown;
  };
  return (
    typeof session.agentSessionId === "string" &&
    session.agentSessionId.trim() === record.agentSessionId &&
    typeof session.workspaceId === "string" &&
    session.workspaceId.trim() === record.workspaceId &&
    (session.activeTurnId === null ||
      typeof session.activeTurnId === "string") &&
    typeof session.cwd === "string" &&
    isString(session.provider) &&
    typeof session.title === "string" &&
    Array.isArray(session.latestTurnInteractions) &&
    Array.isArray(session.pendingInteractions)
  );
}

function scopedDetailEntities(
  childSessions: readonly unknown[],
  turns: readonly unknown[],
  record: Pick<PendingActivationIntentRecord, "agentSessionId" | "workspaceId">
): {
  childSessions: readonly AgentActivitySessionInput[];
  turns: readonly AgentActivityTurn[];
} | null {
  const sessionIds = new Set([record.agentSessionId]);
  for (const childSession of childSessions) {
    if (!sessionInWorkspace(childSession, record.workspaceId)) return null;
    sessionIds.add(childSession.agentSessionId.trim());
  }
  if (
    !turns.every((turn): turn is AgentActivityTurn =>
      Boolean(
        turn &&
        typeof turn === "object" &&
        typeof (turn as Partial<AgentActivityTurn>).agentSessionId ===
          "string" &&
        sessionIds.has(
          (turn as Partial<AgentActivityTurn>).agentSessionId!.trim()
        ) &&
        typeof (turn as Partial<AgentActivityTurn>).turnId === "string" &&
        Boolean((turn as Partial<AgentActivityTurn>).turnId!.trim())
      )
    )
  ) {
    return null;
  }
  return {
    childSessions: childSessions as readonly AgentActivitySessionInput[],
    turns
  };
}

function isEditRetryAvailability(
  value: unknown
): value is AgentActivityEditRetryAvailability {
  if (!value || typeof value !== "object") return false;
  const availability = value as Partial<AgentActivityEditRetryAvailability>;
  return Boolean(
    typeof availability.supported === "boolean" &&
    typeof availability.eligible === "boolean" &&
    typeof availability.historyRevision === "number" &&
    Number.isFinite(availability.historyRevision) &&
    typeof availability.recoveryState === "string" &&
    Array.isArray(availability.availableActions) &&
    availability.availableActions.every(
      (action) => action === "reconcile" || action === "retry_replacement"
    )
  );
}

function sessionInWorkspace(
  value: unknown,
  workspaceId: string
): value is AgentActivitySessionInput {
  if (!value || typeof value !== "object") return false;
  const session = value as Partial<AgentActivitySessionInput>;
  return (
    typeof session.agentSessionId === "string" &&
    Boolean(session.agentSessionId.trim()) &&
    typeof session.workspaceId === "string" &&
    session.workspaceId.trim() === workspaceId &&
    (session.activeTurnId === null ||
      typeof session.activeTurnId === "string") &&
    typeof session.cwd === "string" &&
    isString(session.provider) &&
    typeof session.title === "string" &&
    Array.isArray(session.latestTurnInteractions) &&
    Array.isArray(session.pendingInteractions)
  );
}

function isString(value: unknown): value is string {
  return typeof value === "string";
}

function normalizedString(value: unknown): string | null {
  return typeof value === "string" ? value.trim() || null : null;
}

function invalid(): ActivationCommandSettlement {
  return {
    errorCode: "invalid_command_result",
    errorMessage: null,
    kind: "invalid",
    projectionIntent: null
  };
}

export function activationCanAcceptCommandResult(
  status: PendingActivationStatus
): boolean {
  return (
    status === "requested" || status === "uncertain" || status === "confirmed"
  );
}
