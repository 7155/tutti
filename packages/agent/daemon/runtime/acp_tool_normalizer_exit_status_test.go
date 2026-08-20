package agentruntime

import "testing"

func TestACPInferTerminalToolStatusUsesProviderStatusBeforeExitCode(t *testing.T) {
	tests := []struct {
		name      string
		rawOutput map[string]any
		expected  string
	}{
		{name: "provider completed owns nonzero exit", rawOutput: map[string]any{"status": "completed", "exitCode": 1}, expected: messageStreamStateCompleted},
		{name: "provider failed owns zero exit", rawOutput: map[string]any{"state": "failed", "exitCode": 0}, expected: messageStreamStateFailed},
		{name: "zero exit fallback completes", rawOutput: map[string]any{"exitCode": 0}, expected: messageStreamStateCompleted},
		{name: "nonzero exit fallback fails", rawOutput: map[string]any{"exitCode": 1}, expected: messageStreamStateFailed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := acpInferTerminalToolStatus(test.rawOutput); got != test.expected {
				t.Fatalf("status = %q, want %q", got, test.expected)
			}
		})
	}
}

func TestACPResolvedToolCallStatusTreatsExplicitErrorAsFailed(t *testing.T) {
	tests := []struct {
		name   string
		update map[string]any
		want   string
	}{
		{
			name: "raw output isError overrides completed status",
			update: map[string]any{
				"status": "completed",
				"output": map[string]any{"isError": true},
			},
			want: messageStreamStateFailed,
		},
		{
			name: "top level is_error overrides completed status",
			update: map[string]any{
				"status":   "completed",
				"is_error": true,
			},
			want: messageStreamStateFailed,
		},
		{
			name: "false error flag preserves completed status",
			update: map[string]any{
				"status": "completed",
				"output": map[string]any{"isError": false},
			},
			want: messageStreamStateCompleted,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := acpResolvedToolCallStatus(test.update, messageStreamStateStreaming); got != test.want {
				t.Fatalf("status = %q, want %q", got, test.want)
			}
		})
	}
}
