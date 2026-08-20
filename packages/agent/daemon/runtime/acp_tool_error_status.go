package agentruntime

func acpResolvedToolCallStatus(update map[string]any, fallback string) string {
	if acpToolCallReportsError(update, acpToolCallRawOutput(update)) {
		return messageStreamStateFailed
	}
	status := normalizedCallStatus(firstNonEmpty(asString(update["status"]), fallback))
	if status != messageStreamStateStreaming {
		return status
	}
	rawOutput := acpToolCallRawOutput(update)
	if inferred := acpInferTerminalToolStatus(rawOutput); inferred != "" {
		return inferred
	}
	if inferred := acpInferImageGenerationTerminalStatus(update, rawOutput); inferred != "" {
		return inferred
	}
	return status
}

func acpToolCallReportsError(update map[string]any, rawOutput any) bool {
	return acpValueReportsError(update) || acpValueReportsError(rawOutput)
}

func acpValueReportsError(value any) bool {
	body, ok := value.(map[string]any)
	if !ok {
		return false
	}
	for _, key := range []string{"isError", "is_error"} {
		if reported, ok := body[key].(bool); ok && reported {
			return true
		}
	}
	return false
}
