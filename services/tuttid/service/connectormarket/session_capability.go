package connectormarket

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	agentprincipalbiz "github.com/tutti-os/tutti/services/tuttid/biz/agentprincipal"
)

const (
	sessionCapabilityPrefix    = "tutti-connector-session-v1"
	sessionCapabilityScope     = "connector-broker"
	sessionCapabilitySecretLen = 32
	sessionCapabilityMaxLen    = 4096
)

var ErrSessionCapabilityInvalid = errors.New("connector session capability is invalid")

type SessionCapabilityPrincipalResolver interface {
	ResolveAgentTargetPrincipal(context.Context, string, string) (agentprincipalbiz.Principal, error)
}

type ActiveAgentSessionResolver func(context.Context, string, string) (agentTargetID string, active bool, err error)

type SessionCapabilityAuthorityConfig struct {
	Secret               []byte
	Principals           SessionCapabilityPrincipalResolver
	ResolveActiveSession ActiveAgentSessionResolver
	Now                  func() time.Time
}

// SessionCapabilityAuthority issues daemon-local, stateless capabilities for
// the Connector broker. A capability is useful only while the exact Agent
// Session is live and still resolves to the signed Agent Principal.
type SessionCapabilityAuthority struct {
	secret               []byte
	principals           SessionCapabilityPrincipalResolver
	resolveActiveSession ActiveAgentSessionResolver
	now                  func() time.Time
}

type sessionCapabilityClaims struct {
	WorkspaceID    string `json:"workspaceId"`
	AgentSessionID string `json:"agentSessionId"`
	AgentTargetID  string `json:"agentTargetId"`
	PrincipalID    string `json:"principalId"`
	Scope          string `json:"scope"`
	IssuedAtUnixMS int64  `json:"issuedAtUnixMs"`
}

func NewSessionCapabilityAuthority(config SessionCapabilityAuthorityConfig) (*SessionCapabilityAuthority, error) {
	if config.Principals == nil || config.ResolveActiveSession == nil {
		return nil, errors.New("connector session capability dependencies are required")
	}
	secret := append([]byte(nil), config.Secret...)
	if len(secret) == 0 {
		secret = make([]byte, sessionCapabilitySecretLen)
		if _, err := rand.Read(secret); err != nil {
			return nil, fmt.Errorf("generate Connector Session capability secret: %w", err)
		}
	}
	if len(secret) < sessionCapabilitySecretLen {
		return nil, errors.New("connector session capability secret must be at least 32 bytes")
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &SessionCapabilityAuthority{
		secret: secret, principals: config.Principals,
		resolveActiveSession: config.ResolveActiveSession, now: now,
	}, nil
}

func (authority *SessionCapabilityAuthority) IssueSessionCapability(
	ctx context.Context,
	workspaceID string,
	agentSessionID string,
	agentTargetID string,
) (string, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	agentSessionID = strings.TrimSpace(agentSessionID)
	agentTargetID = strings.TrimSpace(agentTargetID)
	if authority == nil || workspaceID == "" || agentSessionID == "" || agentTargetID == "" {
		return "", ErrSessionCapabilityInvalid
	}
	principal, err := authority.principals.ResolveAgentTargetPrincipal(ctx, workspaceID, agentTargetID)
	if err != nil {
		return "", fmt.Errorf("resolve Agent Principal for Connector Session capability: %w", err)
	}
	claims := sessionCapabilityClaims{
		WorkspaceID: workspaceID, AgentSessionID: agentSessionID, AgentTargetID: agentTargetID,
		PrincipalID: principal.ID, Scope: sessionCapabilityScope, IssuedAtUnixMS: authority.now().UTC().UnixMilli(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("encode Connector Session capability: %w", err)
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	signature := authority.sign(encodedPayload)
	return sessionCapabilityPrefix + "." + encodedPayload + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (authority *SessionCapabilityAuthority) ResolveSessionCapability(
	ctx context.Context,
	capability string,
) (agentprincipalbiz.SessionIdentity, error) {
	claims, err := authority.verify(capability)
	if err != nil {
		return agentprincipalbiz.SessionIdentity{}, err
	}
	activeTargetID, active, err := authority.resolveActiveSession(ctx, claims.WorkspaceID, claims.AgentSessionID)
	if err != nil {
		return agentprincipalbiz.SessionIdentity{}, fmt.Errorf("resolve active Agent Session for Connector capability: %w", err)
	}
	if !active || strings.TrimSpace(activeTargetID) != claims.AgentTargetID {
		return agentprincipalbiz.SessionIdentity{}, ErrSessionCapabilityInvalid
	}
	principal, err := authority.principals.ResolveAgentTargetPrincipal(ctx, claims.WorkspaceID, claims.AgentTargetID)
	if err != nil {
		return agentprincipalbiz.SessionIdentity{}, fmt.Errorf("resolve current Agent Principal for Connector capability: %w", err)
	}
	if !hmac.Equal([]byte(principal.ID), []byte(claims.PrincipalID)) {
		return agentprincipalbiz.SessionIdentity{}, ErrSessionCapabilityInvalid
	}
	return agentprincipalbiz.SessionIdentity{
		WorkspaceID: claims.WorkspaceID, AgentSessionID: claims.AgentSessionID, Principal: principal,
	}, nil
}

func (authority *SessionCapabilityAuthority) verify(capability string) (sessionCapabilityClaims, error) {
	capability = strings.TrimSpace(capability)
	if authority == nil || capability == "" || len(capability) > sessionCapabilityMaxLen {
		return sessionCapabilityClaims{}, ErrSessionCapabilityInvalid
	}
	parts := strings.Split(capability, ".")
	if len(parts) != 3 || parts[0] != sessionCapabilityPrefix {
		return sessionCapabilityClaims{}, ErrSessionCapabilityInvalid
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(signature, authority.sign(parts[1])) {
		return sessionCapabilityClaims{}, ErrSessionCapabilityInvalid
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return sessionCapabilityClaims{}, ErrSessionCapabilityInvalid
	}
	var claims sessionCapabilityClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return sessionCapabilityClaims{}, ErrSessionCapabilityInvalid
	}
	claims.WorkspaceID = strings.TrimSpace(claims.WorkspaceID)
	claims.AgentSessionID = strings.TrimSpace(claims.AgentSessionID)
	claims.AgentTargetID = strings.TrimSpace(claims.AgentTargetID)
	claims.PrincipalID = strings.TrimSpace(claims.PrincipalID)
	if claims.WorkspaceID == "" || claims.AgentSessionID == "" || claims.AgentTargetID == "" ||
		claims.PrincipalID == "" || claims.Scope != sessionCapabilityScope || claims.IssuedAtUnixMS <= 0 {
		return sessionCapabilityClaims{}, ErrSessionCapabilityInvalid
	}
	return claims, nil
}

func (authority *SessionCapabilityAuthority) sign(encodedPayload string) []byte {
	mac := hmac.New(sha256.New, authority.secret)
	_, _ = mac.Write([]byte(sessionCapabilityPrefix))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(encodedPayload))
	return mac.Sum(nil)
}
