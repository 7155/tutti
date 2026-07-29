package agent

import (
	"context"
	"testing"

	agenthost "github.com/tutti-os/tutti/packages/agent/host"
	"github.com/tutti-os/tutti/packages/agent/runtimeprep"
)

func TestNativeCapabilityPlanRuntimeContextProjection(t *testing.T) {
	plan := &runtimeprep.NativeCapabilityPlan{
		CodexHome: "/tmp/session/codex-home",
		Entries: []runtimeprep.NativeCapabilityPlanEntry{{
			Capability: runtimeprep.CodexNativeCapabilityBrowser,
			PluginID:   runtimeprep.CodexNativePluginBrowser,
			State:      runtimeprep.NativeCapabilityReady,
			Backend:    runtimeprep.CapabilityBackendCodexNative,
			Reason:     "ready",
			PluginPath: "plugin://browser@openai-bundled",
		}},
	}
	projected := nativeCapabilityPlanRuntimeContext(plan)
	raw, ok := projected[nativeCapabilityPlanRuntimeContextKey].(map[string]any)
	if !ok {
		t.Fatalf("projected = %#v", projected)
	}
	if raw["codexHome"] != plan.CodexHome {
		t.Fatalf("codexHome = %#v", raw["codexHome"])
	}
	entries, ok := raw["entries"].([]map[string]any)
	if !ok || len(entries) != 1 || entries[0]["backend"] != string(runtimeprep.CapabilityBackendCodexNative) {
		t.Fatalf("entries = %#v", raw["entries"])
	}

	merged := mergeRuntimeContext(map[string]any{"keep": true}, projected)
	if merged["keep"] != true {
		t.Fatalf("merged = %#v", merged)
	}
	if _, ok := merged[nativeCapabilityPlanRuntimeContextKey]; !ok {
		t.Fatalf("missing plan in merged = %#v", merged)
	}
}

type nativeCapabilityPreparationSupport struct {
	prepared preparedRuntime
}

func (*nativeCapabilityPreparationSupport) clampPersistedSessionReasoningEffortForResume(
	_ context.Context,
	session PersistedSession,
) PersistedSession {
	return session
}

func (s *nativeCapabilityPreparationSupport) prepareRuntimeForResume(
	context.Context,
	PersistedSession,
) (preparedRuntime, error) {
	return s.prepared, nil
}

func (*nativeCapabilityPreparationSupport) resolveProviderTargetRefForResume(
	context.Context,
	PersistedSession,
) (map[string]any, error) {
	return nil, nil
}

func (*nativeCapabilityPreparationSupport) cleanupSessionResources(
	context.Context,
	string,
	string,
) error {
	return nil
}

func (*nativeCapabilityPreparationSupport) deleteTuttiModeActivationSessionState(
	context.Context,
	string,
	string,
) error {
	return nil
}

func TestServiceHostPreparationPreservesNativeCapabilityPlan(t *testing.T) {
	plan := &runtimeprep.NativeCapabilityPlan{
		CodexHome: "/tmp/session/codex-home",
		Entries: []runtimeprep.NativeCapabilityPlanEntry{{
			Capability: runtimeprep.CodexNativeCapabilityBrowser,
			Backend:    runtimeprep.CapabilityBackendCodexNative,
		}},
	}
	support := &nativeCapabilityPreparationSupport{
		prepared: preparedRuntime{Cwd: "/tmp/session", NativeCapabilityPlan: plan},
	}
	preparation := serviceHostPreparation{support: support}

	t.Run("create override", func(t *testing.T) {
		ctx := context.WithValue(
			context.Background(),
			servicePreparedRuntimeContextKey{},
			servicePreparedRuntimeContext{support: support, prepared: support.prepared},
		)
		prepared, err := preparation.Prepare(ctx, agenthost.RuntimePreparationInput{})
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := prepared.RuntimeContext[nativeCapabilityPlanRuntimeContextKey]; !ok {
			t.Fatalf("runtime context = %#v", prepared.RuntimeContext)
		}
	})

	t.Run("resume", func(t *testing.T) {
		prepared, err := preparation.Prepare(context.Background(), agenthost.RuntimePreparationInput{
			RuntimeContext: map[string]any{"keep": true},
		})
		if err != nil {
			t.Fatal(err)
		}
		if prepared.RuntimeContext["keep"] != true {
			t.Fatalf("runtime context = %#v", prepared.RuntimeContext)
		}
		if _, ok := prepared.RuntimeContext[nativeCapabilityPlanRuntimeContextKey]; !ok {
			t.Fatalf("runtime context = %#v", prepared.RuntimeContext)
		}
	})
}
