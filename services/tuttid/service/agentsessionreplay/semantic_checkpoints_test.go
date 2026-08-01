package agentsessionreplay

import (
	"context"
	"path/filepath"
	"testing"

	agenthost "github.com/tutti-os/tutti/packages/agent/host"
	replay "github.com/tutti-os/tutti/packages/agent/session-replay"
	storesqlite "github.com/tutti-os/tutti/packages/agent/store-sqlite"
	replaybiz "github.com/tutti-os/tutti/services/tuttid/biz/agentsessionreplay"
)

func TestNeutralBootstrapCheckpointNeedsNoCanonicalSession(t *testing.T) {
	runtime := &SemanticRuntime{
		plans: map[string]replay.CheckpointPlan{
			"cassette-1": replay.NewCheckpointPlan(
				[]replay.ReplayCheckpoint{{
					ID: "checkpoint-0000", Index: 0,
					Kind: "replay.bootstrap", Tags: []string{"replay.bootstrap"},
					Trigger: replay.CheckpointTrigger{
						Source: replay.CheckpointTriggerBootstrap,
					},
					Readiness: replay.CheckpointReadiness{
						All: []replay.ReadinessPredicate{},
					},
				}},
			),
		},
	}
	state, err := runtime.VerifyCheckpoint(
		context.Background(),
		"cassette-1",
		0,
	)
	if err != nil || !state.TriggerMatched || !state.ReadinessSatisfied {
		t.Fatalf("bootstrap state=%#v error=%v", state, err)
	}
}

func TestCanonicalTurnPhaseFoldsActivityVocabulary(t *testing.T) {
	tests := map[string]string{
		"working":            storesqlite.TurnPhaseRunning,
		"streaming":          storesqlite.TurnPhaseRunning,
		"waiting_approval":   storesqlite.TurnPhaseWaiting,
		"awaiting_approval":  storesqlite.TurnPhaseWaiting,
		"waiting_input":      storesqlite.TurnPhaseWaiting,
		" waiting_approval ": storesqlite.TurnPhaseWaiting,
		"running":            storesqlite.TurnPhaseRunning,
		"waiting":            storesqlite.TurnPhaseWaiting,
		"submitted":          storesqlite.TurnPhaseSubmitted,
		"settling":           storesqlite.TurnPhaseSettling,
		"settled":            storesqlite.TurnPhaseSettled,
		"idle":               storesqlite.TurnPhaseSettled,
	}
	for recorded, want := range tests {
		if got := canonicalTurnPhase(recorded); got != want {
			t.Fatalf(
				"canonicalTurnPhase(%q) = %q, want %q",
				recorded,
				got,
				want,
			)
		}
	}
}

func TestSemanticTurnStateMatchesTerminalActivityPhase(t *testing.T) {
	if !semanticTurnStateMatches(
		storesqlite.TurnPhaseSettled,
		storesqlite.TurnOutcomeCompleted,
		"idle",
		storesqlite.TurnOutcomeCompleted,
	) {
		t.Fatal("canonical settled Turn should match activity-layer terminal idle")
	}
}

func TestSemanticTurnStateMatchesFoldsWorkingToRunning(t *testing.T) {
	if !semanticTurnStateMatches(
		storesqlite.TurnPhaseRunning,
		"",
		"working",
		"",
	) {
		t.Fatal("canonical running should match activity-layer working")
	}
	if !semanticRootProviderTurnMatches(
		storesqlite.RootProviderTurnPhaseRunning,
		"",
		"working",
		"",
	) {
		t.Fatal("root-provider running should match activity-layer working")
	}
}

func TestProjectBindingMatchesCanonicalCWDAndRailPlacement(t *testing.T) {
	actual := storesqlite.Session{
		AgentTargetID:   "local:codex",
		Provider:        "codex",
		Cwd:             "/runtime/repo",
		RailSectionKind: storesqlite.RailSectionKindProject,
		RailProjectPath: "/runtime/repo/packages/agent",
		RailSectionKey:  "project:/runtime/repo/packages/agent",
	}
	expected := agenthost.HistoricalSession{
		AgentTargetID:   actual.AgentTargetID,
		Provider:        actual.Provider,
		Cwd:             actual.Cwd,
		RailSectionKind: actual.RailSectionKind,
		RailProjectPath: actual.RailProjectPath,
		RailSectionKey:  actual.RailSectionKey,
	}
	if !projectBindingMatches(actual, expected) {
		t.Fatal("matching project binding was rejected")
	}
	tests := map[string]func(*agenthost.HistoricalSession){
		"cwd": func(value *agenthost.HistoricalSession) {
			value.Cwd = "/runtime/other"
		},
		"rail kind": func(value *agenthost.HistoricalSession) {
			value.RailSectionKind = storesqlite.RailSectionKindConversations
		},
		"project path": func(value *agenthost.HistoricalSession) {
			value.RailProjectPath = "/runtime/repo/packages/other"
		},
		"section key": func(value *agenthost.HistoricalSession) {
			value.RailSectionKey = "project:/runtime/repo/packages/other"
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			mismatched := expected
			mutate(&mismatched)
			if projectBindingMatches(actual, mismatched) {
				t.Fatalf("%s mismatch was accepted", name)
			}
		})
	}
}

func TestProjectBindingReadinessResolvesPortableExpectedState(t *testing.T) {
	replayCWD, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	actual := storesqlite.Session{
		AgentTargetID:   "local:codex",
		Provider:        "codex",
		Cwd:             replayCWD,
		RailSectionKind: storesqlite.RailSectionKindProject,
		RailProjectPath: replayCWD,
		RailSectionKey:  storesqlite.RailSectionKeyForProject(replayCWD),
	}
	portable := agenthost.HistoricalSessionGraph{
		RootSessionID: "session-1",
		Sessions: []agenthost.HistoricalSession{{
			ID:              "session-1",
			AgentTargetID:   "local:codex",
			Provider:        "codex",
			Cwd:             replay.PortableReplayCWDToken,
			RailSectionKind: storesqlite.RailSectionKindProject,
			RailProjectPath: replay.PortableReplayCWDToken,
			RailSectionKey:  "project:" + replay.PortableReplayCWDToken,
		}},
	}
	if projectBindingMatches(actual, portable.Sessions[0]) {
		t.Fatal("portable expected binding must not match a canonical Session")
	}
	resolved, err := replaybiz.ResolvePortableAgentState(portable, replayCWD)
	if err != nil {
		t.Fatal(err)
	}
	if !projectBindingMatches(actual, resolved.Sessions[0]) {
		t.Fatalf(
			"resolved expected binding was rejected: %#v",
			resolved.Sessions[0],
		)
	}
}

func TestSemanticGoalStatusUsesPortableCheckpointVocabulary(t *testing.T) {
	tests := map[string]struct {
		state storesqlite.SessionGoalState
		want  string
	}{
		"running": {
			state: storesqlite.SessionGoalState{
				Observed: map[string]any{"status": "active"},
			},
			want: "running",
		},
		"completed": {
			state: storesqlite.SessionGoalState{
				Observed: map[string]any{"status": "complete"},
			},
			want: "completed",
		},
		"cleared": {
			state: storesqlite.SessionGoalState{Tombstoned: true},
			want:  "cleared",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := semanticGoalStatus(test.state); got != test.want {
				t.Fatalf("semanticGoalStatus() = %q, want %q", got, test.want)
			}
		})
	}
}
