package agentruntime

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestCodexProviderNativeGoalAdoptionProvesAutomaticContinuation(t *testing.T) {
	t.Parallel()

	adapter, transport, session := startedAppServerAdapter(t)
	adapter.goalProvenanceGraceWindow = 10 * time.Millisecond
	controller := NewController([]Adapter{adapter}, nil)
	adapter.SetGoalProvenanceDurableSink(&memoryGoalProvenanceLedger{
		bindings: make(map[string]GoalProvenanceBinding),
	})
	var (
		adoptionMu sync.Mutex
		requests   []ProviderGoalAdoptionRequest
	)
	controller.SetProviderGoalAdoptionSink(func(
		_ context.Context,
		gotSession Session,
		request ProviderGoalAdoptionRequest,
	) (GoalProvenanceBinding, error) {
		if gotSession.RoomID != session.RoomID ||
			gotSession.AgentSessionID != session.AgentSessionID ||
			gotSession.ProviderSessionID != session.ProviderSessionID {
			t.Errorf("adoption session = %#v", gotSession)
		}
		adoptionMu.Lock()
		requests = append(requests, request)
		adoptionMu.Unlock()
		return GoalProvenanceBinding{OperationID: "provider-goal-operation", Revision: 1}, nil
	})
	goal := map[string]any{
		"threadId": session.ProviderSessionID, "objective": "count from one to five",
		"status": "active", "createdAt": int64(100), "updatedAt": int64(101),
	}

	transport.conn.notify(appServerNotifyThreadGoalUpdated, map[string]any{
		"threadId": session.ProviderSessionID,
		"goal":     goal,
	})
	transport.conn.notify(appServerNotifyTurnStarted, map[string]any{
		"threadId": session.ProviderSessionID,
		"turn": map[string]any{
			"id": "provider-native-goal-turn", "status": "inProgress", "items": []any{},
		},
	})

	waitForCondition(t, func() bool {
		active := adapter.sessionActiveTurn(session.AgentSessionID)
		return active != nil &&
			adapter.sessionActiveTurnID(session.AgentSessionID) == "provider-native-goal-turn" &&
			active.goalIdentity.operationID == "provider-goal-operation" &&
			active.goalIdentity.revision == 1 &&
			active.goalProvenance == "ordered_goal_continuation_claim"
	})
	adoptionMu.Lock()
	if len(requests) != 1 ||
		requests[0].Fingerprint != codexGoalGenerationFingerprint(goal) ||
		asString(requests[0].Goal["objective"]) != "count from one to five" {
		t.Fatalf("adoption requests = %#v", requests)
	}
	adoptionMu.Unlock()
	if interrupts := appServerRequestParamsList(t, transport.conn, appServerMethodTurnInterrupt); len(interrupts) != 0 {
		t.Fatalf("provider-native Goal continuation was interrupted: %#v", interrupts)
	}
}
