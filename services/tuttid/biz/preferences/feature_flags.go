package preferences

const FeatureFlagAgentSessionRecording = "agent.sessionRecording"

var capabilityFlagDefaults = map[string]bool{
	FeatureFlagAgentSessionRecording: false,
}

// IsCapabilityFlagEnabled resolves daemon-enforced feature behavior. Stored
// values win and unknown or absent keys fail closed.
func IsCapabilityFlagEnabled(flags map[string]bool, key string) bool {
	if enabled, ok := flags[key]; ok {
		return enabled
	}
	return capabilityFlagDefaults[key]
}
