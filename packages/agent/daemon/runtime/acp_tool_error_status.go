package agentruntime

const maxACPErrorEnvelopeDepth = 8

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
	return acpMapOwnReportsError(update) ||
		acpValueReportsError(update["error"]) ||
		acpValueReportsError(update["content"]) ||
		acpValueReportsError(rawOutput)
}

func acpValueReportsError(value any) bool {
	return acpValueReportsErrorAtDepth(value, 0)
}

func acpMapOwnReportsError(body map[string]any) bool {
	for _, key := range []string{"isError", "is_error"} {
		if reported, ok := body[key].(bool); ok && reported {
			return true
		}
	}
	return false
}

func acpValueReportsErrorAtDepth(value any, depth int) bool {
	body, ok := value.(map[string]any)
	if ok {
		if acpMapOwnReportsError(body) {
			return true
		}
		if depth >= maxACPErrorEnvelopeDepth {
			return false
		}
		for _, nested := range body {
			if acpValueReportsErrorAtDepth(nested, depth+1) {
				return true
			}
		}
		return false
	}
	entries, ok := value.([]any)
	if !ok || depth >= maxACPErrorEnvelopeDepth {
		return false
	}
	for _, nested := range entries {
		if acpValueReportsErrorAtDepth(nested, depth+1) {
			return true
		}
	}
	return false
}
