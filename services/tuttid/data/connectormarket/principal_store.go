package connectormarket

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	market "github.com/tutti-os/tutti/packages/connector/market/daemon"
	agentprincipalbiz "github.com/tutti-os/tutti/services/tuttid/biz/agentprincipal"
)

var (
	ErrAgentPrincipalNotFound = errors.New("agent principal not found")
)

func (store *Store) migrateAgentPrincipals(ctx context.Context) error {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS agent_principals (
  principal_id TEXT PRIMARY KEY,
  kind TEXT NOT NULL CHECK (kind IN ('workspace_agent', 'system_target')),
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  harness_agent_target_id TEXT NOT NULL,
  source_workspace_id TEXT NOT NULL DEFAULT '',
  source_agent_id TEXT NOT NULL DEFAULT '',
  source_revision INTEGER NOT NULL DEFAULT 0,
  created_at_unix_ms INTEGER NOT NULL,
  updated_at_unix_ms INTEGER NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS agent_principals_workspace_agent_source
  ON agent_principals(source_workspace_id, source_agent_id)
  WHERE kind = 'workspace_agent';
CREATE UNIQUE INDEX IF NOT EXISTS agent_principals_system_target_source
  ON agent_principals(harness_agent_target_id)
  WHERE kind = 'system_target';
CREATE TABLE IF NOT EXISTS connector_agent_grants (
  connector_key TEXT NOT NULL,
  principal_id TEXT NOT NULL,
  created_at_unix_ms INTEGER NOT NULL,
  PRIMARY KEY (connector_key, principal_id),
  FOREIGN KEY (connector_key) REFERENCES connector_market_connectors(connector_key) ON DELETE CASCADE,
  FOREIGN KEY (principal_id) REFERENCES agent_principals(principal_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS connector_agent_grants_principal
  ON connector_agent_grants(principal_id, connector_key);
CREATE TABLE IF NOT EXISTS connector_agent_grant_metadata (
  connector_key TEXT PRIMARY KEY,
  revision INTEGER NOT NULL,
  FOREIGN KEY (connector_key) REFERENCES connector_market_connectors(connector_key) ON DELETE CASCADE
);
DROP TABLE IF EXISTS connector_session_execution_tokens;
`); err != nil {
		return fmt.Errorf("create agent principal schema: %w", err)
	}
	if err := ensurePerConnectorGrantMetadata(ctx, tx); err != nil {
		return err
	}

	now := time.Now().UTC()
	if err := backfillWorkspaceAgentPrincipals(ctx, tx, now); err != nil {
		return err
	}
	if err := backfillSystemTargetPrincipals(ctx, tx, now); err != nil {
		return err
	}
	if err := migrateLegacyWorkspaceBindingsToAgentGrants(ctx, tx, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DROP TABLE IF EXISTS connector_market_workspace_bindings`); err != nil {
		return fmt.Errorf("remove legacy connector Workspace bindings: %w", err)
	}
	return tx.Commit()
}

func ensurePerConnectorGrantMetadata(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `PRAGMA table_info(connector_agent_grant_metadata)`)
	if err != nil {
		return err
	}
	legacy := false
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, dataType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			_ = rows.Close()
			return err
		}
		legacy = legacy || name == "id"
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if legacy {
		if _, err := tx.ExecContext(ctx, `DROP TABLE connector_agent_grant_metadata`); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS connector_agent_grant_metadata (
  connector_key TEXT PRIMARY KEY,
  revision INTEGER NOT NULL,
  FOREIGN KEY (connector_key) REFERENCES connector_market_connectors(connector_key) ON DELETE CASCADE
)`)
	return err
}

func backfillWorkspaceAgentPrincipals(ctx context.Context, tx *sql.Tx, now time.Time) error {
	rows, err := tx.QueryContext(ctx, `
SELECT workspace_id, agent_id, name, description, harness_agent_target_id, revision,
       created_at_unix_ms, updated_at_unix_ms
FROM workspace_agents`)
	if err != nil {
		if isMissingTableError(err) {
			return nil
		}
		return fmt.Errorf("list Workspace Agents for principal migration: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var workspaceID, agentID, name, description, targetID string
		var revision, createdAt, updatedAt int64
		if err := rows.Scan(&workspaceID, &agentID, &name, &description, &targetID, &revision, &createdAt, &updatedAt); err != nil {
			return err
		}
		principal := agentprincipalbiz.Principal{ID: agentprincipalbiz.WorkspaceAgentID(workspaceID, agentID), Kind: agentprincipalbiz.KindWorkspaceAgent,
			Name: name, Description: description, HarnessAgentTargetID: targetID, SourceWorkspaceID: workspaceID,
			SourceAgentID: agentID, SourceRevision: revision, CreatedAt: time.UnixMilli(createdAt).UTC(), UpdatedAt: time.UnixMilli(updatedAt).UTC()}
		if principal.CreatedAt.IsZero() {
			principal.CreatedAt = now
		}
		if principal.UpdatedAt.IsZero() {
			principal.UpdatedAt = now
		}
		if err := upsertPrincipal(ctx, tx, principal); err != nil {
			return err
		}
	}
	return rows.Err()
}

func backfillSystemTargetPrincipals(ctx context.Context, tx *sql.Tx, now time.Time) error {
	rows, err := tx.QueryContext(ctx, `SELECT id, name, created_at_ms, updated_at_ms FROM agent_targets`)
	if err != nil {
		if isMissingTableError(err) {
			return nil
		}
		return fmt.Errorf("list Agent Targets for principal migration: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var targetID, name string
		var createdAt, updatedAt int64
		if err := rows.Scan(&targetID, &name, &createdAt, &updatedAt); err != nil {
			return err
		}
		principal := agentprincipalbiz.Principal{ID: agentprincipalbiz.SystemTargetID(targetID), Kind: agentprincipalbiz.KindSystemTarget,
			Name: name, HarnessAgentTargetID: targetID, CreatedAt: time.UnixMilli(createdAt).UTC(), UpdatedAt: time.UnixMilli(updatedAt).UTC()}
		if principal.CreatedAt.IsZero() {
			principal.CreatedAt = now
		}
		if principal.UpdatedAt.IsZero() {
			principal.UpdatedAt = now
		}
		if err := upsertPrincipal(ctx, tx, principal); err != nil {
			return err
		}
	}
	return rows.Err()
}

func migrateLegacyWorkspaceBindingsToAgentGrants(ctx context.Context, tx *sql.Tx, now time.Time) error {
	result, err := tx.ExecContext(ctx, `
INSERT OR IGNORE INTO connector_agent_grants (connector_key, principal_id, created_at_unix_ms)
SELECT binding.connector_key, principal.principal_id, ?
FROM connector_market_workspace_bindings AS binding
JOIN agent_principals AS principal
  ON principal.kind = 'workspace_agent'
 AND principal.source_workspace_id = binding.workspace_id
WHERE binding.enabled = 1`, now.UnixMilli())
	if err != nil {
		if isMissingTableError(err) {
			return nil
		}
		return fmt.Errorf("migrate connector Workspace bindings to Agent grants: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed > 0 {
		_, err = tx.ExecContext(ctx, `
INSERT INTO connector_agent_grant_metadata (connector_key, revision)
SELECT DISTINCT connector_key, 1 FROM connector_agent_grants
ON CONFLICT(connector_key) DO UPDATE SET revision = revision + 1`)
	}
	return err
}

func (store *Store) ListAgentPrincipals(ctx context.Context) ([]agentprincipalbiz.Principal, error) {
	if err := store.refreshAgentPrincipals(ctx); err != nil {
		return nil, err
	}
	rows, err := store.db.QueryContext(ctx, principalSelect+` ORDER BY name, principal_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	principals := make([]agentprincipalbiz.Principal, 0)
	for rows.Next() {
		principal, err := scanPrincipal(rows)
		if err != nil {
			return nil, err
		}
		principals = append(principals, principal)
	}
	return principals, rows.Err()
}

func (store *Store) refreshAgentPrincipals(ctx context.Context) error {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC()
	if err := backfillWorkspaceAgentPrincipals(ctx, tx, now); err != nil {
		return err
	}
	if err := backfillSystemTargetPrincipals(ctx, tx, now); err != nil {
		return err
	}
	if exists, err := tableExists(ctx, tx, "workspace_agents"); err != nil {
		return err
	} else if exists {
		if _, err := tx.ExecContext(ctx, `DELETE FROM agent_principals
WHERE kind = 'workspace_agent' AND NOT EXISTS (
  SELECT 1 FROM workspace_agents AS agent
  WHERE agent.workspace_id = agent_principals.source_workspace_id
    AND agent.agent_id = agent_principals.source_agent_id
)`); err != nil {
			return err
		}
	}
	if exists, err := tableExists(ctx, tx, "agent_targets"); err != nil {
		return err
	} else if exists {
		if _, err := tx.ExecContext(ctx, `DELETE FROM agent_principals
WHERE kind = 'system_target' AND NOT EXISTS (
  SELECT 1 FROM agent_targets AS target
  WHERE target.id = agent_principals.harness_agent_target_id
)`); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func tableExists(ctx context.Context, tx *sql.Tx, name string) (bool, error) {
	var exists bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(
SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?)`, name).Scan(&exists)
	return exists, err
}

func (store *Store) ListConnectorAgentPrincipals(ctx context.Context) ([]market.AgentPrincipal, error) {
	principals, err := store.ListAgentPrincipals(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]market.AgentPrincipal, 0, len(principals))
	for _, principal := range principals {
		result = append(result, market.AgentPrincipal{
			PrincipalID:          principal.ID,
			Kind:                 string(principal.Kind),
			Name:                 principal.Name,
			Description:          principal.Description,
			HarnessAgentTargetID: principal.HarnessAgentTargetID,
		})
	}
	return result, nil
}

func (store *Store) AgentPrincipal(ctx context.Context, principalID string) (agentprincipalbiz.Principal, error) {
	principal, err := scanPrincipal(store.db.QueryRowContext(ctx, principalSelect+` WHERE principal_id = ?`, strings.TrimSpace(principalID)))
	if errors.Is(err, sql.ErrNoRows) {
		return agentprincipalbiz.Principal{}, ErrAgentPrincipalNotFound
	}
	return principal, err
}

func (store *Store) ReplaceAgentGrants(ctx context.Context, connectorKey string, principalIDs []string, expectedRevision uint64) (uint64, error) {
	connectorKey = strings.TrimSpace(connectorKey)
	principalIDs = normalizeIDs(principalIDs)
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	var currentRevision uint64
	if err := tx.QueryRowContext(ctx, `SELECT revision FROM connector_agent_grant_metadata WHERE connector_key = ?`, connectorKey).Scan(&currentRevision); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if currentRevision != expectedRevision {
		rows, err := tx.QueryContext(ctx, `SELECT principal_id FROM connector_agent_grants WHERE connector_key = ? ORDER BY principal_id`, connectorKey)
		if err != nil {
			return 0, err
		}
		currentIDs := make([]string, 0)
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				_ = rows.Close()
				return 0, err
			}
			currentIDs = append(currentIDs, id)
		}
		if err := rows.Close(); err != nil {
			return 0, err
		}
		if slices.Equal(currentIDs, principalIDs) {
			return currentRevision, tx.Commit()
		}
		return 0, market.NewDomainError(market.ErrorCodeRevisionConflict, "connector Agent grants changed", true, nil)
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM connector_market_connectors WHERE connector_key = ?)`, connectorKey).Scan(&exists); err != nil {
		return 0, err
	}
	if !exists {
		return 0, market.ErrNotFound
	}
	for _, principalID := range principalIDs {
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM agent_principals WHERE principal_id = ?)`, principalID).Scan(&exists); err != nil {
			return 0, err
		}
		if !exists {
			return 0, market.NewDomainError(market.ErrorCodeInvalidRequest, "unknown Agent principal: "+principalID, false, ErrAgentPrincipalNotFound)
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM connector_agent_grants WHERE connector_key = ?`, connectorKey); err != nil {
		return 0, err
	}
	now := time.Now().UTC().UnixMilli()
	for _, principalID := range principalIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO connector_agent_grants (connector_key, principal_id, created_at_unix_ms) VALUES (?, ?, ?)`, connectorKey, principalID, now); err != nil {
			return 0, err
		}
	}
	var revision uint64
	if err := tx.QueryRowContext(ctx, `
INSERT INTO connector_agent_grant_metadata (connector_key, revision) VALUES (?, 1)
ON CONFLICT(connector_key) DO UPDATE SET revision = revision + 1
RETURNING revision`, connectorKey).Scan(&revision); err != nil {
		return 0, err
	}
	return revision, tx.Commit()
}

func (store *Store) AgentGrants(ctx context.Context, connectorKey string) ([]string, uint64, error) {
	rows, err := store.db.QueryContext(ctx, `SELECT principal_id FROM connector_agent_grants WHERE connector_key = ? ORDER BY principal_id`, strings.TrimSpace(connectorKey))
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, 0, err
		}
		ids = append(ids, id)
	}
	var revision uint64
	if err := store.db.QueryRowContext(ctx, `SELECT revision FROM connector_agent_grant_metadata WHERE connector_key = ?`, strings.TrimSpace(connectorKey)).Scan(&revision); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, 0, err
	}
	return ids, revision, rows.Err()
}

func (store *Store) PrincipalHasConnectorGrant(ctx context.Context, principalID, connectorKey string) (bool, error) {
	var granted bool
	err := store.db.QueryRowContext(ctx, `SELECT EXISTS(
SELECT 1 FROM connector_agent_grants WHERE principal_id = ? AND connector_key = ?)`,
		strings.TrimSpace(principalID), strings.TrimSpace(connectorKey)).Scan(&granted)
	return granted, err
}

func (store *Store) GrantedConnectorKeys(ctx context.Context, principalID string) ([]string, uint64, error) {
	rows, err := store.db.QueryContext(ctx, `
SELECT connector_key FROM connector_agent_grants
WHERE principal_id = ? ORDER BY connector_key`, strings.TrimSpace(principalID))
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	keys := make([]string, 0)
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, 0, err
		}
		keys = append(keys, key)
	}
	var revision uint64
	_ = store.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(revision), 0) FROM connector_agent_grant_metadata`).Scan(&revision)
	return keys, revision, rows.Err()
}

// ResolveAgentTargetPrincipal maps the exact Agent target used by a runtime to
// the stable Principal used by global Connector grants. Agent target IDs are
// opaque: Workspace Agent IDs retain their workspace-agent: prefix.
func (store *Store) ResolveAgentTargetPrincipal(ctx context.Context, workspaceID, targetID string) (agentprincipalbiz.Principal, error) {
	workspaceID, targetID = strings.TrimSpace(workspaceID), strings.TrimSpace(targetID)
	if targetID == "" {
		return agentprincipalbiz.Principal{}, ErrAgentPrincipalNotFound
	}
	return resolveAgentTargetPrincipal(ctx, store.db, workspaceID, targetID)
}

const principalSelect = `SELECT principal_id, kind, name, description, harness_agent_target_id,
source_workspace_id, source_agent_id, source_revision, created_at_unix_ms, updated_at_unix_ms
FROM agent_principals`

type principalScanner interface{ Scan(...any) error }

type principalSourceReader interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func scanPrincipal(row principalScanner) (agentprincipalbiz.Principal, error) {
	var principal agentprincipalbiz.Principal
	var createdAt, updatedAt int64
	destinations := []any{&principal.ID, &principal.Kind, &principal.Name, &principal.Description,
		&principal.HarnessAgentTargetID, &principal.SourceWorkspaceID, &principal.SourceAgentID,
		&principal.SourceRevision, &createdAt, &updatedAt}
	if err := row.Scan(destinations...); err != nil {
		return agentprincipalbiz.Principal{}, err
	}
	principal.CreatedAt, principal.UpdatedAt = time.UnixMilli(createdAt).UTC(), time.UnixMilli(updatedAt).UTC()
	return principal, nil
}

func resolveAgentTargetPrincipal(ctx context.Context, source principalSourceReader, workspaceID, targetID string) (agentprincipalbiz.Principal, error) {
	if strings.HasPrefix(targetID, "workspace-agent:") {
		var name, description, harnessTargetID string
		var revision, createdAt, updatedAt int64
		if err := source.QueryRowContext(ctx, `
SELECT name, description, harness_agent_target_id, revision, created_at_unix_ms, updated_at_unix_ms
FROM workspace_agents WHERE workspace_id = ? AND agent_id = ?`, workspaceID, targetID).
			Scan(&name, &description, &harnessTargetID, &revision, &createdAt, &updatedAt); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return agentprincipalbiz.Principal{}, ErrAgentPrincipalNotFound
			}
			return agentprincipalbiz.Principal{}, err
		}
		principal := agentprincipalbiz.Principal{ID: agentprincipalbiz.WorkspaceAgentID(workspaceID, targetID), Kind: agentprincipalbiz.KindWorkspaceAgent,
			Name: name, Description: description, HarnessAgentTargetID: harnessTargetID, SourceWorkspaceID: workspaceID,
			SourceAgentID: targetID, SourceRevision: revision, CreatedAt: time.UnixMilli(createdAt).UTC(), UpdatedAt: time.UnixMilli(updatedAt).UTC()}
		return principal, nil
	}
	var name string
	var createdAt, updatedAt int64
	if err := source.QueryRowContext(ctx, `SELECT name, created_at_ms, updated_at_ms FROM agent_targets WHERE id = ?`, targetID).
		Scan(&name, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return agentprincipalbiz.Principal{}, ErrAgentPrincipalNotFound
		}
		return agentprincipalbiz.Principal{}, err
	}
	principal := agentprincipalbiz.Principal{ID: agentprincipalbiz.SystemTargetID(targetID), Kind: agentprincipalbiz.KindSystemTarget,
		Name: name, HarnessAgentTargetID: targetID, CreatedAt: time.UnixMilli(createdAt).UTC(), UpdatedAt: time.UnixMilli(updatedAt).UTC()}
	return principal, nil
}

func upsertPrincipal(ctx context.Context, tx *sql.Tx, principal agentprincipalbiz.Principal) error {
	principal, err := agentprincipalbiz.Normalize(principal)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO agent_principals (
  principal_id, kind, name, description, harness_agent_target_id,
  source_workspace_id, source_agent_id, source_revision,
  created_at_unix_ms, updated_at_unix_ms
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(principal_id) DO UPDATE SET
  name = excluded.name,
  description = excluded.description,
  harness_agent_target_id = excluded.harness_agent_target_id,
  source_revision = excluded.source_revision,
  updated_at_unix_ms = excluded.updated_at_unix_ms`, principal.ID, principal.Kind, principal.Name,
		principal.Description, principal.HarnessAgentTargetID, principal.SourceWorkspaceID, principal.SourceAgentID,
		principal.SourceRevision, principal.CreatedAt.UnixMilli(), principal.UpdatedAt.UnixMilli())
	return err
}

func normalizeIDs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func isMissingTableError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "no such table")
}
