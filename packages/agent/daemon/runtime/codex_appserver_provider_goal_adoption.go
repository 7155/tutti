package agentruntime

import (
	"context"
	"log/slog"
	"strings"
)

// adoptProviderGoalGeneration gives a provider-native create_goal call a
// durable Host identity before any server-started continuation can inherit it.
// It does not weaken the existing fail-closed path: unavailable, conflicting,
// or malformed adoption leaves the continuation unproven.
func (a *CodexAppServerAdapter) adoptProviderGoalGeneration(session Session, goal map[string]any) (goalOperationIdentity, bool) {
	fingerprint := codexGoalGenerationFingerprint(goal)
	if a == nil || fingerprint == "" ||
		strings.TrimSpace(asString(goal["status"])) != "active" {
		return goalOperationIdentity{}, false
	}
	agentSessionID := strings.TrimSpace(session.AgentSessionID)
	a.mu.Lock()
	appSession := a.sessions[agentSessionID]
	if appSession == nil || appSession.provenanceDegraded {
		a.mu.Unlock()
		return goalOperationIdentity{}, false
	}
	current := goalOperationIdentity{
		operationID: appSession.goalOperationID,
		revision:    appSession.goalRevision,
		repairEpoch: appSession.goalRepairEpoch,
	}
	if current.valid() {
		a.mu.Unlock()
		return current, false
	}
	sink := a.providerGoalAdoptionSink
	threadID := appSession.threadID
	a.mu.Unlock()
	if sink == nil || strings.TrimSpace(asString(goal["threadId"])) != threadID {
		return goalOperationIdentity{}, false
	}
	session.ProviderSessionID = threadID

	ackCtx, cancel := context.WithTimeout(context.Background(), goalProvenanceDurableAckTimeout)
	binding, err := sink(ackCtx, session, ProviderGoalAdoptionRequest{
		Fingerprint: fingerprint,
		Goal:        normalizedCodexGoal(goal),
	})
	cancel()
	if err != nil {
		slog.Warn("agent session app-server provider Goal adoption failed",
			"event", "agent_session.app_server.goal.provider_adoption_failed",
			"agent_session_id", agentSessionID,
			"error", err.Error(),
		)
		return goalOperationIdentity{}, false
	}
	identity := goalOperationIdentity{
		operationID: strings.TrimSpace(binding.OperationID),
		revision:    binding.Revision,
		repairEpoch: binding.RepairEpoch,
	}
	if binding.Ambiguous || !identity.valid() {
		slog.Warn("agent session app-server provider Goal adoption returned invalid identity",
			"event", "agent_session.app_server.goal.provider_adoption_invalid",
			"agent_session_id", agentSessionID,
		)
		return goalOperationIdentity{}, false
	}
	if !a.installProviderGoalIdentity(agentSessionID, threadID, identity) {
		return goalOperationIdentity{}, false
	}
	if err := a.bindGoalGeneration(context.Background(), session, goal, identity); err != nil {
		slog.Warn("agent session app-server provider Goal provenance binding failed",
			"event", "agent_session.app_server.goal.provider_adoption_binding_failed",
			"agent_session_id", agentSessionID,
			"error", err.Error(),
		)
		return goalOperationIdentity{}, false
	}
	slog.Info("agent session app-server provider Goal adopted",
		"event", "agent_session.app_server.goal.provider_adopted",
		"agent_session_id", agentSessionID,
		"operation_id", identity.operationID,
		"revision", identity.revision,
	)
	return identity, true
}

func (a *CodexAppServerAdapter) installProviderGoalIdentity(agentSessionID, threadID string, identity goalOperationIdentity) bool {
	if a == nil || !identity.valid() {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	appSession := a.sessions[strings.TrimSpace(agentSessionID)]
	if appSession == nil || appSession.provenanceDegraded || appSession.threadID != strings.TrimSpace(threadID) {
		return false
	}
	current := goalOperationIdentity{
		operationID: appSession.goalOperationID,
		revision:    appSession.goalRevision,
		repairEpoch: appSession.goalRepairEpoch,
	}
	switch {
	case current == identity:
		return true
	case current.valid():
		slog.Warn("agent session app-server provider Goal adoption lost identity race",
			"event", "agent_session.app_server.goal.provider_adoption_superseded",
			"agent_session_id", agentSessionID,
			"current_operation_id", current.operationID,
			"adopted_operation_id", identity.operationID,
		)
		return false
	default:
		appSession.goalOperationID = identity.operationID
		appSession.goalRevision = identity.revision
		appSession.goalRepairEpoch = identity.repairEpoch
		return true
	}
}
