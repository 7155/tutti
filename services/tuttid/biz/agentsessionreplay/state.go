package agentsessionreplay

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	agenthost "github.com/tutti-os/tutti/packages/agent/host"
	sessionreplay "github.com/tutti-os/tutti/packages/agent/session-replay"
	storesqlite "github.com/tutti-os/tutti/packages/agent/store-sqlite"
)

var ErrTuttiReplayStateConflict = errors.New("tutti replay state conflict")

const SchemaVersion = 1
const StateFormat = "tutti.agent-session-replay-state.v1"

type TuttiReplayState struct {
	SchemaVersion int                   `json:"schemaVersion"`
	Agent         TuttiReplayAgent      `json:"agent"`
	TuttiMode     TuttiReplayTuttiMode  `json:"tuttiMode"`
	Workflows     []TuttiReplayWorkflow `json:"workflows"`
	Issues        []TuttiReplayIssue    `json:"issues"`
}

type TuttiReplayAgent = agenthost.HistoricalSessionGraph
type TuttiReplaySession = agenthost.HistoricalSession
type TuttiReplayTurn = agenthost.HistoricalTurn
type TuttiReplayMessage = agenthost.HistoricalMessage
type TuttiReplayInteraction = agenthost.HistoricalInteraction
type TuttiReplayGoal = agenthost.HistoricalGoal

type TuttiReplayTuttiMode struct {
	Activations   []TuttiReplayActivation   `json:"activations"`
	TurnSnapshots []TuttiReplayTurnSnapshot `json:"turnSnapshots"`
}

type TuttiReplayActivation struct {
	ID                string `json:"id"`
	SessionID         string `json:"sessionId"`
	CurrentRevisionID string `json:"currentRevisionId"`
	CurrentRevision   int64  `json:"currentRevision"`
	State             string `json:"state"`
	Source            string `json:"source"`
	Effect            int    `json:"effect"`
	Speed             int    `json:"speed"`
}

type TuttiReplayTurnSnapshot struct {
	SessionID         string `json:"sessionId"`
	TurnID            string `json:"turnId"`
	ActivationID      string `json:"activationId,omitempty"`
	RevisionID        string `json:"revisionId,omitempty"`
	Revision          int64  `json:"revision"`
	State             string `json:"state"`
	Source            string `json:"source,omitempty"`
	PreferenceVersion int    `json:"preferenceVersion"`
	Effect            int    `json:"effect"`
	Speed             int    `json:"speed"`
	DispatchState     string `json:"dispatchState"`
}

type TuttiReplayWorkflow struct {
	ID                string   `json:"id"`
	Type              string   `json:"type"`
	TriggerKind       string   `json:"triggerKind"`
	SourceSessionID   string   `json:"sourceSessionId"`
	SourceTurnID      string   `json:"sourceTurnId"`
	SourceToolCallID  string   `json:"sourceToolCallId"`
	Status            string   `json:"status"`
	CurrentRevisionID string   `json:"currentRevisionId"`
	IssueIDs          []string `json:"issueIds"`
}

type TuttiReplayIssue struct {
	ID      string                 `json:"id"`
	Title   string                 `json:"title"`
	Content string                 `json:"content,omitempty"`
	Status  string                 `json:"status"`
	Tasks   []TuttiReplayIssueTask `json:"tasks"`
}

type TuttiReplayIssueTask struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Content  string `json:"content,omitempty"`
	Status   string `json:"status"`
	Priority string `json:"priority"`
	Position int    `json:"position"`
}

type TuttiReplayStateConflictError struct {
	Path string
}

func (e *TuttiReplayStateConflictError) Error() string {
	return fmt.Sprintf("%s at %s", ErrTuttiReplayStateConflict, e.Path)
}

func (*TuttiReplayStateConflictError) Unwrap() error {
	return ErrTuttiReplayStateConflict
}

type TuttiReplayMergedState struct {
	Agents    []agenthost.HistoricalSessionGraph
	TuttiMode TuttiReplayTuttiMode
	Workflows []TuttiReplayWorkflow
	Issues    []TuttiReplayIssue
}

// ProjectPortableAgentState removes provider runtime context that is not part
// of Tutti's semantic replay contract. Tool-owned nested arguments remain
// untouched; only canceled-Turn completion watermarks and normalized
// tool-message and Interaction envelopes are projected.
func ProjectPortableAgentState(
	agent TuttiReplayAgent,
	stateDirectory string,
) TuttiReplayAgent {
	projected := agent
	projected.Sessions = make([]TuttiReplaySession, len(agent.Sessions))
	copy(projected.Sessions, agent.Sessions)
	rootCWD := replayRootCWD(agent)
	for sessionIndex := range projected.Sessions {
		session := &projected.Sessions[sessionIndex]
		sourceSession := &agent.Sessions[sessionIndex]
		providerHome := replayProviderHome(
			*sourceSession,
			stateDirectory,
		)
		session.Cwd = portableReplayPath(session.Cwd, rootCWD)
		session.RailProjectPath = portableReplayPath(
			session.RailProjectPath,
			rootCWD,
		)
		if session.RailSectionKind == storesqlite.RailSectionKindProject &&
			strings.TrimSpace(session.RailSectionKey) ==
				storesqlite.RailSectionKeyForProject(sourceSession.RailProjectPath) {
			session.RailSectionKey = "project:" + session.RailProjectPath
		}
		session.Turns = make([]TuttiReplayTurn, len(sourceSession.Turns))
		copy(session.Turns, sourceSession.Turns)
		for turnIndex := range session.Turns {
			projectPortableCanceledTurnCompletionWatermark(
				&session.Turns[turnIndex],
			)
		}
		session.Messages = make([]TuttiReplayMessage, len(session.Messages))
		copy(session.Messages, sourceSession.Messages)
		for messageIndex := range session.Messages {
			message := &session.Messages[messageIndex]
			projectPortablePlanDecisionMessage(message)
			projectPortableGeneratedImageMessage(message, providerHome)
			if message.Kind != "tool_call" {
				continue
			}
			input, ok := message.Payload["input"].(map[string]any)
			if !ok {
				continue
			}
			payload := cloneReplayMap(message.Payload)
			payload["input"] = projectPortableApprovalInput(input)
			message.Payload = payload
		}
		session.Interactions = make(
			[]TuttiReplayInteraction,
			len(sourceSession.Interactions),
		)
		copy(session.Interactions, sourceSession.Interactions)
		for interactionIndex := range session.Interactions {
			interaction := &session.Interactions[interactionIndex]
			interaction.Input = projectPortableApprovalInput(interaction.Input)
		}
	}
	return projected
}

func replayProviderHome(
	session TuttiReplaySession,
	stateDirectory string,
) string {
	descriptor, ok := sessionreplay.ResolveProviderReplay(
		session.AgentTargetID,
		session.Provider,
	)
	directory := strings.TrimSpace(
		descriptor.PortableRuntime.SessionHomeDirectory,
	)
	if !ok || directory == "" {
		return ""
	}
	return filepath.Join(
		filepath.Clean(strings.TrimSpace(stateDirectory)),
		"agent",
		"runs",
		session.ID,
		directory,
	)
}

func projectPortableGeneratedImageMessage(
	message *TuttiReplayMessage,
	providerHome string,
) {
	if message.Kind != "tool_call" || !filepath.IsAbs(providerHome) {
		return
	}
	output, ok := message.Payload["output"].(map[string]any)
	if !ok {
		return
	}
	projectedOutput := cloneReplayMap(output)
	changed := false
	if savedPath, ok := output["savedPath"].(string); ok {
		if portable, ok := portableGeneratedImagePath(savedPath, providerHome); ok {
			projectedOutput["savedPath"] = portable
			changed = true
		}
	}
	if savedPaths, ok := output["savedPaths"].([]any); ok {
		projectedPaths := append([]any(nil), savedPaths...)
		for index, value := range projectedPaths {
			savedPath, ok := value.(string)
			if !ok {
				continue
			}
			if portable, ok := portableGeneratedImagePath(savedPath, providerHome); ok {
				projectedPaths[index] = portable
				changed = true
			}
		}
		projectedOutput["savedPaths"] = projectedPaths
	}
	if changed {
		payload := cloneReplayMap(message.Payload)
		payload["output"] = projectedOutput
		message.Payload = payload
	}
}

func portableGeneratedImagePath(value, providerHome string) (string, bool) {
	value = filepath.Clean(strings.TrimSpace(value))
	relative, err := filepath.Rel(providerHome, value)
	if err != nil || filepath.IsAbs(relative) ||
		relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	portable := filepath.ToSlash(relative)
	if !strings.HasPrefix(portable, "generated_images/") {
		return "", false
	}
	return sessionreplay.PortableReplayHomeToken + "/" + portable, true
}

func projectPortableCanceledTurnCompletionWatermark(turn *TuttiReplayTurn) {
	if strings.TrimSpace(turn.Outcome) != "canceled" ||
		turn.CompletedCommand == nil {
		return
	}
	completedCommand := cloneReplayMap(turn.CompletedCommand)
	delete(completedCommand, "finalAssistantMessageId")
	delete(completedCommand, "finalAssistantMessageResolved")
	if len(completedCommand) == 0 {
		completedCommand = nil
	}
	turn.CompletedCommand = completedCommand
}

// ResolvePortableAgentState materializes only the typed Session binding fields
// relative to the replay runtime root. User-authored payloads remain untouched.
func ResolvePortableAgentState(
	agent TuttiReplayAgent,
	replayCWD string,
) (TuttiReplayAgent, error) {
	replayCWD = filepath.Clean(strings.TrimSpace(replayCWD))
	if replayCWD == "." || !filepath.IsAbs(replayCWD) {
		return TuttiReplayAgent{}, errors.New("replay cwd must be absolute")
	}
	resolved := agent
	resolved.Sessions = make([]TuttiReplaySession, len(agent.Sessions))
	copy(resolved.Sessions, agent.Sessions)
	for index := range resolved.Sessions {
		session := &resolved.Sessions[index]
		var err error
		session.Cwd, err = resolvePortableReplayPath(session.Cwd, replayCWD)
		if err != nil {
			return TuttiReplayAgent{}, err
		}
		session.RailProjectPath, err = resolvePortableReplayPath(
			session.RailProjectPath,
			replayCWD,
		)
		if err != nil {
			return TuttiReplayAgent{}, err
		}
		if strings.HasPrefix(
			session.RailSectionKey,
			"project:"+sessionreplay.PortableReplayCWDToken,
		) {
			portablePath := strings.TrimPrefix(session.RailSectionKey, "project:")
			projectPath, err := resolvePortableReplayPath(portablePath, replayCWD)
			if err != nil {
				return TuttiReplayAgent{}, err
			}
			session.RailSectionKey = storesqlite.RailSectionKeyForProject(projectPath)
		}
	}
	return resolved, nil
}

func replayRootCWD(agent TuttiReplayAgent) string {
	for _, session := range agent.Sessions {
		if session.ID == agent.RootSessionID {
			return strings.TrimSpace(session.Cwd)
		}
	}
	return ""
}

func portableReplayPath(path, root string) string {
	path = strings.TrimSpace(path)
	root = strings.TrimSpace(root)
	if path == "" || root == "" {
		return path
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) ||
		filepath.IsAbs(relative) {
		return path
	}
	if relative == "." {
		return sessionreplay.PortableReplayCWDToken
	}
	return sessionreplay.PortableReplayCWDToken + "/" + filepath.ToSlash(relative)
}

func resolvePortableReplayPath(path, replayCWD string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	if path == sessionreplay.PortableReplayCWDToken {
		return replayCWD, nil
	}
	prefix := sessionreplay.PortableReplayCWDToken + "/"
	if !strings.HasPrefix(path, prefix) {
		return path, nil
	}
	relative := filepath.FromSlash(strings.TrimPrefix(path, prefix))
	resolved := filepath.Clean(filepath.Join(replayCWD, relative))
	within, err := filepath.Rel(replayCWD, resolved)
	if err != nil || within == ".." ||
		strings.HasPrefix(within, ".."+string(filepath.Separator)) ||
		filepath.IsAbs(within) {
		return "", errors.New("portable replay path escapes replay cwd")
	}
	return resolved, nil
}

func projectPortablePlanDecisionMessage(message *TuttiReplayMessage) {
	clientSubmitID, _ := message.Payload["clientSubmitId"].(string)
	if strings.HasPrefix(
		strings.TrimSpace(clientSubmitID),
		"plan-decision:",
	) {
		const portableClientSubmitID = "plan-decision:<runtime-operation>"
		payload := cloneReplayMap(message.Payload)
		payload["clientSubmitId"] = portableClientSubmitID
		message.Payload = payload
		if message.ID == "client-submit:user:"+clientSubmitID {
			message.ID = "client-submit:user:" + portableClientSubmitID
		}
		return
	}
	noticeKind, _ := message.Payload["noticeKind"].(string)
	operationID, _ := message.Payload["operationId"].(string)
	if !strings.HasPrefix(noticeKind, "plan_implementation_") ||
		strings.TrimSpace(operationID) == "" {
		return
	}
	const portableOperationID = "<runtime-operation>"
	payload := cloneReplayMap(message.Payload)
	payload["operationId"] = portableOperationID
	message.Payload = payload
	if message.ID == "plan-decision:"+operationID+":status" {
		message.ID = "plan-decision:" + portableOperationID + ":status"
	}
}

func projectPortableApprovalInput(input map[string]any) map[string]any {
	projectedInput := cloneReplayMap(input)
	delete(projectedInput, "cwd")
	projectPortableCommandField(projectedInput, "command")
	if toolCall, ok := projectedInput["toolCall"].(map[string]any); ok {
		projectedToolCall := cloneReplayMap(toolCall)
		projectPortableCommandField(projectedToolCall, "title")
		if toolCallInput, ok := projectedToolCall["input"].(map[string]any); ok {
			projectedToolCallInput := cloneReplayMap(toolCallInput)
			delete(projectedToolCallInput, "cwd")
			projectPortableCommandField(projectedToolCallInput, "command")
			projectedToolCall["input"] = projectedToolCallInput
		}
		projectedInput["toolCall"] = projectedToolCall
	}
	return projectedInput
}

func projectPortableCommandField(input map[string]any, key string) {
	command, ok := input[key].(string)
	if !ok {
		return
	}
	command = strings.TrimSpace(command)
	executable, arguments, hasArguments := strings.Cut(command, " ")
	if !filepath.IsAbs(executable) {
		return
	}
	projected := filepath.Base(executable)
	if hasArguments {
		projected += " " + arguments
	}
	input[key] = projected
}

func cloneReplayMap(source map[string]any) map[string]any {
	if source == nil {
		return nil
	}
	cloned := make(map[string]any, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func ValidateTuttiReplayState(state TuttiReplayState) error {
	if state.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported tutti replay state schema %d", state.SchemaVersion)
	}
	if err := agenthost.ValidateHistoricalSessionGraph(state.Agent); err != nil {
		return err
	}
	if state.TuttiMode.Activations == nil ||
		state.TuttiMode.TurnSnapshots == nil ||
		state.Workflows == nil ||
		state.Issues == nil {
		return errors.New("tutti replay state sections must be explicit arrays")
	}
	for _, workflow := range state.Workflows {
		if workflow.IssueIDs == nil {
			return fmt.Errorf(
				"tutti replay state workflow %q must have explicit issueIds",
				workflow.ID,
			)
		}
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	return validateReplayPortableValue("$", "", value)
}

func validateReplayPortableValue(path, key string, value any) error {
	switch value := value.(type) {
	case map[string]any:
		for childKey, child := range value {
			if childKey == "workspaceId" {
				return fmt.Errorf("tutti replay state contains non-portable %s.%s", path, childKey)
			}
			if err := validateReplayPortableValue(
				path+"."+childKey,
				childKey,
				child,
			); err != nil {
				return err
			}
		}
	case []any:
		for index, child := range value {
			if err := validateReplayPortableValue(
				fmt.Sprintf("%s[%d]", path, index),
				key,
				child,
			); err != nil {
				return err
			}
		}
	case string:
		lowerKey := strings.ToLower(key)
		if (strings.Contains(lowerKey, "path") || lowerKey == "cwd") &&
			(filepath.IsAbs(value) || strings.HasPrefix(value, "file://")) {
			return fmt.Errorf("tutti replay state contains absolute path at %s", path)
		}
	}
	return nil
}

func MergeTuttiReplayStates(
	states []TuttiReplayState,
) (TuttiReplayMergedState, error) {
	merged := TuttiReplayMergedState{
		Agents: []agenthost.HistoricalSessionGraph{},
		TuttiMode: TuttiReplayTuttiMode{
			Activations:   []TuttiReplayActivation{},
			TurnSnapshots: []TuttiReplayTurnSnapshot{},
		},
		Workflows: []TuttiReplayWorkflow{},
		Issues:    []TuttiReplayIssue{},
	}
	sessionObjects := map[string]any{}
	activationObjects := map[string]any{}
	snapshotObjects := map[string]any{}
	workflowObjects := map[string]any{}
	issueObjects := map[string]any{}
	for _, state := range states {
		if err := ValidateTuttiReplayState(state); err != nil {
			return TuttiReplayMergedState{}, err
		}
		for _, session := range state.Agent.Sessions {
			if err := mergeReplayObject(
				"$.agent.sessions["+session.ID+"]",
				session.ID,
				session,
				sessionObjects,
			); err != nil {
				return TuttiReplayMergedState{}, err
			}
		}
		merged.Agents = append(merged.Agents, state.Agent)
		for _, activation := range state.TuttiMode.Activations {
			if err := mergeReplayObject(
				"$.tuttiMode.activations["+activation.ID+"]",
				activation.ID,
				activation,
				activationObjects,
			); err != nil {
				return TuttiReplayMergedState{}, err
			}
		}
		for _, snapshot := range state.TuttiMode.TurnSnapshots {
			key := snapshot.SessionID + "\x00" + snapshot.TurnID
			if err := mergeReplayObject(
				"$.tuttiMode.turnSnapshots["+snapshot.SessionID+"/"+snapshot.TurnID+"]",
				key,
				snapshot,
				snapshotObjects,
			); err != nil {
				return TuttiReplayMergedState{}, err
			}
		}
		for _, workflow := range state.Workflows {
			if err := mergeReplayObject(
				"$.workflows["+workflow.ID+"]",
				workflow.ID,
				workflow,
				workflowObjects,
			); err != nil {
				return TuttiReplayMergedState{}, err
			}
		}
		for _, issue := range state.Issues {
			if err := mergeReplayObject(
				"$.issues["+issue.ID+"]",
				issue.ID,
				issue,
				issueObjects,
			); err != nil {
				return TuttiReplayMergedState{}, err
			}
		}
	}
	merged.TuttiMode.Activations = replayObjectValues[TuttiReplayActivation](activationObjects)
	merged.TuttiMode.TurnSnapshots = replayObjectValues[TuttiReplayTurnSnapshot](snapshotObjects)
	merged.Workflows = replayObjectValues[TuttiReplayWorkflow](workflowObjects)
	merged.Issues = replayObjectValues[TuttiReplayIssue](issueObjects)
	sort.Slice(merged.Agents, func(i, j int) bool {
		return merged.Agents[i].RootSessionID < merged.Agents[j].RootSessionID
	})
	return merged, nil
}

func mergeReplayObject(
	path, key string,
	value any,
	objects map[string]any,
) error {
	if existing, ok := objects[key]; ok {
		mismatch := firstReplayStateMismatch(path, existing, value)
		if mismatch != "" {
			return &TuttiReplayStateConflictError{Path: mismatch}
		}
		return nil
	}
	objects[key] = value
	return nil
}

func replayObjectValues[T any](objects map[string]any) []T {
	keys := make([]string, 0, len(objects))
	for key := range objects {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]T, 0, len(keys))
	for _, key := range keys {
		result = append(result, objects[key].(T))
	}
	return result
}

func CompareTuttiReplayState(expected, actual TuttiReplayState) error {
	if err := ValidateTuttiReplayState(expected); err != nil {
		return fmt.Errorf("invalid expected Tutti Replay State: %w", err)
	}
	if err := ValidateTuttiReplayState(actual); err != nil {
		return fmt.Errorf("invalid actual Tutti Replay State: %w", err)
	}
	expected = normalizeReplayStateForComparison(expected)
	actual = normalizeReplayStateForComparison(actual)
	if mismatch := firstReplayStateMismatch("$", expected, actual); mismatch != "" {
		return &TuttiReplayStateConflictError{Path: mismatch}
	}
	return nil
}

// normalizeReplayStateForComparison preserves relationships while replacing
// runtime-generated identifiers with stable structural names. Historical
// restore still receives the original identifiers; only final-state
// verification treats alpha-equivalent graphs as the same semantic state.
func normalizeReplayStateForComparison(state TuttiReplayState) TuttiReplayState {
	raw, _ := json.Marshal(state)
	var value map[string]any
	_ = json.Unmarshal(raw, &value)

	replacements := map[string]string{}
	registerReplayIDs(replacements, value)
	replaceReplayIDs(value, replacements)

	normalized, _ := json.Marshal(value)
	var result TuttiReplayState
	_ = json.Unmarshal(normalized, &result)
	return result
}

func registerReplayIDs(replacements map[string]string, value map[string]any) {
	agent, _ := value["agent"].(map[string]any)
	sessions, _ := agent["sessions"].([]any)
	for sessionIndex, item := range sessions {
		session, _ := item.(map[string]any)
		registerReplayID(replacements, session["id"], fmt.Sprintf("session:%d", sessionIndex))
		turns, _ := session["turns"].([]any)
		for turnIndex, turnItem := range turns {
			turn, _ := turnItem.(map[string]any)
			registerReplayID(
				replacements,
				turn["id"],
				fmt.Sprintf("session:%d/turn:%d", sessionIndex, turnIndex),
			)
		}
		messages, _ := session["messages"].([]any)
		for messageIndex, messageItem := range messages {
			message, _ := messageItem.(map[string]any)
			registerReplayID(
				replacements,
				message["id"],
				fmt.Sprintf("session:%d/message:%d", sessionIndex, messageIndex),
			)
			if payload, ok := message["payload"].(map[string]any); ok {
				delete(payload, "seq")
				// Goal-control audit rows and similar durable notices mint a
				// fresh operationId on every run; treat it as alpha-equivalent
				// like attachment/session IDs so record→replay can compare.
				registerReplayID(
					replacements,
					payload["operationId"],
					fmt.Sprintf(
						"session:%d/message:%d/operation",
						sessionIndex,
						messageIndex,
					),
				)
				content, _ := payload["content"].([]any)
				for contentIndex, contentItem := range content {
					block, _ := contentItem.(map[string]any)
					registerReplayID(
						replacements,
						block["attachmentId"],
						fmt.Sprintf(
							"session:%d/message:%d/attachment:%d",
							sessionIndex,
							messageIndex,
							contentIndex,
						),
					)
				}
			}
		}
		interactions, _ := session["interactions"].([]any)
		for interactionIndex, interactionItem := range interactions {
			interaction, _ := interactionItem.(map[string]any)
			registerReplayID(
				replacements,
				interaction["requestId"],
				fmt.Sprintf("session:%d/interaction:%d", sessionIndex, interactionIndex),
			)
		}
	}
	tuttiMode, _ := value["tuttiMode"].(map[string]any)
	registerReplayArrayIDs(replacements, tuttiMode["activations"], "activation")
	registerReplayArrayIDs(replacements, value["workflows"], "workflow")
	registerReplayArrayIDs(replacements, value["issues"], "issue")
	if issues, ok := value["issues"].([]any); ok {
		for issueIndex, item := range issues {
			issue, _ := item.(map[string]any)
			tasks, _ := issue["tasks"].([]any)
			for taskIndex, taskItem := range tasks {
				task, _ := taskItem.(map[string]any)
				registerReplayID(
					replacements,
					task["id"],
					fmt.Sprintf("issue:%d/task:%d", issueIndex, taskIndex),
				)
			}
		}
	}
}

func registerReplayArrayIDs(
	replacements map[string]string,
	value any,
	prefix string,
) {
	items, _ := value.([]any)
	for index, item := range items {
		object, _ := item.(map[string]any)
		registerReplayID(replacements, object["id"], fmt.Sprintf("%s:%d", prefix, index))
	}
}

func registerReplayID(replacements map[string]string, value any, replacement string) {
	id, ok := value.(string)
	if ok && id != "" {
		replacements[id] = replacement
	}
}

func replaceReplayIDs(value any, replacements map[string]string) {
	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			if text, ok := child.(string); ok {
				if replacement, exists := replacements[text]; exists {
					value[key] = replacement
				}
				continue
			}
			replaceReplayIDs(child, replacements)
		}
	case []any:
		for _, child := range value {
			replaceReplayIDs(child, replacements)
		}
	}
}

func firstReplayStateMismatch(path string, expected, actual any) string {
	expectedValue := replayComparableValue(expected)
	actualValue := replayComparableValue(actual)
	if expectedValue == nil || actualValue == nil {
		if expectedValue == nil && actualValue == nil {
			return ""
		}
		return path
	}
	expectedType := reflect.TypeOf(expectedValue)
	actualType := reflect.TypeOf(actualValue)
	if expectedType != actualType {
		return path
	}
	switch expectedValue := expectedValue.(type) {
	case map[string]any:
		actualValue := actualValue.(map[string]any)
		keys := make([]string, 0, len(expectedValue)+len(actualValue))
		seen := map[string]struct{}{}
		for key := range expectedValue {
			keys = append(keys, key)
			seen[key] = struct{}{}
		}
		for key := range actualValue {
			if _, ok := seen[key]; !ok {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		for _, key := range keys {
			expectedChild, expectedOK := expectedValue[key]
			actualChild, actualOK := actualValue[key]
			if !expectedOK || !actualOK {
				return path + "." + key
			}
			if mismatch := firstReplayStateMismatch(
				path+"."+key,
				expectedChild,
				actualChild,
			); mismatch != "" {
				return mismatch
			}
		}
	case []any:
		actualValue := actualValue.([]any)
		if len(expectedValue) != len(actualValue) {
			return path
		}
		for index := range expectedValue {
			if mismatch := firstReplayStateMismatch(
				fmt.Sprintf("%s[%d]", path, index),
				expectedValue[index],
				actualValue[index],
			); mismatch != "" {
				return mismatch
			}
		}
	default:
		if !reflect.DeepEqual(expectedValue, actualValue) {
			return path
		}
	}
	return ""
}

func replayComparableValue(value any) any {
	raw, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return value
	}
	return decoded
}
