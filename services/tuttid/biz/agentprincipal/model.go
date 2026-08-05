// Package agentprincipal defines the stable, workspace-independent identity
// used by connector authorization. Workspace Agents remain workspace-scoped
// configuration records; a Principal is the durable subject a grant targets.
package agentprincipal

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

type Kind string

const (
	KindWorkspaceAgent Kind = "workspace_agent"
	KindSystemTarget   Kind = "system_target"
)

var ErrInvalidPrincipal = errors.New("invalid agent principal")

type Principal struct {
	ID                   string    `json:"id"`
	Kind                 Kind      `json:"kind"`
	Name                 string    `json:"name"`
	Description          string    `json:"description,omitempty"`
	HarnessAgentTargetID string    `json:"harnessAgentTargetId"`
	SourceWorkspaceID    string    `json:"sourceWorkspaceId,omitempty"`
	SourceAgentID        string    `json:"sourceAgentId,omitempty"`
	SourceRevision       int64     `json:"sourceRevision"`
	CreatedAt            time.Time `json:"createdAt"`
	UpdatedAt            time.Time `json:"updatedAt"`
}

type SessionIdentity struct {
	WorkspaceID    string
	AgentSessionID string
	Principal      Principal
}

func WorkspaceAgentID(workspaceID, agentID string) string {
	return deterministicID("principal:agent:", workspaceID, agentID)
}

func SystemTargetID(agentTargetID string) string {
	return deterministicID("principal:system:", agentTargetID)
}

func Normalize(principal Principal) (Principal, error) {
	principal.ID = strings.TrimSpace(principal.ID)
	principal.Name = strings.TrimSpace(principal.Name)
	principal.Description = strings.TrimSpace(principal.Description)
	principal.HarnessAgentTargetID = strings.TrimSpace(principal.HarnessAgentTargetID)
	principal.SourceWorkspaceID = strings.TrimSpace(principal.SourceWorkspaceID)
	principal.SourceAgentID = strings.TrimSpace(principal.SourceAgentID)
	if principal.ID == "" || principal.Name == "" || principal.HarnessAgentTargetID == "" {
		return Principal{}, ErrInvalidPrincipal
	}
	switch principal.Kind {
	case KindWorkspaceAgent:
		if principal.SourceWorkspaceID == "" || principal.SourceAgentID == "" {
			return Principal{}, ErrInvalidPrincipal
		}
	case KindSystemTarget:
		if principal.SourceWorkspaceID != "" || principal.SourceAgentID != "" {
			return Principal{}, ErrInvalidPrincipal
		}
	default:
		return Principal{}, ErrInvalidPrincipal
	}
	return principal, nil
}

func deterministicID(prefix string, parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		hash.Write([]byte(strings.TrimSpace(part)))
		hash.Write([]byte{0})
	}
	return prefix + hex.EncodeToString(hash.Sum(nil)[:16])
}
