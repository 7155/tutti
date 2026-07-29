package agent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	agenthost "github.com/tutti-os/tutti/packages/agent/host"
	agentactivitybiz "github.com/tutti-os/tutti/services/tuttid/biz/agentactivity"
	agenttargetbiz "github.com/tutti-os/tutti/services/tuttid/biz/agenttarget"
	workspacebiz "github.com/tutti-os/tutti/services/tuttid/biz/workspace"
)

func TestServiceResumesPersistedSessionBeforeInput(t *testing.T) {
	runtime := newFakeRuntime()
	service := newIsolatedAgentService(runtime)
	service.SessionReader = fakeSessionReader{
		sessions: map[string]PersistedSession{
			"ws-1:session-1": {
				ID:                "session-1",
				WorkspaceID:       "ws-1",
				Provider:          "codex",
				ProviderSessionID: "provider-session-1",
				ActiveTurnID:      "turn-1",
				Title:             "Persisted session",
				CreatedAtUnixMS:   1000,
				UpdatedAtUnixMS:   2000,
			},
		},
	}

	result, err := service.SendInput(context.Background(), "ws-1", "session-1", SendInput{Content: TextPromptContent("hello")})
	if err != nil {
		t.Fatalf("SendInput returned error: %v", err)
	}
	session := result.Session
	if session.ID != "session-1" {
		t.Fatalf("session id = %q", session.ID)
	}
	if len(runtime.resumeCalls) != 1 {
		t.Fatalf("resume calls = %d, want 1", len(runtime.resumeCalls))
	}
	if len(runtime.execCalls) != 1 {
		t.Fatalf("exec calls = %d, want 1", len(runtime.execCalls))
	}
}

type recordingGoalStateStore struct {
	prepared     []agentactivitybiz.GoalControlOperationPrepare
	dispatched   []string
	acknowledged []agentactivitybiz.GoalControlOperationAcknowledge
	released     []agentactivitybiz.ReleaseGoalControlOperationInput
}

func (s *recordingGoalStateStore) PrepareGoalControlOperation(_ context.Context, input agentactivitybiz.GoalControlOperationPrepare) (agentactivitybiz.GoalControlOperation, agentactivitybiz.SessionGoalState, bool, error) {
	s.prepared = append(s.prepared, input)
	return agentactivitybiz.GoalControlOperation{
			OperationID: input.OperationID, WorkspaceID: input.WorkspaceID, AgentSessionID: input.AgentSessionID,
			GoalRevision: 1, Action: input.Action, Objective: input.Objective, ClientSubmitID: input.ClientSubmitID,
		}, agentactivitybiz.SessionGoalState{
			WorkspaceID: input.WorkspaceID, AgentSessionID: input.AgentSessionID, Revision: 1,
			PendingOperationID: input.OperationID, SyncStatus: agentactivitybiz.GoalSyncStatusPending,
		}, true, nil
}

func (s *recordingGoalStateStore) MarkGoalControlOperationDispatched(_ context.Context, _ string, operationID string, _ int64) (agentactivitybiz.GoalControlOperation, bool, error) {
	s.dispatched = append(s.dispatched, operationID)
	return agentactivitybiz.GoalControlOperation{OperationID: operationID, GoalRevision: 1}, true, nil
}

func (s *recordingGoalStateStore) AcknowledgeGoalControlOperation(_ context.Context, input agentactivitybiz.GoalControlOperationAcknowledge) (agentactivitybiz.GoalControlOperation, agentactivitybiz.SessionGoalState, bool, error) {
	s.acknowledged = append(s.acknowledged, input)
	return agentactivitybiz.GoalControlOperation{OperationID: input.OperationID, GoalRevision: 1}, agentactivitybiz.SessionGoalState{
		Revision: 1, PendingOperationID: input.OperationID, SyncStatus: agentactivitybiz.GoalSyncStatusApplying,
	}, true, nil
}

func (s *recordingGoalStateStore) GetGoalControlAudit(_ context.Context, workspaceID string, agentSessionID string, operationID string) (agentactivitybiz.Message, bool, error) {
	for index := len(s.prepared) - 1; index >= 0; index-- {
		prepared := s.prepared[index]
		if prepared.WorkspaceID != workspaceID || prepared.AgentSessionID != agentSessionID || prepared.OperationID != operationID {
			continue
		}
		content := "/goal " + prepared.Action
		if prepared.Action == "set" {
			content = "/goal " + prepared.Objective
		}
		return agentactivitybiz.Message{
			AgentSessionID: agentSessionID,
			MessageID:      "goal-control:" + operationID,
			Version:        1,
			Role:           "user",
			Kind:           "session_audit",
			Status:         "completed",
			Payload: map[string]any{
				"action":      prepared.Action,
				"goalControl": true,
				"operationId": operationID,
				"text":        content,
			},
			OccurredAtUnixMS: prepared.OccurredAtUnixMS,
		}, true, nil
	}
	return agentactivitybiz.Message{}, false, nil
}

type recordingGoalAuditPublisher struct {
	audits []agentactivitybiz.Message
}

func (p *recordingGoalAuditPublisher) ObserveCommitted(_ context.Context, delta agenthost.CommittedDelta) error {
	if delta.GoalOperation != nil && delta.GoalOperation.Audit != nil {
		p.audits = append(p.audits, *delta.GoalOperation.Audit)
	}
	return nil
}

func (*recordingGoalStateStore) CompleteGoalControlOperation(context.Context, agentactivitybiz.GoalControlOperationComplete) (agentactivitybiz.GoalControlOperation, agentactivitybiz.SessionGoalState, bool, error) {
	return agentactivitybiz.GoalControlOperation{}, agentactivitybiz.SessionGoalState{}, false, nil
}

func (s *recordingGoalStateStore) GetSessionGoalState(_ context.Context, workspaceID, agentSessionID string) (agentactivitybiz.SessionGoalState, bool, error) {
	if len(s.prepared) == 0 {
		return agentactivitybiz.SessionGoalState{}, false, nil
	}
	return agentactivitybiz.SessionGoalState{
		WorkspaceID: workspaceID, AgentSessionID: agentSessionID, Revision: 1,
		PendingOperationID: s.prepared[len(s.prepared)-1].OperationID,
		SyncStatus:         agentactivitybiz.GoalSyncStatusApplying,
	}, true, nil
}

func (*recordingGoalStateStore) ReconcileSessionGoalObservation(context.Context, agentactivitybiz.GoalObservationReconcile) (agentactivitybiz.SessionGoalState, error) {
	return agentactivitybiz.SessionGoalState{}, nil
}

func (*recordingGoalStateStore) MarkGoalRevisionTerminalIncident(_ context.Context, input agentactivitybiz.GoalTerminalIncidentInput) (agentactivitybiz.SessionGoalState, error) {
	return agentactivitybiz.SessionGoalState{WorkspaceID: input.WorkspaceID, AgentSessionID: input.AgentSessionID, Revision: input.Revision, SyncStatus: agentactivitybiz.GoalSyncStatusUnknown, LastError: input.LastError}, nil
}

func (*recordingGoalStateStore) GetGoalControlOperation(context.Context, string, string) (agentactivitybiz.GoalControlOperation, bool, error) {
	return agentactivitybiz.GoalControlOperation{}, false, nil
}

func (*recordingGoalStateStore) ListClaimableGoalControlOperations(context.Context, agentactivitybiz.ListClaimableGoalControlOperationsInput) ([]agentactivitybiz.GoalControlOperation, error) {
	return nil, nil
}

func (*recordingGoalStateStore) ClaimGoalControlOperation(_ context.Context, input agentactivitybiz.ClaimGoalControlOperationInput) (agentactivitybiz.GoalControlOperation, bool, error) {
	return agentactivitybiz.GoalControlOperation{OperationID: input.OperationID, GoalRevision: 1, LeaseOwner: input.LeaseOwner}, true, nil
}

func (s *recordingGoalStateStore) ReleaseGoalControlOperation(_ context.Context, input agentactivitybiz.ReleaseGoalControlOperationInput) (agentactivitybiz.GoalControlOperation, bool, error) {
	s.released = append(s.released, input)
	return agentactivitybiz.GoalControlOperation{}, true, nil
}

func (*recordingGoalStateStore) RecordGoalControlOperationEvidence(context.Context, agentactivitybiz.GoalControlOperationEvidence) (agentactivitybiz.GoalControlOperation, bool, error) {
	return agentactivitybiz.GoalControlOperation{}, true, nil
}

func (*recordingGoalStateStore) WakeGoalControlOperation(context.Context, agentactivitybiz.WakeGoalControlOperationInput) (agentactivitybiz.GoalControlOperation, bool, error) {
	return agentactivitybiz.GoalControlOperation{}, true, nil
}

func (*recordingGoalStateStore) EnsureGoalRepairOperation(context.Context, agentactivitybiz.EnsureGoalRepairOperationInput) (agentactivitybiz.GoalControlOperation, bool, error) {
	return agentactivitybiz.GoalControlOperation{}, true, nil
}

func (*recordingGoalStateStore) EnsureOrWakeGoalRepairOperation(context.Context, agentactivitybiz.EnsureGoalRepairOperationInput) (agentactivitybiz.GoalControlOperation, agentactivitybiz.SessionGoalState, bool, error) {
	return agentactivitybiz.GoalControlOperation{}, agentactivitybiz.SessionGoalState{WorkspaceID: "ws-typed", AgentSessionID: "session-typed", Revision: 1}, true, nil
}

func (*recordingGoalStateStore) RequeueLeasedGoalControlOperationsOnStartup(context.Context, int64) (int64, error) {
	return 0, nil
}

func TestServiceTypedGoalUsesDurableSagaBeforeTurnSubmit(t *testing.T) {
	runtime := newFakeRuntime()
	runtime.sessions["ws-typed:session-typed"] = ProviderRuntimeSession{
		ID: "session-typed", Provider: "claude-code", ProviderSessionID: "provider-typed", Status: "ready",
	}
	store := &recordingGoalStateStore{}
	publisher := &recordingGoalAuditPublisher{}
	runtime.goalControlHook = func(_ context.Context, input RuntimeGoalControlInput) (RuntimeGoalControlResult, error) {
		if len(publisher.audits) != 1 {
			t.Fatalf("goal audit count before provider dispatch = %d, want 1", len(publisher.audits))
		}
		return RuntimeGoalControlResult{
			Goal:          map[string]any{"objective": input.Objective, "status": "active"},
			ProviderPhase: "accepted",
			Evidence:      map[string]any{"phase": "accepted"},
		}, nil
	}
	service := newIsolatedAgentService(runtime)
	service.GoalStateStore = store
	service.CommitObserver = publisher

	result, err := service.SendInput(context.Background(), "ws-typed", "session-typed", SendInput{
		Content:  TextPromptContent("/goal ship it"),
		Metadata: map[string]any{"clientSubmitId": "submit-goal-1"},
	})
	if err != nil {
		t.Fatalf("typed goal: %v", err)
	}
	if result.Kind != "goalControl" || result.TurnID != "" || result.GoalControl == nil {
		t.Fatalf("typed goal result=%#v", result)
	}
	if len(runtime.execCalls) != 0 {
		t.Fatalf("typed goal entered Turn Exec: %#v", runtime.execCalls)
	}
	if len(store.prepared) != 1 || store.prepared[0].Action != "set" || store.prepared[0].Objective != "ship it" {
		t.Fatalf("prepared operations=%#v", store.prepared)
	}
	if len(runtime.goalControlCalls) != 1 || runtime.goalControlCalls[0].OperationID == "" || runtime.goalControlCalls[0].GoalRevision != 1 {
		t.Fatalf("runtime goal calls=%#v", runtime.goalControlCalls)
	}
	if runtime.goalControlCalls[0].SubmissionMetadata["clientSubmitId"] != "submit-goal-1" {
		t.Fatalf("runtime goal submission metadata=%#v", runtime.goalControlCalls[0].SubmissionMetadata)
	}
	if len(store.acknowledged) != 1 || store.acknowledged[0].OperationID != runtime.goalControlCalls[0].OperationID {
		t.Fatalf("acknowledgements=%#v", store.acknowledged)
	}
	if len(publisher.audits) != 1 || publisher.audits[0].MessageID != "goal-control:"+runtime.goalControlCalls[0].OperationID {
		t.Fatalf("published goal audits=%#v", publisher.audits)
	}
}

func TestGoalSessionMutationLaneOrdersSlowSetBeforeClear(t *testing.T) {
	ctx := context.Background()
	store := openAgentServiceSQLiteStore(t)
	if err := store.Create(ctx, workspacebiz.Summary{ID: "ws-goal-actor", Name: "Goal Actor"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReportSessionState(ctx, agentactivitybiz.SessionStateReport{
		WorkspaceID: "ws-goal-actor", AgentSessionID: "session-goal-actor", Provider: "codex", OccurredAtUnixMS: 10,
	}); err != nil {
		t.Fatal(err)
	}
	runtime := newFakeRuntime()
	runtime.sessions["ws-goal-actor:session-goal-actor"] = ProviderRuntimeSession{
		ID: "session-goal-actor", Provider: "codex", ProviderSessionID: "thread-goal-actor", Status: "ready",
	}
	setStarted := make(chan struct{})
	releaseSet := make(chan struct{})
	var once sync.Once
	runtime.goalControlHook = func(_ context.Context, input RuntimeGoalControlInput) (RuntimeGoalControlResult, error) {
		if input.Action == "set" {
			once.Do(func() { close(setStarted) })
			<-releaseSet
			return RuntimeGoalControlResult{Goal: map[string]any{"objective": input.Objective, "status": "active"}, ProviderPhase: "applied", Evidence: map[string]any{"confidence": "authoritative"}}, nil
		}
		return RuntimeGoalControlResult{ProviderPhase: "applied", Evidence: map[string]any{"confidence": "authoritative"}}, nil
	}
	runtime.goalReconcileHook = func(_ context.Context, input RuntimeGoalControlInput) (RuntimeGoalReconcileResult, error) {
		return RuntimeGoalReconcileResult{AgentSessionID: input.AgentSessionID,
			Goal:     map[string]any{"objective": "ship it", "status": "active"},
			Evidence: map[string]any{"confidence": "authoritative"}}, nil
	}
	service := newIsolatedAgentService(runtime)
	service.GoalStateStore = store
	service.GoalOperationOwner = "goal-worker-test"

	type goalCallResult struct {
		result GoalControlSessionResult
		err    error
	}
	setDone := make(chan goalCallResult, 1)
	go func() {
		result, err := service.GoalControl(ctx, "ws-goal-actor", "session-goal-actor", "set", "ship it")
		setDone <- goalCallResult{result: result, err: err}
	}()
	<-setStarted
	clearDone := make(chan goalCallResult, 1)
	go func() {
		result, err := service.GoalControl(ctx, "ws-goal-actor", "session-goal-actor", "clear", "")
		clearDone <- goalCallResult{result: result, err: err}
	}()
	close(releaseSet)
	setCall := <-setDone
	if setCall.err != nil {
		t.Fatalf("slow set completion: %v", setCall.err)
	}
	clearCall := <-clearDone
	if clearCall.err != nil {
		t.Fatalf("ordered clear: %v", clearCall.err)
	}
	state, found, err := store.GetSessionGoalState(ctx, "ws-goal-actor", "session-goal-actor")
	if err != nil || !found || state.Revision != 2 || !state.Tombstoned || len(state.Desired) != 0 ||
		state.SyncStatus != agentactivitybiz.GoalSyncStatusSynced || state.PendingOperationID != "" {
		t.Fatalf("current state=%#v found=%v error=%v", state, found, err)
	}
	if setCall.result.GoalState == nil || setCall.result.GoalState.Revision != 1 {
		t.Fatalf("set result=%#v, want revision one", setCall.result)
	}
	if clearCall.result.OperationID == "" {
		t.Fatal("clear operation id is empty")
	}
	runtime.mu.Lock()
	calls := append([]RuntimeGoalControlInput(nil), runtime.goalControlCalls...)
	runtime.mu.Unlock()
	if len(calls) != 2 || calls[0].Action != "set" || calls[1].Action != "clear" {
		t.Fatalf("provider calls=%#v, want ordered set then clear", calls)
	}
}

func TestGoalSessionMutationLaneRunsClearAfterAmbiguousSetFailure(t *testing.T) {
	ctx := context.Background()
	store := openAgentServiceSQLiteStore(t)
	if err := store.Create(ctx, workspacebiz.Summary{ID: "ws-goal-error", Name: "Goal Error"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReportSessionState(ctx, agentactivitybiz.SessionStateReport{
		WorkspaceID: "ws-goal-error", AgentSessionID: "session-goal-error", Provider: "claude-code", OccurredAtUnixMS: 10,
	}); err != nil {
		t.Fatal(err)
	}
	runtime := newFakeRuntime()
	runtime.sessions["ws-goal-error:session-goal-error"] = ProviderRuntimeSession{
		ID: "session-goal-error", Provider: "claude-code", ProviderSessionID: "claude-goal-error", Status: "ready",
	}
	setStarted := make(chan struct{})
	releaseSet := make(chan struct{})
	var once sync.Once
	runtime.goalControlHook = func(_ context.Context, input RuntimeGoalControlInput) (RuntimeGoalControlResult, error) {
		if input.Action == "set" {
			once.Do(func() { close(setStarted) })
			<-releaseSet
			return RuntimeGoalControlResult{}, context.DeadlineExceeded
		}
		return RuntimeGoalControlResult{ProviderPhase: "applied", Evidence: map[string]any{"confidence": "authoritative"}}, nil
	}
	service := newIsolatedAgentService(runtime)
	service.GoalStateStore = store
	service.GoalOperationOwner = "goal-error-worker"
	setDone := make(chan error, 1)
	go func() {
		_, err := service.GoalControl(ctx, "ws-goal-error", "session-goal-error", "set", "old")
		setDone <- err
	}()
	<-setStarted
	close(releaseSet)
	if err := <-setDone; err == nil {
		t.Fatal("ambiguous old provider result unexpectedly succeeded")
	}
	if _, err := service.GoalControl(ctx, "ws-goal-error", "session-goal-error", "clear", ""); err != nil {
		t.Fatalf("clear after failed set: %v", err)
	}
	state, found, err := store.GetSessionGoalState(ctx, "ws-goal-error", "session-goal-error")
	if err != nil || !found || state.Revision != 2 || state.PendingOperationID != "" || state.SyncStatus != agentactivitybiz.GoalSyncStatusSynced || !state.Tombstoned {
		t.Fatalf("cleared state=%#v found=%v err=%v", state, found, err)
	}
	runtime.mu.Lock()
	calls := append([]RuntimeGoalControlInput(nil), runtime.goalControlCalls...)
	runtime.mu.Unlock()
	if len(calls) != 2 || calls[0].Action != "set" || calls[1].Action != "clear" {
		t.Fatalf("provider calls=%#v, want set then clear", calls)
	}
}

func TestGoalRecoveryDoesNotReplayAcceptedClaudeSet(t *testing.T) {
	ctx := context.Background()
	store := openAgentServiceSQLiteStore(t)
	if err := store.Create(ctx, workspacebiz.Summary{ID: "ws-goal-recovery", Name: "Goal Recovery"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReportSessionState(ctx, agentactivitybiz.SessionStateReport{
		WorkspaceID: "ws-goal-recovery", AgentSessionID: "session-goal-recovery", Provider: "claude-code", OccurredAtUnixMS: 10,
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := store.PrepareGoalControlOperation(ctx, agentactivitybiz.GoalControlOperationPrepare{
		OperationID: "goal-unsafe-set", WorkspaceID: "ws-goal-recovery", AgentSessionID: "session-goal-recovery",
		Action: "set", Objective: "ship it", OccurredAtUnixMS: 20,
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.MarkGoalControlOperationDispatched(ctx, "ws-goal-recovery", "goal-unsafe-set", 21); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := store.AcknowledgeGoalControlOperation(ctx, agentactivitybiz.GoalControlOperationAcknowledge{
		WorkspaceID: "ws-goal-recovery", OperationID: "goal-unsafe-set",
		Evidence: map[string]any{"phase": "accepted"}, OccurredAtUnixMS: 22,
	}); err != nil {
		t.Fatal(err)
	}
	runtime := newFakeRuntime()
	runtime.goalRecoveryPolicyHook = func(context.Context, RuntimeGoalControlInput) (RuntimeGoalRecoveryPolicy, error) {
		return RuntimeGoalRecoveryPolicy{QuerySupported: false, ReplaySetAfterRestart: false}, nil
	}
	runtime.sessions["ws-goal-recovery:session-goal-recovery"] = ProviderRuntimeSession{
		ID: "session-goal-recovery", Provider: "claude-code", ProviderSessionID: "claude-session", Status: "ready",
	}
	service := newIsolatedAgentService(runtime)
	service.GoalStateStore = store
	service.GoalOperationOwner = "goal-recovery-worker"
	service.GoalOperationClock = func() time.Time { return time.UnixMilli(6000) }
	if err := service.ApplicationHost().StepGoalOperationWorker(ctx, true); err != nil {
		t.Fatalf("recover accepted Claude set: %v", err)
	}
	if len(runtime.goalControlCalls) != 0 {
		t.Fatalf("unsafe Claude set replayed: %#v", runtime.goalControlCalls)
	}
	op, found, err := store.GetGoalControlOperation(ctx, "ws-goal-recovery", "goal-unsafe-set")
	if err != nil || !found || op.Status != "failed" {
		t.Fatalf("operation=%#v found=%v error=%v", op, found, err)
	}
	state, found, err := store.GetSessionGoalState(ctx, "ws-goal-recovery", "session-goal-recovery")
	if err != nil || !found || state.SyncStatus != agentactivitybiz.GoalSyncStatusFailed || state.PendingOperationID != "" {
		t.Fatalf("state=%#v found=%v error=%v", state, found, err)
	}
}

func TestGoalRecoveryPolicyIsCapabilityDrivenAndMissingResolverFailsClosed(t *testing.T) {
	runtime := newFakeRuntime()
	runtime.goalRecoveryPolicyHook = func(context.Context, RuntimeGoalControlInput) (RuntimeGoalRecoveryPolicy, error) {
		return RuntimeGoalRecoveryPolicy{QuerySupported: true, ReplaySetAfterRestart: true}, nil
	}
	policy, err := agenthost.ResolveRuntimeGoalRecoveryPolicy(context.Background(), runtime, RuntimeGoalControlInput{})
	if err != nil || !policy.QuerySupported || !policy.ReplaySetAfterRestart {
		t.Fatalf("adapter policy=%#v err=%v", policy, err)
	}
	// Embedding only the mandatory controller contract deliberately hides the
	// optional resolver, modeling an unknown/older provider adapter.
	unknown := struct{ RuntimeController }{RuntimeController: runtime}
	policy, err = agenthost.ResolveRuntimeGoalRecoveryPolicy(context.Background(), unknown, RuntimeGoalControlInput{})
	if err != nil || policy.QuerySupported || policy.ReplaySetAfterRestart {
		t.Fatalf("missing-resolver policy must fail closed: %#v err=%v", policy, err)
	}
}

func TestGoalRecoveryTimeoutThenRestartDoesNotReplayClaudeSet(t *testing.T) {
	ctx := context.Background()
	store := openAgentServiceSQLiteStore(t)
	if err := store.Create(ctx, workspacebiz.Summary{ID: "ws-goal-timeout", Name: "Goal Timeout"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReportSessionState(ctx, agentactivitybiz.SessionStateReport{
		WorkspaceID: "ws-goal-timeout", AgentSessionID: "session-goal-timeout", Provider: "claude-code", OccurredAtUnixMS: 10,
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := store.PrepareGoalControlOperation(ctx, agentactivitybiz.GoalControlOperationPrepare{
		OperationID: "goal-timeout-set", WorkspaceID: "ws-goal-timeout", AgentSessionID: "session-goal-timeout",
		Action: "set", Objective: "ship it", OccurredAtUnixMS: 20,
	}); err != nil {
		t.Fatal(err)
	}
	runtime := newFakeRuntime()
	runtime.goalRecoveryPolicyHook = func(context.Context, RuntimeGoalControlInput) (RuntimeGoalRecoveryPolicy, error) {
		return RuntimeGoalRecoveryPolicy{QuerySupported: false, ReplaySetAfterRestart: false}, nil
	}
	runtime.sessions["ws-goal-timeout:session-goal-timeout"] = ProviderRuntimeSession{
		ID: "session-goal-timeout", Provider: "claude-code", ProviderSessionID: "claude-timeout", Status: "ready",
	}
	runtime.goalControlHook = func(context.Context, RuntimeGoalControlInput) (RuntimeGoalControlResult, error) {
		return RuntimeGoalControlResult{}, context.DeadlineExceeded
	}
	nowMS := int64(30)
	service := newIsolatedAgentService(runtime)
	service.GoalStateStore = store
	service.GoalOperationOwner = "goal-timeout-worker"
	service.GoalOperationClock = func() time.Time { return time.UnixMilli(nowMS) }
	service.GoalOperationMaxAttempts = 1
	if err := service.ApplicationHost().StepGoalOperationWorker(ctx, false); err != nil {
		t.Fatalf("timeout attempt: %v", err)
	}
	op, found, err := store.GetGoalControlOperation(ctx, "ws-goal-timeout", "goal-timeout-set")
	if err != nil || !found || op.Status != agentactivitybiz.GoalOperationStatusDispatched ||
		op.ProviderPhase != agentactivitybiz.GoalProviderPhaseDispatched || op.LeaseOwner != "" || op.NextAttemptAtMS <= nowMS {
		t.Fatalf("timed out operation=%#v found=%v err=%v", op, found, err)
	}
	nowMS = op.NextAttemptAtMS
	if err := service.ApplicationHost().StepGoalOperationWorker(ctx, false); err != nil {
		t.Fatalf("startup recovery: %v", err)
	}
	op, found, err = store.GetGoalControlOperation(ctx, "ws-goal-timeout", "goal-timeout-set")
	if err != nil || !found || op.Status != agentactivitybiz.GoalOperationStatusFailed || op.LeaseOwner != "" {
		t.Fatalf("startup operation=%#v found=%v err=%v", op, found, err)
	}
	runtime.mu.Lock()
	callCount := len(runtime.goalControlCalls)
	runtime.mu.Unlock()
	if callCount != 1 {
		t.Fatalf("Claude set provider calls=%d, want exactly one", callCount)
	}
}

func TestGoalRepairSetTimeoutThenRestartDoesNotReplayClaudeSet(t *testing.T) {
	ctx := context.Background()
	store := openAgentServiceSQLiteStore(t)
	if err := store.Create(ctx, workspacebiz.Summary{ID: "ws-repair-set-timeout", Name: "Repair Set Timeout"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReportSessionState(ctx, agentactivitybiz.SessionStateReport{
		WorkspaceID: "ws-repair-set-timeout", AgentSessionID: "session-repair-set-timeout", Provider: "claude-code", OccurredAtUnixMS: 10,
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := store.PrepareGoalControlOperation(ctx, agentactivitybiz.GoalControlOperationPrepare{
		OperationID: "goal-original-set", WorkspaceID: "ws-repair-set-timeout", AgentSessionID: "session-repair-set-timeout",
		Action: "set", Objective: "ship it", OccurredAtUnixMS: 20,
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := store.CompleteGoalControlOperation(ctx, agentactivitybiz.GoalControlOperationComplete{
		WorkspaceID: "ws-repair-set-timeout", OperationID: "goal-original-set", Succeeded: true,
		Observed: map[string]any{"objective": "ship it", "status": "active"}, OccurredAtUnixMS: 21,
	}); err != nil {
		t.Fatal(err)
	}
	repair, _, created, err := store.EnsureOrWakeGoalRepairOperation(ctx, agentactivitybiz.EnsureGoalRepairOperationInput{
		WorkspaceID: "ws-repair-set-timeout", AgentSessionID: "session-repair-set-timeout",
		SourceOperationID: "stale-old-operation", SourceRevision: 0, CurrentRevision: 1, OccurredAtUnixMS: 30,
	})
	if err != nil || !created || repair.Action != "set" {
		t.Fatalf("repair=%#v created=%v err=%v", repair, created, err)
	}
	runtime := newFakeRuntime()
	runtime.goalRecoveryPolicyHook = func(context.Context, RuntimeGoalControlInput) (RuntimeGoalRecoveryPolicy, error) {
		return RuntimeGoalRecoveryPolicy{QuerySupported: false, ReplaySetAfterRestart: false}, nil
	}
	runtime.sessions["ws-repair-set-timeout:session-repair-set-timeout"] = ProviderRuntimeSession{
		ID: "session-repair-set-timeout", Provider: "claude-code", ProviderSessionID: "claude-repair-timeout", Status: "ready",
	}
	runtime.goalControlHook = func(context.Context, RuntimeGoalControlInput) (RuntimeGoalControlResult, error) {
		return RuntimeGoalControlResult{}, context.DeadlineExceeded
	}
	nowMS := int64(30)
	service := newIsolatedAgentService(runtime)
	service.GoalStateStore = store
	service.GoalOperationOwner = "goal-repair-timeout-worker"
	service.GoalOperationClock = func() time.Time { return time.UnixMilli(nowMS) }
	service.GoalOperationMaxAttempts = 1
	if err := service.ApplicationHost().StepGoalOperationWorker(ctx, false); err != nil {
		t.Fatal(err)
	}
	repair, found, err := store.GetGoalControlOperation(ctx, "ws-repair-set-timeout", repair.OperationID)
	if err != nil || !found || repair.ProviderPhase != agentactivitybiz.GoalProviderPhaseDispatched || repair.NextAttemptAtMS <= nowMS {
		t.Fatalf("timed out repair=%#v found=%v err=%v", repair, found, err)
	}
	nowMS = repair.NextAttemptAtMS
	if err := service.ApplicationHost().StepGoalOperationWorker(ctx, false); err != nil {
		t.Fatal(err)
	}
	repair, found, err = store.GetGoalControlOperation(ctx, "ws-repair-set-timeout", repair.OperationID)
	if err != nil || !found || repair.Status != agentactivitybiz.GoalOperationStatusFailed {
		t.Fatalf("startup repair=%#v found=%v err=%v", repair, found, err)
	}
	runtime.mu.Lock()
	callCount := len(runtime.goalControlCalls)
	runtime.mu.Unlock()
	if callCount != 1 {
		t.Fatalf("repair set provider calls=%d, want exactly one", callCount)
	}
}

func TestGoalClearRepeatedTimeoutEventuallyFails(t *testing.T) {
	ctx := context.Background()
	store := openAgentServiceSQLiteStore(t)
	if err := store.Create(ctx, workspacebiz.Summary{ID: "ws-clear-timeout", Name: "Clear timeout"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReportSessionState(ctx, agentactivitybiz.SessionStateReport{WorkspaceID: "ws-clear-timeout", AgentSessionID: "s", Provider: "claude-code", OccurredAtUnixMS: 10}); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := store.PrepareGoalControlOperation(ctx, agentactivitybiz.GoalControlOperationPrepare{OperationID: "clear-timeout", WorkspaceID: "ws-clear-timeout", AgentSessionID: "s", Action: "clear", OccurredAtUnixMS: 20}); err != nil {
		t.Fatal(err)
	}
	runtime := newFakeRuntime()
	runtime.sessions["ws-clear-timeout:s"] = ProviderRuntimeSession{ID: "s", Provider: "claude-code", Status: "ready"}
	runtime.goalControlHook = func(context.Context, RuntimeGoalControlInput) (RuntimeGoalControlResult, error) {
		return RuntimeGoalControlResult{}, context.DeadlineExceeded
	}
	now := int64(30)
	service := newIsolatedAgentService(runtime)
	service.GoalStateStore = store
	service.GoalOperationClock = func() time.Time { return time.UnixMilli(now) }
	service.GoalOperationMaxAttempts = 1
	if err := service.ApplicationHost().StepGoalOperationWorker(ctx, false); err != nil {
		t.Fatal(err)
	}
	op, _, _ := store.GetGoalControlOperation(ctx, "ws-clear-timeout", "clear-timeout")
	now = op.NextAttemptAtMS
	if err := service.ApplicationHost().StepGoalOperationWorker(ctx, false); err != nil {
		t.Fatal(err)
	}
	op, _, _ = store.GetGoalControlOperation(ctx, "ws-clear-timeout", "clear-timeout")
	if op.Status != agentactivitybiz.GoalOperationStatusFailed {
		t.Fatalf("clear remained nonterminal: %#v", op)
	}
}

func TestGoalRuntimeUnavailableKeepsPreparedUntilFirstProviderDispatch(t *testing.T) {
	ctx := context.Background()
	store := openAgentServiceSQLiteStore(t)
	if err := store.Create(ctx, workspacebiz.Summary{ID: "ws-runtime-late", Name: "Runtime late"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReportSessionState(ctx, agentactivitybiz.SessionStateReport{WorkspaceID: "ws-runtime-late", AgentSessionID: "s", Provider: "claude-code", OccurredAtUnixMS: 10}); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := store.PrepareGoalControlOperation(ctx, agentactivitybiz.GoalControlOperationPrepare{OperationID: "late", WorkspaceID: "ws-runtime-late", AgentSessionID: "s", Action: "clear", ClientSubmitID: "submit-late-goal", OccurredAtUnixMS: 20}); err != nil {
		t.Fatal(err)
	}
	runtime := newFakeRuntime()
	runtime.goalControlHook = func(context.Context, RuntimeGoalControlInput) (RuntimeGoalControlResult, error) {
		return RuntimeGoalControlResult{}, context.DeadlineExceeded
	}
	now := int64(20)
	service := newIsolatedAgentService(runtime)
	service.GoalStateStore = store
	service.GoalOperationClock = func() time.Time { return time.UnixMilli(now) }
	service.GoalOperationMaxAttempts = 1
	if err := service.ApplicationHost().StepGoalOperationWorker(ctx, false); err != nil {
		t.Fatal(err)
	}
	op, _, _ := store.GetGoalControlOperation(ctx, "ws-runtime-late", "late")
	if op.Status != agentactivitybiz.GoalOperationStatusPrepared || op.ProviderPhase != agentactivitybiz.GoalProviderPhasePrepared || op.FirstDispatchedAtUnixMS != 0 || op.LeaseOwner != "" || op.NextAttemptAtMS <= now || len(runtime.goalControlCalls) != 0 {
		t.Fatalf("unavailable runtime advanced delivery: %#v calls=%#v", op, runtime.goalControlCalls)
	}
	runtime.sessions["ws-runtime-late:s"] = ProviderRuntimeSession{ID: "s", Provider: "claude-code", Status: "ready"}
	now = op.NextAttemptAtMS
	if err := service.ApplicationHost().StepGoalOperationWorker(ctx, false); err != nil {
		t.Fatal(err)
	}
	op, _, _ = store.GetGoalControlOperation(ctx, "ws-runtime-late", "late")
	if op.Status != agentactivitybiz.GoalOperationStatusDispatched || op.FirstDispatchedAtUnixMS != now || len(runtime.goalControlCalls) != 1 {
		t.Fatalf("first dispatch=%#v calls=%#v", op, runtime.goalControlCalls)
	}
	if runtime.goalControlCalls[0].SubmissionMetadata["clientSubmitId"] != "submit-late-goal" {
		t.Fatalf("recovered submission metadata=%#v", runtime.goalControlCalls[0].SubmissionMetadata)
	}
	now = op.NextAttemptAtMS
	if err := service.ApplicationHost().StepGoalOperationWorker(ctx, false); err != nil {
		t.Fatal(err)
	}
	op, _, _ = store.GetGoalControlOperation(ctx, "ws-runtime-late", "late")
	if op.Status != agentactivitybiz.GoalOperationStatusFailed || len(runtime.goalControlCalls) != 1 {
		t.Fatalf("budget terminal=%#v calls=%#v", op, runtime.goalControlCalls)
	}
}

func TestGoalRecoveryProviderQueryAndApplyTimeoutReleaseLease(t *testing.T) {
	for _, phase := range []string{"query", "apply"} {
		t.Run(phase, func(t *testing.T) {
			ctx := context.Background()
			workerCtx, expire := newControlledDeadlineContext()
			store := openAgentServiceSQLiteStore(t)
			workspaceID := "ws-goal-hang-" + phase
			sessionID := "session-goal-hang-" + phase
			operationID := "goal-hang-" + phase
			if err := store.Create(ctx, workspacebiz.Summary{ID: workspaceID, Name: "Goal Hang"}); err != nil {
				t.Fatal(err)
			}
			if _, err := store.ReportSessionState(ctx, agentactivitybiz.SessionStateReport{
				WorkspaceID: workspaceID, AgentSessionID: sessionID, Provider: "codex", OccurredAtUnixMS: 10,
			}); err != nil {
				t.Fatal(err)
			}
			if _, _, _, err := store.PrepareGoalControlOperation(ctx, agentactivitybiz.GoalControlOperationPrepare{
				OperationID: operationID, WorkspaceID: workspaceID, AgentSessionID: sessionID,
				Action: "clear", OccurredAtUnixMS: 20,
			}); err != nil {
				t.Fatal(err)
			}
			runtime := newFakeRuntime()
			runtime.sessions[workspaceID+":"+sessionID] = ProviderRuntimeSession{
				ID: sessionID, Provider: "codex", ProviderSessionID: "codex-hang", Status: "ready",
			}
			if phase == "query" {
				runtime.goalReconcileHook = func(ctx context.Context, _ RuntimeGoalControlInput) (RuntimeGoalReconcileResult, error) {
					expire()
					<-ctx.Done()
					return RuntimeGoalReconcileResult{}, ctx.Err()
				}
			} else {
				runtime.goalReconcileHook = func(_ context.Context, _ RuntimeGoalControlInput) (RuntimeGoalReconcileResult, error) {
					return RuntimeGoalReconcileResult{Evidence: map[string]any{"confidence": "unknown"}}, nil
				}
			}
			runtime.goalControlHook = func(ctx context.Context, _ RuntimeGoalControlInput) (RuntimeGoalControlResult, error) {
				expire()
				<-ctx.Done()
				return RuntimeGoalControlResult{}, ctx.Err()
			}
			service := newIsolatedAgentService(runtime)
			service.GoalStateStore = store
			service.GoalOperationOwner = "goal-hang-worker"
			nowMS := int64(30)
			service.GoalOperationClock = func() time.Time { return time.UnixMilli(nowMS) }
			service.GoalOperationMaxAttempts = 1
			if err := service.ApplicationHost().StepGoalOperationWorker(workerCtx, false); err != nil {
				t.Fatalf("worker timeout: %v", err)
			}
			op, found, err := store.GetGoalControlOperation(ctx, workspaceID, operationID)
			expectedPhase := agentactivitybiz.GoalProviderPhaseDispatched
			if err != nil || !found || op.LeaseOwner != "" || op.NextAttemptAtMS <= 30 ||
				op.ProviderPhase != expectedPhase {
				t.Fatalf("released operation=%#v found=%v err=%v", op, found, err)
			}
			if phase == "apply" {
				nowMS = op.NextAttemptAtMS
				if err := service.ApplicationHost().StepGoalOperationWorker(ctx, false); err != nil {
					t.Fatal(err)
				}
				op, _, _ = store.GetGoalControlOperation(ctx, workspaceID, operationID)
				if op.Status != agentactivitybiz.GoalOperationStatusFailed {
					t.Fatalf("codex retry did not terminate: %#v", op)
				}
			}
		})
	}
}

func TestGoalRecoveryStartupBudgetBoundsHangingProvider(t *testing.T) {
	ctx := context.Background()
	store := openAgentServiceSQLiteStore(t)
	if err := store.Create(ctx, workspacebiz.Summary{ID: "ws-goal-startup-budget", Name: "Startup Budget"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReportSessionState(ctx, agentactivitybiz.SessionStateReport{
		WorkspaceID: "ws-goal-startup-budget", AgentSessionID: "session-goal-startup-budget", Provider: "codex", OccurredAtUnixMS: 10,
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := store.PrepareGoalControlOperation(ctx, agentactivitybiz.GoalControlOperationPrepare{
		OperationID: "goal-startup-budget", WorkspaceID: "ws-goal-startup-budget", AgentSessionID: "session-goal-startup-budget",
		Action: "clear", OccurredAtUnixMS: 20,
	}); err != nil {
		t.Fatal(err)
	}
	runtime := newFakeRuntime()
	runtime.sessions["ws-goal-startup-budget:session-goal-startup-budget"] = ProviderRuntimeSession{
		ID: "session-goal-startup-budget", Provider: "codex", ProviderSessionID: "codex-startup-budget", Status: "ready",
	}
	runtime.goalReconcileHook = func(ctx context.Context, _ RuntimeGoalControlInput) (RuntimeGoalReconcileResult, error) {
		<-ctx.Done()
		return RuntimeGoalReconcileResult{}, ctx.Err()
	}
	service := newIsolatedAgentService(runtime)
	service.GoalStateStore = store
	service.GoalOperationOwner = "goal-startup-budget-worker"
	service.GoalOperationClock = func() time.Time { return time.UnixMilli(30) }
	service.GoalOperationAttemptTimeout = time.Second
	service.GoalOperationRecoveryBudget = 250 * time.Millisecond
	started := time.Now()
	if err := service.ApplicationHost().RecoverGoalOperations(ctx); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Fatalf("startup recovery exceeded bounded budget: %s", elapsed)
	}
	op, found, err := store.GetGoalControlOperation(ctx, "ws-goal-startup-budget", "goal-startup-budget")
	if err != nil || !found || op.LeaseOwner != "" || op.Status != agentactivitybiz.GoalOperationStatusDispatched && op.Status != agentactivitybiz.GoalOperationStatusPrepared {
		t.Fatalf("startup operation=%#v found=%v err=%v", op, found, err)
	}
}

func TestAcceptedClaudeGoalExpiresWithoutProviderReplay(t *testing.T) {
	ctx := context.Background()
	store := openAgentServiceSQLiteStore(t)
	if err := store.Create(ctx, workspacebiz.Summary{ID: "ws-goal-accepted-age", Name: "Accepted Age"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReportSessionState(ctx, agentactivitybiz.SessionStateReport{
		WorkspaceID: "ws-goal-accepted-age", AgentSessionID: "session-goal-accepted-age", Provider: "claude-code", OccurredAtUnixMS: 10,
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := store.PrepareGoalControlOperation(ctx, agentactivitybiz.GoalControlOperationPrepare{
		OperationID: "goal-accepted-age", WorkspaceID: "ws-goal-accepted-age", AgentSessionID: "session-goal-accepted-age",
		Action: "set", Objective: "ship it", OccurredAtUnixMS: 20,
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.MarkGoalControlOperationDispatched(ctx, "ws-goal-accepted-age", "goal-accepted-age", 21); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := store.AcknowledgeGoalControlOperation(ctx, agentactivitybiz.GoalControlOperationAcknowledge{
		WorkspaceID: "ws-goal-accepted-age", OperationID: "goal-accepted-age", OccurredAtUnixMS: 22,
	}); err != nil {
		t.Fatal(err)
	}
	runtime := newFakeRuntime()
	runtime.sessions["ws-goal-accepted-age:session-goal-accepted-age"] = ProviderRuntimeSession{
		ID: "session-goal-accepted-age", Provider: "claude-code", ProviderSessionID: "claude-accepted", Status: "ready",
	}
	service := newIsolatedAgentService(runtime)
	service.GoalStateStore = store
	service.GoalOperationOwner = "goal-accepted-worker"
	service.GoalOperationClock = func() time.Time { return time.UnixMilli(130_000) }
	if err := service.ApplicationHost().StepGoalOperationWorker(ctx, false); err != nil {
		t.Fatal(err)
	}
	op, found, err := store.GetGoalControlOperation(ctx, "ws-goal-accepted-age", "goal-accepted-age")
	if err != nil || !found || op.Status != agentactivitybiz.GoalOperationStatusFailed {
		t.Fatalf("expired operation=%#v found=%v err=%v", op, found, err)
	}
	if len(runtime.goalControlCalls) != 0 {
		t.Fatalf("expired accepted operation replayed: %#v", runtime.goalControlCalls)
	}
}

func TestAcceptedClaudeClearExpiresWhenLifecycleEvidenceIsLost(t *testing.T) {
	ctx := context.Background()
	store := openAgentServiceSQLiteStore(t)
	if err := store.Create(ctx, workspacebiz.Summary{ID: "ws-clear-age", Name: "Clear Age"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReportSessionState(ctx, agentactivitybiz.SessionStateReport{WorkspaceID: "ws-clear-age", AgentSessionID: "session-clear-age", Provider: "claude-code", OccurredAtUnixMS: 10}); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := store.PrepareGoalControlOperation(ctx, agentactivitybiz.GoalControlOperationPrepare{OperationID: "clear-age", WorkspaceID: "ws-clear-age", AgentSessionID: "session-clear-age", Action: "clear", OccurredAtUnixMS: 20}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.MarkGoalControlOperationDispatched(ctx, "ws-clear-age", "clear-age", 21); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := store.AcknowledgeGoalControlOperation(ctx, agentactivitybiz.GoalControlOperationAcknowledge{WorkspaceID: "ws-clear-age", OperationID: "clear-age", OccurredAtUnixMS: 22}); err != nil {
		t.Fatal(err)
	}
	runtime := newFakeRuntime()
	runtime.sessions["ws-clear-age:session-clear-age"] = ProviderRuntimeSession{ID: "session-clear-age", Provider: "claude-code", ProviderSessionID: "claude-clear-age", Status: "ready"}
	service := newIsolatedAgentService(runtime)
	service.GoalStateStore = store
	service.GoalOperationOwner = "clear-age-worker"
	service.GoalOperationClock = func() time.Time { return time.UnixMilli(130_000) }
	if err := service.ApplicationHost().StepGoalOperationWorker(ctx, false); err != nil {
		t.Fatal(err)
	}
	op, found, err := store.GetGoalControlOperation(ctx, "ws-clear-age", "clear-age")
	if err != nil || !found || op.Status != agentactivitybiz.GoalOperationStatusFailed {
		t.Fatalf("op=%#v found=%v err=%v", op, found, err)
	}
	state, found, err := store.GetSessionGoalState(ctx, "ws-clear-age", "session-clear-age")
	if err != nil || !found || state.PendingOperationID != "" || state.SyncStatus != agentactivitybiz.GoalSyncStatusFailed {
		t.Fatalf("state=%#v found=%v err=%v", state, found, err)
	}
	if len(runtime.goalControlCalls) != 0 {
		t.Fatalf("expired clear replayed: %#v", runtime.goalControlCalls)
	}
}

func TestGoalReconcileFenceConflictRequeriesProvider(t *testing.T) {
	ctx := context.Background()
	store := openAgentServiceSQLiteStore(t)
	if err := store.Create(ctx, workspacebiz.Summary{ID: "ws-goal-requery", Name: "Goal Requery"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReportSessionState(ctx, agentactivitybiz.SessionStateReport{
		WorkspaceID: "ws-goal-requery", AgentSessionID: "session-goal-requery", Provider: "codex", OccurredAtUnixMS: 10,
	}); err != nil {
		t.Fatal(err)
	}
	runtime := newFakeRuntime()
	runtime.sessions["ws-goal-requery:session-goal-requery"] = ProviderRuntimeSession{
		ID: "session-goal-requery", Provider: "codex", ProviderSessionID: "codex-requery", Status: "ready",
	}
	queryCount := 0
	runtime.goalReconcileHook = func(_ context.Context, input RuntimeGoalControlInput) (RuntimeGoalReconcileResult, error) {
		queryCount++
		if queryCount == 1 {
			if _, _, _, err := store.PrepareGoalControlOperation(ctx, agentactivitybiz.GoalControlOperationPrepare{
				OperationID: "goal-created-during-query", WorkspaceID: "ws-goal-requery", AgentSessionID: "session-goal-requery",
				Action: "set", Objective: "new desired", OccurredAtUnixMS: 20,
			}); err != nil {
				return RuntimeGoalReconcileResult{}, err
			}
			return RuntimeGoalReconcileResult{AgentSessionID: input.AgentSessionID,
				Evidence: map[string]any{"confidence": "authoritative"}}, nil
		}
		return RuntimeGoalReconcileResult{AgentSessionID: input.AgentSessionID,
			Goal:     map[string]any{"objective": "new desired", "status": "active"},
			Evidence: map[string]any{"confidence": "authoritative"}}, nil
	}
	service := newIsolatedAgentService(runtime)
	service.GoalStateStore = store
	service.GoalOperationClock = func() time.Time { return time.UnixMilli(30) }
	result, err := service.ReconcileGoal(ctx, "ws-goal-requery", "session-goal-requery")
	if err != nil {
		t.Fatal(err)
	}
	if queryCount != 2 {
		t.Fatalf("provider query count=%d, want 2 after fence conflict", queryCount)
	}
	if result.State.Revision != 1 || result.State.PendingOperationID != "" ||
		result.State.SyncStatus != agentactivitybiz.GoalSyncStatusSynced || result.State.Observed["objective"] != "new desired" {
		t.Fatalf("reconciled state=%#v", result.State)
	}
}

func TestManualGoalReconcileProviderTimeoutIsBounded(t *testing.T) {
	ctx, expire := newControlledDeadlineContext()
	runtime := newFakeRuntime()
	runtime.sessions["ws-timeout:session-timeout"] = ProviderRuntimeSession{ID: "session-timeout", Provider: "codex", Status: "ready"}
	runtime.goalReconcileHook = func(ctx context.Context, _ RuntimeGoalControlInput) (RuntimeGoalReconcileResult, error) {
		expire()
		<-ctx.Done()
		return RuntimeGoalReconcileResult{}, ctx.Err()
	}
	service := newIsolatedAgentService(runtime)
	_, err := service.ReconcileGoal(ctx, "ws-timeout", "session-timeout")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%v, want context deadline exceeded", err)
	}
}

func TestServiceCreateWithTypedGoalCreatesNoInitialTurn(t *testing.T) {
	runtime := newFakeRuntime()
	store := &recordingGoalStateStore{}
	service := newTestService(runtime)
	service.GoalStateStore = store

	session, err := service.Create(context.Background(), "ws-typed", CreateSessionInput{
		AgentSessionID: "session-new-goal",
		AgentTargetID:  agenttargetbiz.IDLocalClaudeCode,
		InitialContent: TextPromptContent("/goal ship it"),
		Metadata:       map[string]any{"clientSubmitId": "submit-new-goal"},
	})
	if err != nil {
		t.Fatalf("Create typed goal: %v", err)
	}
	if session.ID != "session-new-goal" {
		t.Fatalf("session=%#v", session)
	}
	if len(runtime.execCalls) != 0 {
		t.Fatalf("typed Goal create entered Turn Exec: %#v", runtime.execCalls)
	}
	if len(runtime.goalControlCalls) != 1 || runtime.goalControlCalls[0].OperationID == "" {
		t.Fatalf("goal control calls=%#v", runtime.goalControlCalls)
	}
	if runtime.goalControlCalls[0].SubmissionMetadata["clientSubmitId"] != "submit-new-goal" {
		t.Fatalf("goal control submission metadata=%#v", runtime.goalControlCalls[0].SubmissionMetadata)
	}
	if len(store.prepared) != 1 || store.prepared[0].AgentSessionID != "session-new-goal" {
		t.Fatalf("prepared operations=%#v", store.prepared)
	}
}

func TestParseTypedGoalControlUsesSemanticContentAndUnicodeWhitespace(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		displayPrompt string
		want          agenthost.TypedGoalControl
		wantOK        bool
	}{
		{name: "space", content: "/goal clear", want: agenthost.TypedGoalControl{Action: "clear"}, wantOK: true},
		{name: "tab", content: "/goal\tclear", want: agenthost.TypedGoalControl{Action: "clear"}, wantOK: true},
		{name: "newline", content: "/goal\nship it", want: agenthost.TypedGoalControl{Action: "set", Objective: "ship it"}, wantOK: true},
		{name: "unicode space", content: "/goal\u3000ship it", want: agenthost.TypedGoalControl{Action: "set", Objective: "ship it"}, wantOK: true},
		{name: "display cannot hide control", content: "/goal clear", displayPrompt: "clear chip", want: agenthost.TypedGoalControl{Action: "clear"}, wantOK: true},
		{name: "display cannot create control", content: "ordinary prompt", displayPrompt: "/goal clear", wantOK: false},
		{name: "bare goal is not a mutation", content: "/goal", wantOK: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := agenthost.ParseTypedGoalControl(TextPromptContent(test.content), false)
			if ok != test.wantOK || got != test.want {
				t.Fatalf("parseTypedGoalControl()=(%#v,%v), want (%#v,%v)", got, ok, test.want, test.wantOK)
			}
		})
	}
}
