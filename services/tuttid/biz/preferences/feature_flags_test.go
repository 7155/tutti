package preferences

import "testing"

func TestAgentSessionRecordingCapabilityFlagFailsClosed(t *testing.T) {
	if IsCapabilityFlagEnabled(nil, FeatureFlagAgentSessionRecording) {
		t.Fatal("absent agent Session Recording flag must resolve false")
	}
	if !IsCapabilityFlagEnabled(
		map[string]bool{FeatureFlagAgentSessionRecording: true},
		FeatureFlagAgentSessionRecording,
	) {
		t.Fatal("stored agent Session Recording true must win")
	}
	if IsCapabilityFlagEnabled(nil, "agent.unknown") {
		t.Fatal("unknown capability flag must resolve false")
	}
}
