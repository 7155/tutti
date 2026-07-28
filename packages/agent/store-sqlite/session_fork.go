//revive:disable:file-length-limit

package storesqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type sessionForkTurnSnapshot struct {
	Sequence int64 `json:"sequence"`
	Turn     Turn  `json:"turn"`
}

type sessionForkSnapshot struct {
	Version              int                       `json:"version"`
	BoundaryMessageID    int64                     `json:"boundaryMessageId"`
	TargetCwd            string                    `json:"targetCwd,omitempty"`
	TargetRuntimeContext map[string]any            `json:"targetRuntimeContext,omitempty"`
	TargetSettings       map[string]any            `json:"targetSettings,omitempty"`
	TargetTitle          string                    `json:"targetTitle,omitempty"`
	Session              Session                   `json:"session"`
	Turns                []sessionForkTurnSnapshot `json:"turns"`
	Messages             []Message                 `json:"messages"`
	Interactions         []Interaction             `json:"interactions"`
}

// SessionForkSourceHash fingerprints the complete canonical source row read by
// Host before product context policy and provider capability resolution. The
// Store compares it again inside Prepare's transaction, before installing the
// source fence, so provider identity and host-owned context cannot drift
// across that gap.
func SessionForkSourceHash(session Session) (string, error) {
	encoded, err := json.Marshal(session)
	if err != nil {
		return "", fmt.Errorf("encode session fork source proof: %w", err)
	}
	return hashSessionForkBytes(encoded), nil
}

const sessionForkOperationSelectSQL = `
SELECT operation_id, workspace_id, request_id, request_hash,
       source_agent_session_id, target_agent_session_id,
       source_provider_session_id, source_turn_id, source_provider_turn_id,
       COALESCE(target_turn_id, ''),
       point_kind, driver_kind, driver_version, status,
       COALESCE(target_provider_session_id, ''), snapshot_hash, last_error,
       created_at_unix_ms, updated_at_unix_ms,
       COALESCE(dispatched_at_unix_ms, 0), COALESCE(accepted_at_unix_ms, 0),
       COALESCE(completed_at_unix_ms, 0),
       COALESCE(client_observed_at_unix_ms, 0)
FROM workspace_agent_session_fork_operations`

func (s *Store) PrepareSessionFork(ctx context.Context, input SessionForkPrepare) (SessionForkOperation, bool, error) {
	if s == nil || s.db == nil {
		return SessionForkOperation{}, false, errors.New("workspace database is not initialized")
	}
	normalizeSessionForkPrepare(&input)
	if input.OperationID == "" || input.WorkspaceID == "" || input.RequestID == "" ||
		input.RequestHash == "" || input.SourceAgentSessionID == "" ||
		input.TargetAgentSessionID == "" || input.SourceTurnID == "" ||
		input.PointKind != SessionForkPointThroughTurn ||
		input.DriverKind == "" || input.DriverVersion == "" || input.OccurredAtUnixMS <= 0 ||
		input.ExpectedSourceHash == "" ||
		input.SourceAgentSessionID == input.TargetAgentSessionID {
		return SessionForkOperation{}, false, errors.New("valid session fork prepare input is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SessionForkOperation{}, false, fmt.Errorf("begin prepare session fork: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if existing, found, err := getSessionForkOperationByRequestTx(ctx, tx, input.WorkspaceID, input.RequestID); err != nil {
		return SessionForkOperation{}, false, err
	} else if found {
		if existing.RequestHash != input.RequestHash {
			return SessionForkOperation{}, false, ErrSessionForkRequestConflict
		}
		if _, err := s.commitTransaction(ctx, tx, input.WorkspaceID, nil); err != nil {
			return SessionForkOperation{}, false, err
		}
		return existing, false, nil
	}
	if existing, found, err := getSessionForkBoundaryBarrierTx(
		ctx,
		tx,
		input.WorkspaceID,
		input.SourceAgentSessionID,
		input.PointKind,
		input.SourceTurnID,
	); err != nil {
		return SessionForkOperation{}, false, err
	} else if found {
		if _, err := s.commitTransaction(ctx, tx, input.WorkspaceID, nil); err != nil {
			return SessionForkOperation{}, false, err
		}
		return existing, false, nil
	}
	if err := requireSessionForkSourceWritableTx(
		ctx, tx, input.WorkspaceID, input.SourceAgentSessionID,
	); err != nil {
		return SessionForkOperation{}, false, err
	}

	source, found, err := getSessionForkSourceTx(ctx, tx, input.WorkspaceID, input.SourceAgentSessionID)
	if err != nil {
		return SessionForkOperation{}, false, err
	}
	if !found || source.Kind != SessionKindRoot || source.ActiveTurnID != "" ||
		strings.TrimSpace(source.ProviderSessionID) == "" {
		return SessionForkOperation{}, false, ErrSessionForkSourceState
	}
	sourceHash, err := SessionForkSourceHash(source)
	if err != nil {
		return SessionForkOperation{}, false, err
	}
	if sourceHash != input.ExpectedSourceHash {
		return SessionForkOperation{}, false, ErrSessionForkSourceState
	}
	var pendingInteractions int
	if err := tx.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1 FROM workspace_agent_interactions
  WHERE workspace_id = ? AND agent_session_id = ? AND status = ?
)
`, input.WorkspaceID, input.SourceAgentSessionID, InteractionStatusPending).Scan(&pendingInteractions); err != nil {
		return SessionForkOperation{}, false, fmt.Errorf("read pending session fork interactions: %w", err)
	}
	if pendingInteractions != 0 {
		return SessionForkOperation{}, false, ErrSessionForkSourceState
	}
	if err := requireSessionForkSourceQuiescentTx(
		ctx, tx, input.WorkspaceID, input.SourceAgentSessionID,
	); err != nil {
		return SessionForkOperation{}, false, err
	}
	var selectedSequence int64
	var selectedProvenance string
	selected, found, err := getAgentTurnTx(ctx, tx, input.WorkspaceID, input.SourceAgentSessionID, input.SourceTurnID)
	if err != nil {
		return SessionForkOperation{}, false, err
	}
	if !found {
		return SessionForkOperation{}, false, ErrSessionForkTurnState
	}
	if err := tx.QueryRowContext(ctx, `
SELECT turn_sequence, provenance
FROM workspace_agent_turn_sequences
WHERE workspace_id = ? AND agent_session_id = ? AND turn_id = ?
`, input.WorkspaceID, input.SourceAgentSessionID, input.SourceTurnID).Scan(&selectedSequence, &selectedProvenance); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SessionForkOperation{}, false, ErrSessionForkTurnState
		}
		return SessionForkOperation{}, false, fmt.Errorf("read session fork turn sequence: %w", err)
	}
	if selected.Phase != TurnPhaseSettled || !isVerifiedSessionForkSequence(selectedProvenance) ||
		strings.TrimSpace(selected.RootProviderTurnID) == "" {
		return SessionForkOperation{}, false, ErrSessionForkTurnState
	}
	if descendants, err := hasSessionForkDescendantsTx(
		ctx, tx, input.WorkspaceID, input.SourceAgentSessionID, selectedSequence,
	); err != nil {
		return SessionForkOperation{}, false, err
	} else if descendants {
		return SessionForkOperation{}, false, ErrSessionForkTurnState
	}
	snapshot, err := loadSessionForkSnapshotTx(ctx, tx, source, selectedSequence, 0)
	if err != nil {
		return SessionForkOperation{}, false, err
	}
	identityOperation := SessionForkOperation{
		OperationID:          input.OperationID,
		WorkspaceID:          input.WorkspaceID,
		SourceAgentSessionID: input.SourceAgentSessionID,
		TargetAgentSessionID: input.TargetAgentSessionID,
	}
	if _, err := buildSessionForkCanonicalIdentityMap(identityOperation, snapshot); err != nil {
		return SessionForkOperation{}, false, errors.Join(ErrSessionForkTurnState, err)
	}
	if sessionForkSnapshotHasAttachmentReference(snapshot) {
		// Session-scoped local attachments do not yet have an immutable,
		// through-Turn resource manifest. Never copy the whole source
		// namespace because it can contain resources created after the
		// selected boundary.
		return SessionForkOperation{}, false, ErrSessionForkTurnState
	}
	snapshot.TargetCwd = strings.TrimSpace(input.TargetCwd)
	snapshot.TargetRuntimeContext = cloneJSONMap(input.TargetRuntimeContext)
	snapshot.TargetSettings = cloneJSONMap(input.TargetSettings)
	snapshot.TargetTitle, err = nextSessionForkTargetTitleTx(
		ctx,
		tx,
		input.WorkspaceID,
		input.SourceAgentSessionID,
		source.Title,
	)
	if err != nil {
		return SessionForkOperation{}, false, err
	}
	snapshot.Version = 2
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return SessionForkOperation{}, false, fmt.Errorf("encode session fork snapshot: %w", err)
	}
	snapshotHash := hashSessionForkBytes(snapshotJSON)
	if len(snapshot.Turns) == 0 || snapshot.Turns[len(snapshot.Turns)-1].Turn.TurnID != input.SourceTurnID {
		return SessionForkOperation{}, false, ErrSessionForkTurnState
	}

	var targetExists int
	if err := tx.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1 FROM workspace_agent_sessions
  WHERE workspace_id = ? AND agent_session_id = ?
  UNION ALL
  SELECT 1 FROM workspace_agent_session_fork_target_reservations
  WHERE workspace_id = ? AND target_agent_session_id = ?
)
`, input.WorkspaceID, input.TargetAgentSessionID, input.WorkspaceID, input.TargetAgentSessionID).Scan(&targetExists); err != nil {
		return SessionForkOperation{}, false, fmt.Errorf("read session fork target identity: %w", err)
	}
	if targetExists != 0 {
		return SessionForkOperation{}, false, ErrSessionForkTargetReserved
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO workspace_agent_session_fork_operations (
  operation_id, workspace_id, request_id, request_hash,
  source_agent_session_id, target_agent_session_id,
  source_provider_session_id, source_turn_id, source_provider_turn_id,
  point_kind, driver_kind, driver_version, status, snapshot_json, snapshot_hash,
  created_at_unix_ms, updated_at_unix_ms
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, input.OperationID, input.WorkspaceID, input.RequestID, input.RequestHash,
		input.SourceAgentSessionID, input.TargetAgentSessionID,
		source.ProviderSessionID, input.SourceTurnID, selected.RootProviderTurnID,
		input.PointKind, input.DriverKind, input.DriverVersion, SessionForkStatusPrepared,
		string(snapshotJSON), snapshotHash, input.OccurredAtUnixMS, input.OccurredAtUnixMS); err != nil {
		return SessionForkOperation{}, false, fmt.Errorf("insert session fork operation: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO workspace_agent_session_fork_target_reservations (
  workspace_id, target_agent_session_id, operation_id, request_id, request_hash, created_at_unix_ms
) VALUES (?, ?, ?, ?, ?, ?)
`, input.WorkspaceID, input.TargetAgentSessionID, input.OperationID,
		input.RequestID, input.RequestHash, input.OccurredAtUnixMS); err != nil {
		return SessionForkOperation{}, false, fmt.Errorf("reserve session fork target: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO workspace_agent_session_fork_boundary_barriers (
  workspace_id, source_agent_session_id, point_kind, source_turn_id,
  operation_id, created_at_unix_ms
) VALUES (?, ?, ?, ?, ?, ?)
`, input.WorkspaceID, input.SourceAgentSessionID, input.PointKind,
		input.SourceTurnID, input.OperationID, input.OccurredAtUnixMS); err != nil {
		return SessionForkOperation{}, false, fmt.Errorf(
			"install session fork boundary barrier: %w",
			err,
		)
	}
	op, _, err := getSessionForkOperationTx(ctx, tx, input.WorkspaceID, input.OperationID)
	if err != nil {
		return SessionForkOperation{}, false, err
	}
	delta, err := s.commitTransaction(ctx, tx, input.WorkspaceID, []TransactionMutation{
		transactionMutation(input.WorkspaceID, input.SourceAgentSessionID, MutationEntitySessionForkOperation, input.OperationID, "prepare", input.OccurredAtUnixMS),
	})
	if err != nil {
		return SessionForkOperation{}, false, fmt.Errorf("commit prepare session fork: %w", err)
	}
	op.CommitTransactionID, op.CommitDelta = delta.TransactionID, delta
	return op, true, nil
}

func (s *Store) GetSessionForkOperation(ctx context.Context, workspaceID, operationID string) (SessionForkOperation, bool, error) {
	if s == nil || s.db == nil {
		return SessionForkOperation{}, false, errors.New("workspace database is not initialized")
	}
	return getSessionForkOperation(ctx, s.db, strings.TrimSpace(workspaceID), strings.TrimSpace(operationID))
}

func (s *Store) GetSessionForkSource(
	ctx context.Context,
	workspaceID, sourceSessionID string,
) (Session, bool, error) {
	if s == nil || s.db == nil {
		return Session{}, false, errors.New("workspace database is not initialized")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	sourceSessionID = strings.TrimSpace(sourceSessionID)
	if workspaceID == "" || sourceSessionID == "" {
		return Session{}, false, nil
	}
	session, found, err := s.GetSession(ctx, workspaceID, sourceSessionID)
	if err != nil || !found {
		return Session{}, found, err
	}
	if session.Kind != SessionKindRoot || strings.TrimSpace(session.ProviderSessionID) == "" {
		return Session{}, false, nil
	}
	return session, true, nil
}

func (s *Store) GetSessionForkOperationByRequest(
	ctx context.Context,
	workspaceID, requestID string,
) (SessionForkOperation, bool, error) {
	if s == nil || s.db == nil {
		return SessionForkOperation{}, false, errors.New("workspace database is not initialized")
	}
	op, err := scanSessionForkOperation(s.db.QueryRowContext(ctx, sessionForkOperationSelectSQL+`
WHERE workspace_id = ? AND request_id = ?`,
		strings.TrimSpace(workspaceID), strings.TrimSpace(requestID)))
	if errors.Is(err, sql.ErrNoRows) {
		return SessionForkOperation{}, false, nil
	}
	return op, err == nil, err
}

func (s *Store) GetUnknownSessionForkOperation(
	ctx context.Context,
	workspaceID, sourceSessionID, pointKind, sourceTurnID string,
) (SessionForkOperation, bool, error) {
	if s == nil || s.db == nil {
		return SessionForkOperation{}, false, errors.New(
			"workspace database is not initialized",
		)
	}
	workspaceID = strings.TrimSpace(workspaceID)
	sourceSessionID = strings.TrimSpace(sourceSessionID)
	pointKind = strings.TrimSpace(pointKind)
	sourceTurnID = strings.TrimSpace(sourceTurnID)
	if workspaceID == "" || sourceSessionID == "" ||
		pointKind != SessionForkPointThroughTurn || sourceTurnID == "" {
		return SessionForkOperation{}, false, errors.New(
			"valid unknown session fork lookup is required",
		)
	}
	op, err := scanSessionForkOperation(s.db.QueryRowContext(
		ctx,
		sessionForkOperationSelectSQL+`
WHERE workspace_id = ?
  AND source_agent_session_id = ?
  AND point_kind = ?
  AND source_turn_id = ?
  AND status = ?
ORDER BY created_at_unix_ms DESC, operation_id DESC
LIMIT 1`,
		workspaceID,
		sourceSessionID,
		pointKind,
		sourceTurnID,
		SessionForkStatusUnknown,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return SessionForkOperation{}, false, nil
	}
	return op, err == nil, err
}

func (s *Store) GetBlockingSessionForkOperation(
	ctx context.Context,
	workspaceID, sourceSessionID, pointKind, sourceTurnID string,
) (SessionForkOperation, bool, error) {
	if s == nil || s.db == nil {
		return SessionForkOperation{}, false, errors.New(
			"workspace database is not initialized",
		)
	}
	workspaceID = strings.TrimSpace(workspaceID)
	sourceSessionID = strings.TrimSpace(sourceSessionID)
	pointKind = strings.TrimSpace(pointKind)
	sourceTurnID = strings.TrimSpace(sourceTurnID)
	if workspaceID == "" || sourceSessionID == "" ||
		pointKind != SessionForkPointThroughTurn || sourceTurnID == "" {
		return SessionForkOperation{}, false, errors.New(
			"valid session fork boundary barrier lookup is required",
		)
	}
	var operationID string
	err := s.db.QueryRowContext(ctx, `
SELECT operation_id
FROM workspace_agent_session_fork_boundary_barriers
WHERE workspace_id = ?
  AND source_agent_session_id = ?
  AND point_kind = ?
  AND source_turn_id = ?
`, workspaceID, sourceSessionID, pointKind, sourceTurnID).Scan(&operationID)
	if errors.Is(err, sql.ErrNoRows) {
		return SessionForkOperation{}, false, nil
	}
	if err != nil {
		return SessionForkOperation{}, false, fmt.Errorf(
			"read session fork boundary barrier: %w",
			err,
		)
	}
	return getSessionForkOperation(ctx, s.db, workspaceID, operationID)
}

// CheckSessionForkThroughTurn performs the fail-closed, read-only half of
// PrepareSessionFork. PrepareSessionFork repeats every check transactionally.
func (s *Store) CheckSessionForkThroughTurn(
	ctx context.Context,
	workspaceID, sourceSessionID, throughTurnID string,
) (SessionForkBoundary, bool, error) {
	if s == nil || s.db == nil {
		return SessionForkBoundary{}, false, errors.New("workspace database is not initialized")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	sourceSessionID = strings.TrimSpace(sourceSessionID)
	throughTurnID = strings.TrimSpace(throughTurnID)
	if workspaceID == "" || sourceSessionID == "" || throughTurnID == "" {
		return SessionForkBoundary{}, false, nil
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return SessionForkBoundary{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	session, found, err := getSessionForkSourceTx(ctx, tx, workspaceID, sourceSessionID)
	if err != nil || !found {
		return SessionForkBoundary{}, false, err
	}
	if session.Kind != SessionKindRoot || session.ActiveTurnID != "" ||
		strings.TrimSpace(session.ProviderSessionID) == "" {
		return SessionForkBoundary{}, false, nil
	}
	var pendingInteractions int
	if err := tx.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1 FROM workspace_agent_interactions
  WHERE workspace_id = ? AND agent_session_id = ? AND status = ?
)
`, workspaceID, sourceSessionID, InteractionStatusPending).Scan(&pendingInteractions); err != nil {
		return SessionForkBoundary{}, false, err
	}
	if pendingInteractions != 0 {
		return SessionForkBoundary{}, false, nil
	}
	if err := requireSessionForkSourceQuiescentTx(ctx, tx, workspaceID, sourceSessionID); err != nil {
		if errors.Is(err, ErrSessionForkSourceState) {
			return SessionForkBoundary{}, false, nil
		}
		return SessionForkBoundary{}, false, err
	}
	turn, found, err := getAgentTurnTx(ctx, tx, workspaceID, sourceSessionID, throughTurnID)
	if err != nil || !found {
		return SessionForkBoundary{}, false, err
	}
	var sequence int64
	var provenance string
	if err := tx.QueryRowContext(ctx, `
SELECT turn_sequence, provenance FROM workspace_agent_turn_sequences
WHERE workspace_id = ? AND agent_session_id = ? AND turn_id = ?
`, workspaceID, sourceSessionID, throughTurnID).Scan(&sequence, &provenance); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SessionForkBoundary{}, false, nil
		}
		return SessionForkBoundary{}, false, err
	}
	if turn.Phase != TurnPhaseSettled || !isVerifiedSessionForkSequence(provenance) ||
		strings.TrimSpace(turn.RootProviderTurnID) == "" {
		return SessionForkBoundary{}, false, nil
	}
	var invalidPrefix int
	if err := tx.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1
  FROM workspace_agent_turn_sequences sequence
  LEFT JOIN workspace_agent_turns turn
    ON turn.workspace_id = sequence.workspace_id
   AND turn.agent_session_id = sequence.agent_session_id
   AND turn.turn_id = sequence.turn_id
  WHERE sequence.workspace_id = ?
    AND sequence.agent_session_id = ?
    AND sequence.turn_sequence <= ?
    AND (
      sequence.provenance NOT IN ('verified','fork_clone_verified')
      OR turn.turn_id IS NULL
      OR turn.phase <> ?
      OR COALESCE(turn.root_provider_turn_id, '') = ''
    )
)
`, workspaceID, sourceSessionID, sequence, TurnPhaseSettled).Scan(&invalidPrefix); err != nil {
		return SessionForkBoundary{}, false, err
	}
	if invalidPrefix != 0 {
		return SessionForkBoundary{}, false, nil
	}
	providerTurnRows, err := tx.QueryContext(ctx, `
SELECT turn.root_provider_turn_id
FROM workspace_agent_turn_sequences sequence
JOIN workspace_agent_turns turn
  ON turn.workspace_id = sequence.workspace_id
 AND turn.agent_session_id = sequence.agent_session_id
 AND turn.turn_id = sequence.turn_id
WHERE sequence.workspace_id = ?
  AND sequence.agent_session_id = ?
  AND sequence.turn_sequence <= ?
ORDER BY sequence.turn_sequence
`, workspaceID, sourceSessionID, sequence)
	if err != nil {
		return SessionForkBoundary{}, false, err
	}
	rootProviderTurnIDs := make([]string, 0)
	seenRootProviderTurnIDs := make(map[string]struct{})
	for providerTurnRows.Next() {
		var providerTurnID string
		if err := providerTurnRows.Scan(&providerTurnID); err != nil {
			providerTurnRows.Close()
			return SessionForkBoundary{}, false, err
		}
		providerTurnID = strings.TrimSpace(providerTurnID)
		if providerTurnID == "" {
			providerTurnRows.Close()
			return SessionForkBoundary{}, false, nil
		}
		if _, exists := seenRootProviderTurnIDs[providerTurnID]; exists {
			providerTurnRows.Close()
			return SessionForkBoundary{}, false, nil
		}
		seenRootProviderTurnIDs[providerTurnID] = struct{}{}
		rootProviderTurnIDs = append(rootProviderTurnIDs, providerTurnID)
	}
	if err := providerTurnRows.Err(); err != nil {
		providerTurnRows.Close()
		return SessionForkBoundary{}, false, err
	}
	if err := providerTurnRows.Close(); err != nil {
		return SessionForkBoundary{}, false, err
	}
	if len(rootProviderTurnIDs) == 0 ||
		rootProviderTurnIDs[len(rootProviderTurnIDs)-1] != strings.TrimSpace(turn.RootProviderTurnID) {
		return SessionForkBoundary{}, false, nil
	}
	if descendants, err := hasSessionForkDescendantsTx(
		ctx, tx, workspaceID, sourceSessionID, sequence,
	); err != nil {
		return SessionForkBoundary{}, false, err
	} else if descendants {
		return SessionForkBoundary{}, false, nil
	}
	var boundaryMessageID int64
	if err := tx.QueryRowContext(ctx, `
SELECT COALESCE(MAX(message.id), 0)
FROM workspace_agent_messages message
JOIN workspace_agent_turn_sequences sequence
  ON sequence.workspace_id = message.workspace_id
 AND sequence.agent_session_id = message.agent_session_id
 AND sequence.turn_id = message.turn_id
WHERE message.workspace_id = ?
  AND message.agent_session_id = ?
  AND message.deleted_at_unix_ms = 0
  AND sequence.turn_sequence <= ?
`, workspaceID, sourceSessionID, sequence).Scan(&boundaryMessageID); err != nil {
		return SessionForkBoundary{}, false, err
	}
	if boundaryMessageID <= 0 {
		return SessionForkBoundary{}, false, nil
	}
	var unsupportedTurnless int
	if err := tx.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1
  FROM workspace_agent_messages
  WHERE workspace_id = ?
    AND agent_session_id = ?
    AND deleted_at_unix_ms = 0
    AND id <= ?
    AND turn_id IS NULL
    AND kind <> 'session_audit'
)
`, workspaceID, sourceSessionID, boundaryMessageID).Scan(&unsupportedTurnless); err != nil {
		return SessionForkBoundary{}, false, err
	}
	if unsupportedTurnless != 0 {
		return SessionForkBoundary{}, false, nil
	}
	var attachmentReferences int
	if err := tx.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1
  FROM workspace_agent_messages message
  JOIN json_tree(message.payload_json) payload_node
  WHERE message.workspace_id = ?
    AND message.agent_session_id = ?
    AND message.deleted_at_unix_ms = 0
    AND (
      (
        message.turn_id IS NULL
        AND message.kind = 'session_audit'
        AND message.id <= ?
      )
      OR EXISTS (
        SELECT 1
        FROM workspace_agent_turn_sequences sequence
        WHERE sequence.workspace_id = message.workspace_id
          AND sequence.agent_session_id = message.agent_session_id
          AND sequence.turn_id = message.turn_id
          AND sequence.turn_sequence <= ?
      )
    )
    AND payload_node.key = 'attachmentId'
    AND payload_node.type = 'text'
    AND TRIM(CAST(payload_node.value AS TEXT)) <> ''
)
`, workspaceID, sourceSessionID, boundaryMessageID, sequence).Scan(&attachmentReferences); err != nil {
		return SessionForkBoundary{}, false, err
	}
	if attachmentReferences != 0 {
		return SessionForkBoundary{}, false, nil
	}
	return SessionForkBoundary{
		Session:             session,
		Turn:                turn,
		RootProviderTurnIDs: rootProviderTurnIDs,
	}, true, nil
}

// ListSessionForkTurnIdentities returns the canonical/provider Turn identity
// sequence used to intersect provider-native thread history with UI actions.
// Final boundary eligibility is still rechecked transactionally by
// CheckSessionForkThroughTurn and PrepareSessionFork.
func (s *Store) ListSessionForkTurnIdentities(
	ctx context.Context,
	workspaceID, sourceSessionID string,
) ([]SessionForkTurnIdentity, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("workspace database is not initialized")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	sourceSessionID = strings.TrimSpace(sourceSessionID)
	if workspaceID == "" || sourceSessionID == "" {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT t.turn_id, COALESCE(t.root_provider_turn_id, ''), t.phase
FROM workspace_agent_turn_sequences AS sequence
JOIN workspace_agent_turns AS t
  ON t.workspace_id = sequence.workspace_id
 AND t.agent_session_id = sequence.agent_session_id
 AND t.turn_id = sequence.turn_id
WHERE sequence.workspace_id = ? AND sequence.agent_session_id = ?
ORDER BY sequence.turn_sequence ASC
`, workspaceID, sourceSessionID)
	if err != nil {
		return nil, fmt.Errorf("list session fork turn identities: %w", err)
	}
	defer rows.Close()
	identities := make([]SessionForkTurnIdentity, 0)
	for rows.Next() {
		var identity SessionForkTurnIdentity
		if err := rows.Scan(
			&identity.TurnID,
			&identity.ProviderTurnID,
			&identity.Phase,
		); err != nil {
			return nil, fmt.Errorf("scan session fork turn identity: %w", err)
		}
		identities = append(identities, identity)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate session fork turn identities: %w", err)
	}
	return identities, nil
}

func hasSessionForkDescendantsTx(
	ctx context.Context,
	tx *sql.Tx,
	workspaceID, sourceSessionID string,
	throughSequence int64,
) (bool, error) {
	var descendants int
	if err := tx.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1
  FROM workspace_agent_sessions child
  JOIN workspace_agent_turn_sequences root_sequence
    ON root_sequence.workspace_id = child.workspace_id
   AND root_sequence.agent_session_id = child.root_agent_session_id
   AND root_sequence.turn_id = child.root_turn_id
  WHERE child.workspace_id = ?
    AND child.session_kind = ?
    AND child.root_agent_session_id = ?
    AND child.deleted_at_unix_ms = 0
    AND root_sequence.turn_sequence <= ?
)
`, workspaceID, SessionKindChild, sourceSessionID, throughSequence).Scan(&descendants); err != nil {
		return false, fmt.Errorf("read session fork descendant lanes: %w", err)
	}
	return descendants != 0, nil
}

func sessionForkSnapshotHasAttachmentReference(snapshot sessionForkSnapshot) bool {
	for _, message := range snapshot.Messages {
		if sessionForkValueHasAttachmentReference(message.Payload) {
			return true
		}
	}
	return false
}

func sessionForkValueHasAttachmentReference(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		if attachmentID, ok := typed["attachmentId"].(string); ok &&
			strings.TrimSpace(attachmentID) != "" {
			return true
		}
		for _, nested := range typed {
			if sessionForkValueHasAttachmentReference(nested) {
				return true
			}
		}
	case []any:
		for _, nested := range typed {
			if sessionForkValueHasAttachmentReference(nested) {
				return true
			}
		}
	}
	return false
}

func (s *Store) ListRecoverableSessionForkOperations(ctx context.Context, limit int) ([]SessionForkOperation, error) {
	return s.ListRecoverableSessionForkOperationsPage(ctx, SessionForkRecoveryCursor{}, limit)
}

func (s *Store) ListRecoverableSessionForkOperationsPage(
	ctx context.Context,
	after SessionForkRecoveryCursor,
	limit int,
) ([]SessionForkOperation, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("workspace database is not initialized")
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, sessionForkOperationSelectSQL+`
WHERE status IN (?, ?, ?)
  AND (
    created_at_unix_ms > ?
    OR (created_at_unix_ms = ? AND operation_id > ?)
  )
ORDER BY created_at_unix_ms, operation_id
LIMIT ?`, SessionForkStatusPrepared, SessionForkStatusDispatching,
		SessionForkStatusProviderAccepted, after.CreatedAtUnixMS,
		after.CreatedAtUnixMS, strings.TrimSpace(after.OperationID), limit)
	if err != nil {
		return nil, fmt.Errorf("list recoverable session fork operations: %w", err)
	}
	defer rows.Close()
	var result []SessionForkOperation
	for rows.Next() {
		op, err := scanSessionForkOperation(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, op)
	}
	return result, rows.Err()
}

func (s *Store) MarkSessionForkDispatching(ctx context.Context, workspaceID, operationID string, now int64) (SessionForkOperation, bool, error) {
	return s.transitionSessionFork(ctx, workspaceID, operationID, SessionForkStatusPrepared,
		SessionForkStatusDispatching, "", "", now)
}

func (s *Store) FailPreparedSessionFork(
	ctx context.Context,
	workspaceID, operationID, lastError string,
	now int64,
) (SessionForkOperation, bool, error) {
	return s.transitionSessionFork(
		ctx,
		workspaceID,
		operationID,
		SessionForkStatusPrepared,
		SessionForkStatusFailed,
		"",
		lastError,
		now,
	)
}

func (s *Store) RecordSessionForkProviderResult(ctx context.Context, input SessionForkProviderResult) (SessionForkOperation, bool, error) {
	input.WorkspaceID, input.OperationID = strings.TrimSpace(input.WorkspaceID), strings.TrimSpace(input.OperationID)
	input.TargetProviderSessionID = strings.TrimSpace(input.TargetProviderSessionID)
	input.LastError = strings.TrimSpace(input.LastError)
	switch input.Status {
	case SessionForkStatusProviderAccepted:
		if input.TargetProviderSessionID == "" {
			return SessionForkOperation{}, false, errors.New("accepted session fork requires target provider session id")
		}
	case SessionForkStatusFailed, SessionForkStatusUnknown:
	default:
		return SessionForkOperation{}, false, errors.New("invalid session fork provider result")
	}
	return s.transitionSessionFork(ctx, input.WorkspaceID, input.OperationID,
		SessionForkStatusDispatching, input.Status, input.TargetProviderSessionID,
		input.LastError, input.OccurredAtUnixMS)
}

func (s *Store) CommitSessionFork(ctx context.Context, workspaceID, operationID string, now int64) (SessionForkCommitResult, error) {
	if s == nil || s.db == nil || strings.TrimSpace(workspaceID) == "" ||
		strings.TrimSpace(operationID) == "" || now <= 0 {
		return SessionForkCommitResult{}, errors.New("valid session fork commit input is required")
	}
	workspaceID, operationID = strings.TrimSpace(workspaceID), strings.TrimSpace(operationID)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SessionForkCommitResult{}, fmt.Errorf("begin commit session fork: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	op, found, snapshotJSON, err := getSessionForkOperationWithSnapshotTx(ctx, tx, workspaceID, operationID)
	if err != nil || !found {
		return SessionForkCommitResult{}, err
	}
	if hashSessionForkBytes([]byte(snapshotJSON)) != op.SnapshotHash {
		return SessionForkCommitResult{}, errors.New("agent session fork snapshot hash mismatch")
	}
	var snapshot sessionForkSnapshot
	if err := json.Unmarshal([]byte(snapshotJSON), &snapshot); err != nil {
		return SessionForkCommitResult{}, fmt.Errorf("decode session fork snapshot: %w", err)
	}
	if snapshot.Version != 1 && snapshot.Version != 2 {
		return SessionForkCommitResult{}, fmt.Errorf(
			"unsupported session fork snapshot version %d",
			snapshot.Version,
		)
	}
	if snapshot.BoundaryMessageID <= 0 || len(snapshot.Turns) == 0 ||
		snapshot.Turns[len(snapshot.Turns)-1].Turn.TurnID != op.SourceTurnID {
		return SessionForkCommitResult{}, ErrSessionForkTurnState
	}
	if op.Status == SessionForkStatusCommitted {
		materialized := materializeSessionForkSnapshot(snapshot)
		session := sessionForkResultSession(op, materialized, op.CompletedAtUnixMS)
		lineage := sessionForkResultLineage(op, op.CompletedAtUnixMS)
		if _, err := s.commitTransaction(ctx, tx, workspaceID, nil); err != nil {
			return SessionForkCommitResult{}, err
		}
		return SessionForkCommitResult{
			Operation: op,
			Session:   session,
			Lineage:   lineage,
		}, nil
	}
	if op.Status != SessionForkStatusProviderAccepted || op.TargetProviderSessionID == "" {
		return SessionForkCommitResult{}, ErrSessionForkTransition
	}
	currentSource, found, err := getSessionForkSourceTx(
		ctx, tx, workspaceID, op.SourceAgentSessionID,
	)
	if err != nil {
		return SessionForkCommitResult{}, err
	}
	if !found || currentSource.ProviderSessionID != op.SourceProviderSessionID {
		return SessionForkCommitResult{}, ErrSessionForkSourceState
	}
	var throughSequence int64
	var currentProviderTurnID string
	if err := tx.QueryRowContext(ctx, `
SELECT sequence.turn_sequence, COALESCE(turn.root_provider_turn_id, '')
FROM workspace_agent_turn_sequences sequence
JOIN workspace_agent_turns turn
  ON turn.workspace_id = sequence.workspace_id
 AND turn.agent_session_id = sequence.agent_session_id
 AND turn.turn_id = sequence.turn_id
WHERE sequence.workspace_id = ?
  AND sequence.agent_session_id = ?
  AND sequence.turn_id = ?
`, workspaceID, op.SourceAgentSessionID, op.SourceTurnID).
		Scan(&throughSequence, &currentProviderTurnID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SessionForkCommitResult{}, ErrSessionForkTurnState
		}
		return SessionForkCommitResult{}, fmt.Errorf("re-read session fork boundary: %w", err)
	}
	if currentProviderTurnID != op.SourceProviderTurnID {
		return SessionForkCommitResult{}, ErrSessionForkTurnState
	}
	identityMap, err := buildSessionForkCanonicalIdentityMap(op, snapshot)
	if err != nil {
		return SessionForkCommitResult{}, errors.Join(ErrSessionForkTurnState, err)
	}
	op.TargetTurnID = identityMap.TurnIDs[op.SourceTurnID]
	if strings.TrimSpace(op.TargetTurnID) == "" {
		return SessionForkCommitResult{}, ErrSessionForkTurnState
	}
	currentSnapshot, err := loadSessionForkSnapshotTx(
		ctx, tx, currentSource, throughSequence, snapshot.BoundaryMessageID,
	)
	if err != nil {
		return SessionForkCommitResult{}, err
	}
	// Session display/configuration fields are intentionally frozen at
	// prepare. Re-proof only the canonical prefix plus provider identity so a
	// harmless title/pin update does not invalidate the fork.
	currentSnapshot.Session = snapshot.Session
	currentSnapshot.TargetCwd = snapshot.TargetCwd
	currentSnapshot.TargetRuntimeContext = cloneJSONMap(snapshot.TargetRuntimeContext)
	currentSnapshot.TargetSettings = cloneJSONMap(snapshot.TargetSettings)
	currentSnapshot.TargetTitle = snapshot.TargetTitle
	currentSnapshot.Version = snapshot.Version
	currentSnapshotJSON, err := json.Marshal(currentSnapshot)
	if err != nil {
		return SessionForkCommitResult{}, fmt.Errorf("encode current session fork prefix: %w", err)
	}
	if hashSessionForkBytes(currentSnapshotJSON) != op.SnapshotHash {
		return SessionForkCommitResult{}, errors.New("agent session fork source prefix changed after prepare")
	}
	var reservationOperationID string
	if err := tx.QueryRowContext(ctx, `
SELECT operation_id
FROM workspace_agent_session_fork_target_reservations
WHERE workspace_id = ? AND target_agent_session_id = ?
`, workspaceID, op.TargetAgentSessionID).Scan(&reservationOperationID); err != nil {
		return SessionForkCommitResult{}, fmt.Errorf("read session fork target reservation: %w", err)
	}
	if reservationOperationID != op.OperationID {
		return SessionForkCommitResult{}, ErrSessionForkTargetReserved
	}
	materializedSnapshot := materializeSessionForkSnapshot(snapshot)
	if err := insertForkedSessionTx(ctx, tx, op, materializedSnapshot, now); err != nil {
		return SessionForkCommitResult{}, err
	}
	mutations := []TransactionMutation{
		transactionMutation(workspaceID, op.TargetAgentSessionID, MutationEntitySession, op.TargetAgentSessionID, "insert", now),
	}
	for _, item := range snapshot.Turns {
		turn, err := remapSessionForkTurn(item.Turn, identityMap)
		if err != nil {
			return SessionForkCommitResult{}, err
		}
		if err := insertForkedTurnTx(ctx, tx, workspaceID, op.TargetAgentSessionID, turn); err != nil {
			return SessionForkCommitResult{}, err
		}
		mutations = append(mutations, transactionMutation(workspaceID, op.TargetAgentSessionID, MutationEntityTurn, turn.TurnID, "insert", turn.UpdatedAtUnixMS))
	}
	for index, message := range snapshot.Messages {
		message, err = remapSessionForkMessage(message, identityMap)
		if err != nil {
			return SessionForkCommitResult{}, err
		}
		version := uint64(index + 1)
		if err := insertForkedMessageTx(ctx, tx, workspaceID, op.TargetAgentSessionID, message, version); err != nil {
			return SessionForkCommitResult{}, err
		}
		mutations = append(mutations, transactionMutation(workspaceID, op.TargetAgentSessionID, MutationEntityMessage, message.MessageID, "insert", int64(version)))
	}
	for _, interaction := range snapshot.Interactions {
		interaction, err = remapSessionForkInteraction(interaction, identityMap)
		if err != nil {
			return SessionForkCommitResult{}, err
		}
		if err := insertForkedInteractionTx(ctx, tx, workspaceID, op.TargetAgentSessionID, interaction); err != nil {
			return SessionForkCommitResult{}, err
		}
		mutations = append(mutations, transactionMutation(
			workspaceID,
			op.TargetAgentSessionID,
			MutationEntityInteraction,
			interactionMutationEntityID(interaction.TurnID, interaction.RequestID),
			"insert",
			interaction.UpdatedAtUnixMS,
		))
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE workspace_agent_sessions
SET message_version = ?, updated_at_unix_ms = ?
WHERE workspace_id = ? AND agent_session_id = ?
`, len(snapshot.Messages), now, workspaceID, op.TargetAgentSessionID); err != nil {
		return SessionForkCommitResult{}, fmt.Errorf("finalize forked session message version: %w", err)
	}
	targetSession, found, err := getSessionForkSourceTx(
		ctx, tx, workspaceID, op.TargetAgentSessionID,
	)
	if err != nil {
		return SessionForkCommitResult{}, err
	}
	if !found {
		return SessionForkCommitResult{}, errors.New(
			"forked workspace agent session was not readable after insert",
		)
	}
	lineage := sessionForkResultLineage(op, now)
	if _, err := tx.ExecContext(ctx, `
INSERT INTO workspace_agent_session_forks (
  workspace_id, target_agent_session_id, source_agent_session_id,
  source_turn_id, target_turn_id, operation_id, forked_at_unix_ms
) VALUES (?, ?, ?, ?, ?, ?, ?)
`, lineage.WorkspaceID, lineage.TargetAgentSessionID, lineage.SourceAgentSessionID,
		lineage.SourceTurnID, lineage.TargetTurnID, lineage.OperationID,
		lineage.ForkedAtUnixMS); err != nil {
		return SessionForkCommitResult{}, fmt.Errorf("insert session fork lineage: %w", err)
	}
	result, err := tx.ExecContext(ctx, `
UPDATE workspace_agent_session_fork_operations
SET status = ?, target_turn_id = ?, completed_at_unix_ms = ?, updated_at_unix_ms = ?
WHERE workspace_id = ? AND operation_id = ? AND status = ?
`, SessionForkStatusCommitted, op.TargetTurnID, now, now, workspaceID, operationID,
		SessionForkStatusProviderAccepted)
	if err != nil {
		return SessionForkCommitResult{}, fmt.Errorf("complete session fork operation: %w", err)
	}
	changed, err := rowsWereAffected(result, "complete session fork operation")
	if err != nil || !changed {
		return SessionForkCommitResult{}, errors.Join(err, ErrSessionForkTransition)
	}
	mutations = append(mutations,
		transactionMutation(workspaceID, op.SourceAgentSessionID, MutationEntitySessionForkOperation, op.OperationID, "complete", now),
	)
	delta, err := s.commitTransaction(ctx, tx, workspaceID, mutations)
	if err != nil {
		return SessionForkCommitResult{}, fmt.Errorf("commit session fork clone: %w", err)
	}
	op.Status, op.CompletedAtUnixMS, op.UpdatedAtUnixMS = SessionForkStatusCommitted, now, now
	op.CommitTransactionID, op.CommitDelta = delta.TransactionID, delta
	return SessionForkCommitResult{
		TransactionID: delta.TransactionID, CommitDelta: delta, Operation: op,
		Session: targetSession, Lineage: lineage, Changed: true,
	}, nil
}

func (s *Store) AcknowledgeSessionForkOperation(
	ctx context.Context,
	workspaceID, operationID string,
	now int64,
) (SessionForkOperation, bool, bool, error) {
	if s == nil || s.db == nil || strings.TrimSpace(workspaceID) == "" ||
		strings.TrimSpace(operationID) == "" || now <= 0 {
		return SessionForkOperation{}, false, false, errors.New(
			"valid session fork acknowledgement input is required",
		)
	}
	workspaceID = strings.TrimSpace(workspaceID)
	operationID = strings.TrimSpace(operationID)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SessionForkOperation{}, false, false, fmt.Errorf(
			"begin acknowledge session fork: %w",
			err,
		)
	}
	defer func() { _ = tx.Rollback() }()
	op, found, err := getSessionForkOperationTx(ctx, tx, workspaceID, operationID)
	if err != nil || !found {
		return SessionForkOperation{}, found, false, err
	}
	if op.Status != SessionForkStatusCommitted {
		return op, true, false, ErrSessionForkTransition
	}
	changed := op.ClientObservedAtUnixMS == 0
	if changed {
		result, err := tx.ExecContext(ctx, `
UPDATE workspace_agent_session_fork_operations
SET client_observed_at_unix_ms = ?, updated_at_unix_ms = ?
WHERE workspace_id = ? AND operation_id = ?
  AND status = 'committed' AND client_observed_at_unix_ms IS NULL
`, now, now, workspaceID, operationID)
		if err != nil {
			return SessionForkOperation{}, true, false, fmt.Errorf(
				"acknowledge committed session fork: %w",
				err,
			)
		}
		if changed, err = rowsWereAffected(
			result,
			"acknowledge committed session fork",
		); err != nil {
			return SessionForkOperation{}, true, false, err
		}
	}
	if _, err := tx.ExecContext(ctx, `
DELETE FROM workspace_agent_session_fork_boundary_barriers
WHERE workspace_id = ? AND operation_id = ?
`, workspaceID, operationID); err != nil {
		return SessionForkOperation{}, true, false, fmt.Errorf(
			"release observed session fork boundary barrier: %w",
			err,
		)
	}
	op, found, err = getSessionForkOperationTx(ctx, tx, workspaceID, operationID)
	if err != nil || !found {
		return SessionForkOperation{}, found, false, err
	}
	mutations := []TransactionMutation(nil)
	if changed {
		mutations = append(mutations, transactionMutation(
			workspaceID,
			op.SourceAgentSessionID,
			MutationEntitySessionForkOperation,
			operationID,
			"client_observed",
			now,
		))
	}
	delta, err := s.commitTransaction(ctx, tx, workspaceID, mutations)
	if err != nil {
		return SessionForkOperation{}, true, false, err
	}
	op.CommitTransactionID, op.CommitDelta = delta.TransactionID, delta
	return op, true, changed, nil
}

func (s *Store) GetSessionForkLineage(ctx context.Context, workspaceID, targetSessionID string) (SessionForkLineage, bool, error) {
	if s == nil || s.db == nil {
		return SessionForkLineage{}, false, errors.New("workspace database is not initialized")
	}
	row := s.db.QueryRowContext(ctx, `
SELECT workspace_id, target_agent_session_id, source_agent_session_id,
       source_turn_id, target_turn_id, operation_id, forked_at_unix_ms
FROM workspace_agent_session_forks
WHERE workspace_id = ? AND target_agent_session_id = ?
`, strings.TrimSpace(workspaceID), strings.TrimSpace(targetSessionID))
	return scanSessionForkLineage(row)
}

func (s *Store) transitionSessionFork(
	ctx context.Context,
	workspaceID, operationID, fromStatus, toStatus, targetProviderSessionID, lastError string,
	now int64,
) (SessionForkOperation, bool, error) {
	if s == nil || s.db == nil || strings.TrimSpace(workspaceID) == "" ||
		strings.TrimSpace(operationID) == "" || now <= 0 {
		return SessionForkOperation{}, false, errors.New("valid session fork transition input is required")
	}
	workspaceID, operationID = strings.TrimSpace(workspaceID), strings.TrimSpace(operationID)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SessionForkOperation{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `
UPDATE workspace_agent_session_fork_operations
SET status = ?, target_provider_session_id = NULLIF(?, ''), last_error = ?,
    dispatched_at_unix_ms = CASE WHEN ? = 'dispatching' THEN ? ELSE dispatched_at_unix_ms END,
    accepted_at_unix_ms = CASE WHEN ? = 'provider_accepted' THEN ? ELSE accepted_at_unix_ms END,
    completed_at_unix_ms = CASE WHEN ? IN ('failed','unknown') THEN ? ELSE completed_at_unix_ms END,
    updated_at_unix_ms = ?
WHERE workspace_id = ? AND operation_id = ? AND status = ?
`, toStatus, strings.TrimSpace(targetProviderSessionID), strings.TrimSpace(lastError),
		toStatus, now, toStatus, now, toStatus, now, now, workspaceID, operationID, fromStatus)
	if err != nil {
		return SessionForkOperation{}, false, fmt.Errorf("transition session fork operation: %w", err)
	}
	changed, err := rowsWereAffected(result, "transition session fork operation")
	if err != nil {
		return SessionForkOperation{}, false, err
	}
	op, found, err := getSessionForkOperationTx(ctx, tx, workspaceID, operationID)
	if err != nil || !found {
		return SessionForkOperation{}, false, err
	}
	if !changed && op.Status != toStatus {
		return op, false, ErrSessionForkTransition
	}
	if changed && toStatus == SessionForkStatusFailed {
		if _, err := tx.ExecContext(ctx, `
DELETE FROM workspace_agent_session_fork_target_reservations
WHERE workspace_id = ? AND operation_id = ?
`, workspaceID, operationID); err != nil {
			return SessionForkOperation{}, false, fmt.Errorf(
				"release failed session fork target reservation: %w",
				err,
			)
		}
		if _, err := tx.ExecContext(ctx, `
DELETE FROM workspace_agent_session_fork_boundary_barriers
WHERE workspace_id = ? AND operation_id = ?
`, workspaceID, operationID); err != nil {
			return SessionForkOperation{}, false, fmt.Errorf(
				"release failed session fork boundary barrier: %w",
				err,
			)
		}
	}
	mutations := []TransactionMutation(nil)
	if changed {
		mutations = append(mutations, transactionMutation(workspaceID, op.SourceAgentSessionID, MutationEntitySessionForkOperation, operationID, toStatus, now))
	}
	delta, err := s.commitTransaction(ctx, tx, workspaceID, mutations)
	if err != nil {
		return SessionForkOperation{}, false, err
	}
	op.CommitTransactionID, op.CommitDelta = delta.TransactionID, delta
	return op, changed, nil
}

func loadSessionForkSnapshotTx(
	ctx context.Context,
	tx *sql.Tx,
	session Session,
	throughSequence int64,
	frozenBoundaryMessageID int64,
) (sessionForkSnapshot, error) {
	snapshot := sessionForkSnapshot{Version: 1, Session: session}
	rows, err := tx.QueryContext(ctx, `
SELECT turn_id, turn_sequence, provenance
FROM workspace_agent_turn_sequences
WHERE workspace_id = ? AND agent_session_id = ? AND turn_sequence <= ?
ORDER BY turn_sequence
`, session.WorkspaceID, session.ID, throughSequence)
	if err != nil {
		return snapshot, fmt.Errorf("read session fork turns: %w", err)
	}
	type turnBoundary struct {
		turnID     string
		sequence   int64
		provenance string
	}
	var boundaries []turnBoundary
	for rows.Next() {
		var boundary turnBoundary
		if err := rows.Scan(&boundary.turnID, &boundary.sequence, &boundary.provenance); err != nil {
			rows.Close()
			return snapshot, err
		}
		boundaries = append(boundaries, boundary)
	}
	if err := rows.Close(); err != nil {
		return snapshot, err
	}
	for _, boundary := range boundaries {
		turn, found, err := getAgentTurnTx(ctx, tx, session.WorkspaceID, session.ID, boundary.turnID)
		if err != nil {
			return snapshot, err
		}
		if !found || !isVerifiedSessionForkSequence(boundary.provenance) ||
			turn.Phase != TurnPhaseSettled || strings.TrimSpace(turn.RootProviderTurnID) == "" {
			return snapshot, ErrSessionForkTurnState
		}
		snapshot.Turns = append(snapshot.Turns, sessionForkTurnSnapshot{Sequence: boundary.sequence, Turn: turn})
	}
	boundaryMessageID := frozenBoundaryMessageID
	if boundaryMessageID <= 0 {
		if err := tx.QueryRowContext(ctx, `
SELECT COALESCE(MAX(message.id), 0)
FROM workspace_agent_messages message
JOIN workspace_agent_turn_sequences sequence
  ON sequence.workspace_id = message.workspace_id
 AND sequence.agent_session_id = message.agent_session_id
 AND sequence.turn_id = message.turn_id
WHERE message.workspace_id = ?
  AND message.agent_session_id = ?
  AND message.deleted_at_unix_ms = 0
  AND sequence.turn_sequence <= ?
`, session.WorkspaceID, session.ID, throughSequence).Scan(&boundaryMessageID); err != nil {
			return snapshot, fmt.Errorf("read session fork message boundary: %w", err)
		}
	}
	if boundaryMessageID <= 0 {
		return snapshot, ErrSessionForkTurnState
	}
	snapshot.BoundaryMessageID = boundaryMessageID
	var unsupportedTurnless int
	if err := tx.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1
  FROM workspace_agent_messages
  WHERE workspace_id = ?
    AND agent_session_id = ?
    AND deleted_at_unix_ms = 0
    AND id <= ?
    AND turn_id IS NULL
    AND kind <> 'session_audit'
)
`, session.WorkspaceID, session.ID, boundaryMessageID).Scan(&unsupportedTurnless); err != nil {
		return snapshot, fmt.Errorf("read unsupported turnless session fork messages: %w", err)
	}
	if unsupportedTurnless != 0 {
		return snapshot, ErrSessionForkTurnState
	}
	messageRows, err := tx.QueryContext(ctx, `
SELECT message.id, message.agent_session_id, message.message_id, message.version,
       message.turn_id, message.role, message.kind, message.status,
       message.semantics_json, message.payload_json, message.occurred_at_unix_ms,
       message.started_at_unix_ms, message.completed_at_unix_ms,
       message.created_at_unix_ms, message.updated_at_unix_ms
FROM workspace_agent_messages message
WHERE message.workspace_id = ? AND message.agent_session_id = ?
  AND message.deleted_at_unix_ms = 0
  AND (
    (
      message.turn_id IS NULL
      AND message.kind = 'session_audit'
      AND message.id <= ?
    )
    OR EXISTS (
      SELECT 1
      FROM workspace_agent_turn_sequences sequence
      WHERE sequence.workspace_id = message.workspace_id
        AND sequence.agent_session_id = message.agent_session_id
        AND sequence.turn_id = message.turn_id
        AND sequence.turn_sequence <= ?
    )
  )
ORDER BY message.id
`, session.WorkspaceID, session.ID, boundaryMessageID, throughSequence)
	if err != nil {
		return snapshot, fmt.Errorf("read session fork messages: %w", err)
	}
	for messageRows.Next() {
		message, err := scanAgentMessage(messageRows)
		if err != nil {
			messageRows.Close()
			return snapshot, err
		}
		snapshot.Messages = append(snapshot.Messages, message)
	}
	if err := messageRows.Close(); err != nil {
		return snapshot, err
	}
	interactionRows, err := tx.QueryContext(ctx, `
SELECT interaction.workspace_id, interaction.agent_session_id,
       interaction.request_id, interaction.turn_id, interaction.kind,
       interaction.status, interaction.tool_name, interaction.input_json,
       interaction.output_json, interaction.metadata_json,
       interaction.created_at_unix_ms, interaction.updated_at_unix_ms
FROM workspace_agent_interactions interaction
JOIN workspace_agent_turn_sequences sequence
  ON sequence.workspace_id = interaction.workspace_id
 AND sequence.agent_session_id = interaction.agent_session_id
 AND sequence.turn_id = interaction.turn_id
WHERE interaction.workspace_id = ?
  AND interaction.agent_session_id = ?
  AND sequence.turn_sequence <= ?
  AND interaction.status IN (?, ?)
ORDER BY sequence.turn_sequence, interaction.created_at_unix_ms,
         interaction.request_id
`, session.WorkspaceID, session.ID, throughSequence,
		InteractionStatusAnswered, InteractionStatusSuperseded)
	if err != nil {
		return snapshot, fmt.Errorf("read session fork interactions: %w", err)
	}
	for interactionRows.Next() {
		interaction, err := scanAgentInteraction(interactionRows)
		if err != nil {
			interactionRows.Close()
			return snapshot, err
		}
		snapshot.Interactions = append(snapshot.Interactions, interaction)
	}
	if err := interactionRows.Close(); err != nil {
		return snapshot, err
	}
	return snapshot, nil
}

func getSessionForkSourceTx(ctx context.Context, tx *sql.Tx, workspaceID, sessionID string) (Session, bool, error) {
	row := tx.QueryRowContext(ctx, `
SELECT workspace_id, agent_session_id, session_kind, root_agent_session_id, root_turn_id,
       parent_agent_session_id, parent_turn_id, parent_tool_call_id,
       origin, agent_target_id, provider, provider_session_id, model,
       user_id, settings_json, session_metadata_json, internal_runtime_context_json, cwd,
       rail_section_key, title, message_version, last_event_at_unix_ms,
       started_at_unix_ms, ended_at_unix_ms, pinned_at_unix_ms,
       created_at_unix_ms, updated_at_unix_ms, active_turn_id
FROM workspace_agent_sessions
WHERE workspace_id = ? AND agent_session_id = ? AND deleted_at_unix_ms = 0
`, workspaceID, sessionID)
	session, err := scanAgentSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, false, nil
	}
	if err != nil {
		return Session{}, false, err
	}
	var railKind, railPath string
	if err := tx.QueryRowContext(ctx, `
SELECT rail_section_kind, rail_project_path
FROM workspace_agent_sessions
WHERE workspace_id = ? AND agent_session_id = ?
`, workspaceID, sessionID).Scan(&railKind, &railPath); err != nil {
		return Session{}, false, err
	}
	session.RailSectionKind, session.RailProjectPath = railKind, railPath
	return session, true, nil
}

func insertForkedSessionTx(ctx context.Context, tx *sql.Tx, op SessionForkOperation, snapshot sessionForkSnapshot, now int64) error {
	target := sessionForkResultSession(op, snapshot, now)
	metadataJSON, err := marshalSessionMetadata(target.Metadata)
	if err != nil {
		return err
	}
	settingsJSON, err := marshalJSONMap(target.Settings)
	if err != nil {
		return err
	}
	runtimeContextJSON, err := marshalJSONMap(target.InternalRuntimeContext)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO workspace_agent_sessions (
  workspace_id, agent_session_id, session_kind,
  root_agent_session_id, root_turn_id, parent_agent_session_id, parent_turn_id, parent_tool_call_id,
  origin, user_id, agent_target_id, provider, provider_session_id, model,
  settings_json, session_metadata_json, internal_runtime_context_json,
  cwd, rail_section_kind, rail_project_path, rail_section_key,
  title, message_version, last_event_at_unix_ms, started_at_unix_ms,
  ended_at_unix_ms, pinned_at_unix_ms, deleted_at_unix_ms,
  created_at_unix_ms, updated_at_unix_ms, active_turn_id
) VALUES (?, ?, ?, NULL, NULL, NULL, NULL, NULL, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, 0, 0, 0, ?, ?, NULL)
`, target.WorkspaceID, target.ID, target.Kind,
		target.Origin, target.UserID, nullString(target.AgentTargetID),
		target.Provider, target.ProviderSessionID, target.Model, settingsJSON, metadataJSON,
		runtimeContextJSON, target.Cwd, target.RailSectionKind, target.RailProjectPath,
		target.RailSectionKey, target.Title, target.LastEventUnixMS, target.StartedAtUnixMS,
		target.CreatedAtUnixMS, target.UpdatedAtUnixMS)
	if err != nil {
		return fmt.Errorf("insert forked workspace agent session: %w", err)
	}
	return nil
}

func materializeSessionForkSnapshot(snapshot sessionForkSnapshot) sessionForkSnapshot {
	if snapshot.Version != 1 {
		return snapshot
	}
	if strings.TrimSpace(snapshot.TargetCwd) == "" {
		snapshot.TargetCwd = strings.TrimSpace(snapshot.Session.Cwd)
	}
	if snapshot.TargetRuntimeContext == nil {
		snapshot.TargetRuntimeContext = cloneJSONMap(
			snapshot.Session.InternalRuntimeContext,
		)
	}
	if snapshot.TargetSettings == nil {
		snapshot.TargetSettings = cloneJSONMap(snapshot.Session.Settings)
	}
	return snapshot
}

func sessionForkResultSession(
	op SessionForkOperation,
	snapshot sessionForkSnapshot,
	committedAtUnixMS int64,
) Session {
	source := snapshot.Session
	targetTitle := strings.TrimSpace(snapshot.TargetTitle)
	if targetTitle == "" {
		// Compatibility for snapshots prepared before Fork titles were
		// materialized independently from the source title.
		targetTitle = strings.TrimSpace(source.Title)
	}
	metadata := source.Metadata
	metadata.Imported, metadata.Usage, metadata.Goal = false, nil, nil
	targetSettings := snapshot.TargetSettings
	if targetSettings == nil {
		targetSettings = source.Settings
	}
	return Session{
		ID:                     op.TargetAgentSessionID,
		WorkspaceID:            op.WorkspaceID,
		Kind:                   SessionKindRoot,
		Origin:                 "WORKSPACE_AGENT_SESSION_ORIGIN_RUNTIME",
		UserID:                 source.UserID,
		AgentTargetID:          source.AgentTargetID,
		Provider:               source.Provider,
		ProviderSessionID:      op.TargetProviderSessionID,
		Model:                  sessionForkTargetModel(source.Model, targetSettings),
		Settings:               cloneJSONMap(targetSettings),
		Metadata:               metadata,
		InternalRuntimeContext: cloneJSONMap(snapshot.TargetRuntimeContext),
		Cwd:                    strings.TrimSpace(snapshot.TargetCwd),
		RailSectionKind:        source.RailSectionKind,
		RailProjectPath:        source.RailProjectPath,
		RailSectionKey:         source.RailSectionKey,
		Title:                  targetTitle,
		MessageVersion:         uint64(len(snapshot.Messages)),
		LastEventUnixMS:        committedAtUnixMS,
		StartedAtUnixMS:        committedAtUnixMS,
		CreatedAtUnixMS:        committedAtUnixMS,
		UpdatedAtUnixMS:        committedAtUnixMS,
	}
}

func nextSessionForkTargetTitleTx(
	ctx context.Context,
	tx *sql.Tx,
	workspaceID string,
	sourceAgentSessionID string,
	sourceTitle string,
) (string, error) {
	sourceTitle = strings.TrimSpace(sourceTitle)
	if sourceTitle == "" {
		return "", nil
	}
	var activeForkCount int64
	if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM workspace_agent_session_fork_operations
WHERE workspace_id = ?
  AND source_agent_session_id = ?
  AND status <> ?
`, workspaceID, sourceAgentSessionID, SessionForkStatusFailed).Scan(&activeForkCount); err != nil {
		return "", fmt.Errorf("count source session forks for title: %w", err)
	}
	return fmt.Sprintf("%s (%d)", sourceTitle, activeForkCount+2), nil
}

func sessionForkResultLineage(
	op SessionForkOperation,
	committedAtUnixMS int64,
) SessionForkLineage {
	return SessionForkLineage{
		WorkspaceID:          op.WorkspaceID,
		TargetAgentSessionID: op.TargetAgentSessionID,
		SourceAgentSessionID: op.SourceAgentSessionID,
		SourceTurnID:         op.SourceTurnID,
		TargetTurnID:         op.TargetTurnID,
		OperationID:          op.OperationID,
		ForkedAtUnixMS:       committedAtUnixMS,
	}
}

func sessionForkTargetModel(sourceModel string, targetSettings map[string]any) string {
	if model, ok := targetSettings["model"].(string); ok {
		return strings.TrimSpace(model)
	}
	return sourceModel
}

func insertForkedTurnTx(ctx context.Context, tx *sql.Tx, workspaceID, sessionID string, turn Turn) error {
	capabilityRefsJSON, err := json.Marshal(turn.CapabilityRefs)
	if err != nil {
		return err
	}
	fileChangesJSON, err := marshalNullableJSONMap(turn.FileChanges)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO workspace_agent_turns (
  workspace_id, agent_session_id, turn_id, capability_refs_json, phase, outcome,
  error_json, file_changes_json, completed_command_json, backfilled,
  started_at_unix_ms, settled_at_unix_ms, created_at_unix_ms, updated_at_unix_ms,
  turn_origin, source_goal_operation_id, source_goal_revision, source_goal_repair_epoch,
  root_provider_turn_id, root_provider_turn_phase, root_provider_turn_outcome,
  root_provider_turn_error_json, root_provider_turn_completed_command_json,
  root_provider_turn_updated_at_unix_ms
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, NULL, ?, ?, ?, ?, ?, ?)
`, workspaceID, sessionID, turn.TurnID, string(capabilityRefsJSON), turn.Phase,
		nullString(turn.Outcome), encodeTurnErrorJSON(turn.ErrorMessage, turn.ErrorCode),
		fileChangesJSON,
		encodeCompletedCommandJSON(turn.CompletedCommandKind, turn.CompletedCommandStatus, finalAssistantWatermark{
			MessageID: turn.FinalAssistantMessageID, Resolved: turn.FinalAssistantMessageResolved,
		}),
		turn.Backfilled, turn.StartedAtUnixMS, nullInt64(turn.SettledAtUnixMS),
		turn.CreatedAtUnixMS, turn.UpdatedAtUnixMS, turn.Origin,
		nullString(turn.RootProviderTurnID), nullString(turn.RootProviderTurnPhase),
		nullString(turn.RootProviderTurnOutcome),
		encodeTurnErrorJSON(turn.RootProviderTurnErrorMessage, turn.RootProviderTurnErrorCode),
		encodeCompletedCommandJSON(turn.RootProviderTurnCompletedCommandKind, turn.RootProviderTurnCompletedCommandStatus, finalAssistantWatermark{}),
		turn.RootProviderTurnUpdatedAtUnixMS)
	if err != nil {
		return fmt.Errorf("insert forked workspace agent turn: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE workspace_agent_turn_sequences
SET provenance = 'fork_clone_verified'
WHERE workspace_id = ? AND agent_session_id = ? AND turn_id = ?
`, workspaceID, sessionID, turn.TurnID); err != nil {
		return fmt.Errorf("verify forked workspace agent turn sequence: %w", err)
	}
	return nil
}

func insertForkedMessageTx(ctx context.Context, tx *sql.Tx, workspaceID, sessionID string, message Message, version uint64) error {
	payloadJSON, err := marshalJSONMap(message.Payload)
	if err != nil {
		return err
	}
	semanticsJSON, err := json.Marshal(message.Semantics)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO workspace_agent_messages (
  workspace_id, agent_session_id, message_id, version, turn_id, role, kind, status,
  semantics_json, payload_json, occurred_at_unix_ms, started_at_unix_ms,
  completed_at_unix_ms, deleted_at_unix_ms, created_at_unix_ms, updated_at_unix_ms
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?)
`, workspaceID, sessionID, message.MessageID, version, nullString(message.TurnID),
		message.Role, message.Kind, message.Status, string(semanticsJSON), payloadJSON,
		message.OccurredAtUnixMS, message.StartedAtUnixMS, message.CompletedAtUnixMS,
		message.CreatedAtUnixMS, message.UpdatedAtUnixMS)
	if err != nil {
		return fmt.Errorf("insert forked workspace agent message: %w", err)
	}
	return nil
}

func insertForkedInteractionTx(ctx context.Context, tx *sql.Tx, workspaceID, sessionID string, interaction Interaction) error {
	inputJSON, err := marshalJSONMap(interaction.Input)
	if err != nil {
		return err
	}
	outputJSON, err := marshalJSONMap(interaction.Output)
	if err != nil {
		return err
	}
	metadataJSON, err := marshalJSONMap(interaction.Metadata)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO workspace_agent_interactions (
  workspace_id, agent_session_id, request_id, turn_id, kind, status,
  tool_name, input_json, output_json, metadata_json,
  created_at_unix_ms, updated_at_unix_ms
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, workspaceID, sessionID, interaction.RequestID, interaction.TurnID,
		interaction.Kind, interaction.Status, interaction.ToolName, inputJSON,
		outputJSON, metadataJSON, interaction.CreatedAtUnixMS, interaction.UpdatedAtUnixMS)
	if err != nil {
		return fmt.Errorf("insert forked workspace agent interaction: %w", err)
	}
	return nil
}

func getSessionForkOperation(ctx context.Context, db *sql.DB, workspaceID, operationID string) (SessionForkOperation, bool, error) {
	op, err := scanSessionForkOperation(db.QueryRowContext(ctx, sessionForkOperationSelectSQL+`
WHERE workspace_id = ? AND operation_id = ?`, workspaceID, operationID))
	if errors.Is(err, sql.ErrNoRows) {
		return SessionForkOperation{}, false, nil
	}
	return op, err == nil, err
}

func getSessionForkOperationTx(ctx context.Context, tx *sql.Tx, workspaceID, operationID string) (SessionForkOperation, bool, error) {
	op, err := scanSessionForkOperation(tx.QueryRowContext(ctx, sessionForkOperationSelectSQL+`
WHERE workspace_id = ? AND operation_id = ?`, workspaceID, operationID))
	if errors.Is(err, sql.ErrNoRows) {
		return SessionForkOperation{}, false, nil
	}
	return op, err == nil, err
}

func getSessionForkOperationByRequestTx(ctx context.Context, tx *sql.Tx, workspaceID, requestID string) (SessionForkOperation, bool, error) {
	op, err := scanSessionForkOperation(tx.QueryRowContext(ctx, sessionForkOperationSelectSQL+`
WHERE workspace_id = ? AND request_id = ?`, workspaceID, requestID))
	if errors.Is(err, sql.ErrNoRows) {
		return SessionForkOperation{}, false, nil
	}
	return op, err == nil, err
}

func getSessionForkOperationWithSnapshotTx(ctx context.Context, tx *sql.Tx, workspaceID, operationID string) (SessionForkOperation, bool, string, error) {
	row := tx.QueryRowContext(ctx, `
SELECT operation_id, workspace_id, request_id, request_hash,
       source_agent_session_id, target_agent_session_id,
       source_provider_session_id, source_turn_id, source_provider_turn_id,
       COALESCE(target_turn_id, ''),
       point_kind, driver_kind, driver_version, status,
       COALESCE(target_provider_session_id, ''), snapshot_hash, last_error,
       created_at_unix_ms, updated_at_unix_ms,
       COALESCE(dispatched_at_unix_ms, 0), COALESCE(accepted_at_unix_ms, 0),
       COALESCE(completed_at_unix_ms, 0),
       COALESCE(client_observed_at_unix_ms, 0), snapshot_json
FROM workspace_agent_session_fork_operations
WHERE workspace_id = ? AND operation_id = ?`, workspaceID, operationID)
	var snapshotJSON string
	op, err := scanSessionForkOperationWithExtra(row, &snapshotJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return SessionForkOperation{}, false, "", nil
	}
	return op, err == nil, snapshotJSON, err
}

func getSessionForkBoundaryBarrierTx(
	ctx context.Context,
	tx *sql.Tx,
	workspaceID, sourceSessionID, pointKind, sourceTurnID string,
) (SessionForkOperation, bool, error) {
	var operationID string
	err := tx.QueryRowContext(ctx, `
SELECT operation_id
FROM workspace_agent_session_fork_boundary_barriers
WHERE workspace_id = ?
  AND source_agent_session_id = ?
  AND point_kind = ?
  AND source_turn_id = ?
`, workspaceID, sourceSessionID, pointKind, sourceTurnID).Scan(&operationID)
	if errors.Is(err, sql.ErrNoRows) {
		return SessionForkOperation{}, false, nil
	}
	if err != nil {
		return SessionForkOperation{}, false, fmt.Errorf(
			"read session fork boundary barrier: %w",
			err,
		)
	}
	return getSessionForkOperationTx(ctx, tx, workspaceID, operationID)
}

func scanSessionForkOperation(scanner rowScanner) (SessionForkOperation, error) {
	return scanSessionForkOperationWithExtra(scanner)
}

func scanSessionForkOperationWithExtra(scanner rowScanner, extra ...any) (SessionForkOperation, error) {
	var op SessionForkOperation
	destinations := []any{
		&op.OperationID, &op.WorkspaceID, &op.RequestID, &op.RequestHash,
		&op.SourceAgentSessionID, &op.TargetAgentSessionID,
		&op.SourceProviderSessionID, &op.SourceTurnID, &op.SourceProviderTurnID,
		&op.TargetTurnID,
		&op.PointKind, &op.DriverKind, &op.DriverVersion, &op.Status, &op.TargetProviderSessionID,
		&op.SnapshotHash, &op.LastError, &op.CreatedAtUnixMS, &op.UpdatedAtUnixMS,
		&op.DispatchedAtUnixMS, &op.AcceptedAtUnixMS, &op.CompletedAtUnixMS,
		&op.ClientObservedAtUnixMS,
	}
	destinations = append(destinations, extra...)
	if err := scanner.Scan(destinations...); err != nil {
		return SessionForkOperation{}, err
	}
	if op.Status == SessionForkStatusCommitted &&
		strings.TrimSpace(op.TargetTurnID) == "" {
		return SessionForkOperation{}, errors.New(
			"committed session fork operation omitted target turn identity",
		)
	}
	return op, nil
}

func scanSessionForkLineage(scanner rowScanner) (SessionForkLineage, bool, error) {
	var lineage SessionForkLineage
	err := scanner.Scan(&lineage.WorkspaceID, &lineage.TargetAgentSessionID,
		&lineage.SourceAgentSessionID, &lineage.SourceTurnID,
		&lineage.TargetTurnID,
		&lineage.OperationID, &lineage.ForkedAtUnixMS)
	if errors.Is(err, sql.ErrNoRows) {
		return SessionForkLineage{}, false, nil
	}
	if err == nil && strings.TrimSpace(lineage.TargetTurnID) == "" {
		return SessionForkLineage{}, false, errors.New(
			"session fork lineage omitted target turn identity",
		)
	}
	return lineage, err == nil, err
}

func normalizeSessionForkPrepare(input *SessionForkPrepare) {
	input.OperationID = strings.TrimSpace(input.OperationID)
	input.WorkspaceID = strings.TrimSpace(input.WorkspaceID)
	input.RequestID = strings.TrimSpace(input.RequestID)
	input.RequestHash = strings.TrimSpace(input.RequestHash)
	input.SourceAgentSessionID = strings.TrimSpace(input.SourceAgentSessionID)
	input.TargetAgentSessionID = strings.TrimSpace(input.TargetAgentSessionID)
	input.SourceTurnID = strings.TrimSpace(input.SourceTurnID)
	input.PointKind = strings.TrimSpace(input.PointKind)
	if input.PointKind == "" {
		// Session fork v1 only supported inclusive through-Turn forks. Preserve
		// that exact meaning for in-process callers compiled before Point was
		// promoted into the durable operation contract.
		input.PointKind = SessionForkPointThroughTurn
	}
	input.DriverKind = strings.TrimSpace(input.DriverKind)
	input.DriverVersion = strings.TrimSpace(input.DriverVersion)
}

func hashSessionForkBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func isVerifiedSessionForkSequence(provenance string) bool {
	return provenance == "verified" || provenance == "fork_clone_verified"
}
