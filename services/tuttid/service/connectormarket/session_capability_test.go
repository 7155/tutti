package connectormarket

import (
	"context"
	"errors"
	"testing"
	"time"

	agentprincipalbiz "github.com/tutti-os/tutti/services/tuttid/biz/agentprincipal"
)

type sessionCapabilityPrincipalStub struct {
	principal agentprincipalbiz.Principal
	err       error
}

func (stub *sessionCapabilityPrincipalStub) ResolveAgentTargetPrincipal(context.Context, string, string) (agentprincipalbiz.Principal, error) {
	return stub.principal, stub.err
}

func TestSessionCapabilityBindsLiveSessionAndCurrentPrincipal(t *testing.T) {
	ctx := context.Background()
	principal := &sessionCapabilityPrincipalStub{principal: agentprincipalbiz.Principal{
		ID: "principal-1", Kind: agentprincipalbiz.KindWorkspaceAgent, Name: "Researcher",
		HarnessAgentTargetID: "local:codex", SourceWorkspaceID: "workspace-1",
		SourceAgentID: "workspace-agent:researcher",
	}}
	active, activeTargetID := false, "workspace-agent:researcher"
	authority, err := NewSessionCapabilityAuthority(SessionCapabilityAuthorityConfig{
		Secret:     []byte("0123456789abcdef0123456789abcdef"),
		Principals: principal,
		ResolveActiveSession: func(context.Context, string, string) (string, bool, error) {
			return activeTargetID, active, nil
		},
		Now: func() time.Time { return time.UnixMilli(1234).UTC() },
	})
	if err != nil {
		t.Fatal(err)
	}

	capability, err := authority.IssueSessionCapability(ctx, "workspace-1", "session-1", "workspace-agent:researcher")
	if err != nil || capability == "" {
		t.Fatalf("issue capability = %q err = %v", capability, err)
	}
	if _, err := authority.ResolveSessionCapability(ctx, capability); !errors.Is(err, ErrSessionCapabilityInvalid) {
		t.Fatalf("inactive Session resolution error = %v", err)
	}

	active = true
	identity, err := authority.ResolveSessionCapability(ctx, capability)
	if err != nil {
		t.Fatal(err)
	}
	if identity.WorkspaceID != "workspace-1" || identity.AgentSessionID != "session-1" || identity.Principal.ID != "principal-1" {
		t.Fatalf("identity = %#v", identity)
	}

	activeTargetID = "workspace-agent:other"
	if _, err := authority.ResolveSessionCapability(ctx, capability); !errors.Is(err, ErrSessionCapabilityInvalid) {
		t.Fatalf("retargeted Session resolution error = %v", err)
	}
}

func TestSessionCapabilityRejectsTamperingAndPrincipalRemoval(t *testing.T) {
	ctx := context.Background()
	principal := &sessionCapabilityPrincipalStub{principal: agentprincipalbiz.Principal{ID: "principal-1"}}
	authority, err := NewSessionCapabilityAuthority(SessionCapabilityAuthorityConfig{
		Secret:     []byte("0123456789abcdef0123456789abcdef"),
		Principals: principal,
		ResolveActiveSession: func(context.Context, string, string) (string, bool, error) {
			return "local:codex", true, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	capability, err := authority.IssueSessionCapability(ctx, "workspace-1", "session-1", "local:codex")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authority.ResolveSessionCapability(ctx, capability+"x"); !errors.Is(err, ErrSessionCapabilityInvalid) {
		t.Fatalf("tampered capability error = %v", err)
	}
	restarted, err := NewSessionCapabilityAuthority(SessionCapabilityAuthorityConfig{
		Secret:     []byte("abcdef0123456789abcdef0123456789"),
		Principals: principal,
		ResolveActiveSession: func(context.Context, string, string) (string, bool, error) {
			return "local:codex", true, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.ResolveSessionCapability(ctx, capability); !errors.Is(err, ErrSessionCapabilityInvalid) {
		t.Fatalf("previous daemon capability error = %v", err)
	}
	principal.err = errors.New("Agent target was deleted")
	if _, err := authority.ResolveSessionCapability(ctx, capability); err == nil {
		t.Fatal("deleted Agent target remained authorized")
	}
}
