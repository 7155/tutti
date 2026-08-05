package connectormarket

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	market "github.com/tutti-os/tutti/packages/connector/market/daemon"
	agentprincipalbiz "github.com/tutti-os/tutti/services/tuttid/biz/agentprincipal"
)

func TestAgentPrincipalMigrationAndExactAgentTargetResolution(t *testing.T) {
	ctx := context.Background()
	databasePath := filepath.Join(t.TempDir(), "tuttid.db")
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `
CREATE TABLE workspace_agents (
  workspace_id TEXT NOT NULL, agent_id TEXT NOT NULL, name TEXT NOT NULL,
  description TEXT NOT NULL, harness_agent_target_id TEXT NOT NULL,
  revision INTEGER NOT NULL, created_at_unix_ms INTEGER NOT NULL,
  updated_at_unix_ms INTEGER NOT NULL,
  PRIMARY KEY (workspace_id, agent_id)
);
CREATE TABLE agent_targets (
  id TEXT PRIMARY KEY, name TEXT NOT NULL, created_at_ms INTEGER NOT NULL,
  updated_at_ms INTEGER NOT NULL
);
INSERT INTO workspace_agents VALUES (
  'workspace-1', 'workspace-agent:researcher', 'Researcher', 'Finds evidence', 'local:codex', 3, 1000, 2000
);
INSERT INTO agent_targets VALUES ('local:codex', 'Codex', 1000, 2000);`); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(ctx, databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	principals, err := store.ListAgentPrincipals(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(principals) != 2 {
		t.Fatalf("principals = %#v", principals)
	}
	principalID := agentprincipalbiz.WorkspaceAgentID("workspace-1", "workspace-agent:researcher")
	principal, err := store.AgentPrincipal(ctx, principalID)
	if err != nil {
		t.Fatal(err)
	}
	if principal.Name != "Researcher" || principal.SourceRevision != 3 {
		t.Fatalf("principal = %#v", principal)
	}

	connector := testConnector()
	if err := store.Transaction(ctx, func(tx market.Transaction) error { return tx.SaveConnector(connector) }); err != nil {
		t.Fatal(err)
	}
	revision, err := store.ReplaceAgentGrants(ctx, connector.Key, []string{principalID, principalID}, 0)
	if err != nil {
		t.Fatal(err)
	}
	grants, persistedRevision, err := store.AgentGrants(ctx, connector.Key)
	if err != nil {
		t.Fatal(err)
	}
	if revision != 1 || persistedRevision != revision || len(grants) != 1 || grants[0] != principalID {
		t.Fatalf("grants = %#v revision = %d/%d", grants, revision, persistedRevision)
	}

	resolved, err := store.ResolveAgentTargetPrincipal(ctx, "workspace-1", "workspace-agent:researcher")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ID != principalID || resolved.SourceAgentID != "workspace-agent:researcher" {
		t.Fatalf("resolved principal = %#v", resolved)
	}
	if _, err := store.db.ExecContext(ctx, `DELETE FROM workspace_agents WHERE workspace_id = ? AND agent_id = ?`,
		"workspace-1", "workspace-agent:researcher"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveAgentTargetPrincipal(ctx, "workspace-1", "workspace-agent:researcher"); !errors.Is(err, ErrAgentPrincipalNotFound) {
		t.Fatalf("resolve deleted Agent principal error = %v", err)
	}
	var legacyTokenTable bool
	if err := store.db.QueryRowContext(ctx, `SELECT EXISTS(
SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'connector_session_execution_tokens')`).Scan(&legacyTokenTable); err != nil {
		t.Fatal(err)
	}
	if legacyTokenTable {
		t.Fatal("legacy Connector Session token table still exists")
	}
}
