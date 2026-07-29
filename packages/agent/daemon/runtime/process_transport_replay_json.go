package agentruntime

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"

	replay "github.com/tutti-os/tutti/packages/agent/session-replay"
)

func processCassetteJSONMatch(
	expected []byte,
	actual []byte,
	recordedCWD string,
	replayCWD string,
	replayHome string,
) (map[string]any, bool) {
	expectedValues, ok := decodeProcessCassetteJSONValues(expected)
	if !ok {
		return nil, false
	}
	actualValues, ok := decodeProcessCassetteJSONValues(actual)
	if !ok || len(expectedValues) != len(actualValues) {
		return nil, false
	}
	projectProcessCassetteRuntimeGeneratedFields(expectedValues)
	projectProcessCassetteRuntimeGeneratedFields(actualValues)
	responseIDs := map[string]any{}
	for index := range expectedValues {
		expectedValues[index] = mapProcessCassettePathFields(
			expectedValues[index],
			recordedCWD,
			replayCWD,
		)
		expectedValues[index] = mapProcessCassettePathFields(
			expectedValues[index],
			portableProcessCassetteHomeToken,
			replayHome,
		)
		expectedMessage, expectedIsMessage := expectedValues[index].(map[string]any)
		actualMessage, actualIsMessage := actualValues[index].(map[string]any)
		if expectedIsMessage && actualIsMessage &&
			expectedMessage["method"] != nil && actualMessage["method"] != nil {
			expectedID, expectedHasID := expectedMessage["id"]
			actualID, actualHasID := actualMessage["id"]
			if expectedHasID != actualHasID {
				return nil, false
			}
			if expectedHasID {
				responseIDs[processCassetteJSONRPCID(expectedID)] = actualID
				expectedMessage["id"] = actualID
			}
		}
		if !reflect.DeepEqual(expectedValues[index], actualValues[index]) {
			return nil, false
		}
	}
	return responseIDs, true
}

func processCassetteJSONRPCRequest(
	chunk processCassetteChunk,
) (method string, responseID string, ok bool) {
	if chunk.Kind != "outbound" {
		return "", "", false
	}
	data, err := base64.StdEncoding.DecodeString(chunk.Data)
	if err != nil {
		return "", "", false
	}
	values, ok := decodeProcessCassetteJSONValues(data)
	if !ok || len(values) != 1 {
		return "", "", false
	}
	request, ok := values[0].(map[string]any)
	if !ok {
		return "", "", false
	}
	method, ok = request["method"].(string)
	if !ok || strings.TrimSpace(method) == "" {
		return "", "", false
	}
	if id, exists := request["id"]; exists {
		responseID = processCassetteJSONRPCID(id)
	}
	return method, responseID, true
}

func processCassetteJSONRPCID(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(raw)
}

func isOptionalReplayProbeMethod(method string) bool {
	switch strings.TrimSpace(method) {
	case "thread/read", "thread/goal/get":
		return true
	default:
		return false
	}
}

func suppressSkippedProcessCassetteResponses(
	data []byte,
	skipped map[string]struct{},
) []byte {
	if len(data) == 0 || len(skipped) == 0 {
		return data
	}
	values, ok := decodeProcessCassetteJSONValues(data)
	if !ok {
		return data
	}
	filtered := make([]any, 0, len(values))
	for _, value := range values {
		message, isObject := value.(map[string]any)
		if !isObject || message["method"] != nil {
			filtered = append(filtered, value)
			continue
		}
		id := processCassetteJSONRPCID(message["id"])
		_, isSkipped := skipped[id]
		_, hasResult := message["result"]
		_, hasError := message["error"]
		if !isSkipped || (!hasResult && !hasError) {
			filtered = append(filtered, value)
			continue
		}
		delete(skipped, id)
	}
	if len(filtered) == len(values) {
		return data
	}
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	for _, value := range filtered {
		if err := encoder.Encode(value); err != nil {
			return data
		}
	}
	return output.Bytes()
}

func mapProcessCassetteResponseIDs(data []byte, responseIDs map[string]any) []byte {
	if len(data) == 0 || len(responseIDs) == 0 {
		return data
	}
	values, ok := decodeProcessCassetteJSONValues(data)
	if !ok {
		return data
	}
	changed := false
	for _, value := range values {
		message, isObject := value.(map[string]any)
		if !isObject || message["method"] != nil {
			continue
		}
		_, hasResult := message["result"]
		_, hasError := message["error"]
		if !hasResult && !hasError {
			continue
		}
		recordedID := processCassetteJSONRPCID(message["id"])
		replayID, exists := responseIDs[recordedID]
		if !exists {
			continue
		}
		message["id"] = replayID
		changed = true
	}
	if !changed {
		return data
	}
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	for _, value := range values {
		if err := encoder.Encode(value); err != nil {
			return data
		}
	}
	return output.Bytes()
}

func decodeProcessCassetteJSONValues(data []byte) ([]any, bool) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var values []any
	for {
		var value any
		if err := decoder.Decode(&value); err != nil {
			return values, errors.Is(err, io.EOF) && len(values) > 0
		}
		values = append(values, value)
	}
}

func mapProcessCassettePathFields(value any, oldValue string, newValue string) any {
	switch typed := value.(type) {
	case []any:
		for index := range typed {
			typed[index] = mapProcessCassettePathFields(
				typed[index],
				oldValue,
				newValue,
			)
		}
		return typed
	case map[string]any:
		for key, child := range typed {
			if isProcessCassettePathField(key) {
				if path, ok := child.(string); ok {
					if mapped, changed := mapProcessCassettePath(path, oldValue, newValue); changed {
						typed[key] = mapped
						continue
					}
				}
				if values, ok := child.([]any); ok {
					typed[key] = mapProcessCassettePathValues(values, oldValue, newValue)
					continue
				}
			}
			typed[key] = mapProcessCassettePathFields(child, oldValue, newValue)
		}
		return typed
	default:
		return value
	}
}

func mapProcessCassettePathValues(values []any, oldValue, newValue string) []any {
	for index, value := range values {
		if path, ok := value.(string); ok {
			if mapped, changed := mapProcessCassettePath(path, oldValue, newValue); changed {
				values[index] = mapped
			}
		}
	}
	return values
}

func mapProcessCassettePath(path, oldValue, newValue string) (string, bool) {
	if path == oldValue {
		return newValue, true
	}
	for _, separator := range []string{"/", `\`} {
		prefix := strings.TrimRight(oldValue, `/\`) + separator
		if strings.HasPrefix(path, prefix) {
			return strings.TrimRight(newValue, `/\`) + separator + strings.TrimPrefix(path, prefix), true
		}
	}
	return path, false
}

func isProcessCassettePathField(key string) bool {
	return replay.IsProviderPathField(key)
}
