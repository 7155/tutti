package connectormarket

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	agentprincipalbiz "github.com/tutti-os/tutti/services/tuttid/biz/agentprincipal"
	cliservice "github.com/tutti-os/tutti/services/tuttid/service/cli"
)

type brokerAccessStub struct {
	granted bool
}

type sessionCapabilityResolverStub struct{}

func (sessionCapabilityResolverStub) ResolveSessionCapability(context.Context, string) (agentprincipalbiz.SessionIdentity, error) {
	return agentprincipalbiz.SessionIdentity{WorkspaceID: "workspace-1", AgentSessionID: "session-1",
		Principal: agentprincipalbiz.Principal{ID: "principal-1"}}, nil
}

func (stub brokerAccessStub) GrantedConnectorKeys(context.Context, string) ([]string, uint64, error) {
	if !stub.granted {
		return []string{}, 2, nil
	}
	return []string{"demo"}, 2, nil
}

func (stub brokerAccessStub) PrincipalHasConnectorGrant(context.Context, string, string) (bool, error) {
	return stub.granted, nil
}

func TestConnectorBrokerDiscoversSkillsAndInvokesInternalCapability(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "skills", "diagnostic")
	if err := os.MkdirAll(skillDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tutti.connector.json"), []byte(`{"name":{"en-US":"Demo Package"},"description":{"en-US":"Package description"},"skills":["./skills/diagnostic/SKILL.md"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: run-diagnostic\ndescription: Run one diagnostic.\n---\n\n# Run Diagnostic\n\nUse the broker.\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	commands := NewConnectorCommandRegistry()
	commands.routes["workspace-1\x00demo"] = &connectorRoute{id: "workspace-1\x00demo", connectionID: "workspace-1", connectorKey: "demo",
		displayName: "Demo", description: "Demo connector", installedRoot: root,
		capabilities: map[string]connectorCommand{"connector.demo.cli.diagnostic.describe": {
			capability: cliservice.Capability{ID: "connector.demo.cli.diagnostic.describe"},
			invoke: func(context.Context, cliservice.InvokeRequest) (cliservice.CommandOutput, error) {
				return jsonValue(map[string]any{"ok": true}), nil
			},
		}}}
	broker, err := NewConnectorBroker(commands, sessionCapabilityResolverStub{}, brokerAccessStub{granted: true})
	if err != nil {
		t.Fatal(err)
	}
	capabilities := broker.Capabilities(context.Background(), cliservice.InvokeContext{})
	if len(capabilities) != 4 || capabilities[0].ID != connectorAvailableCommandID || capabilities[3].ID != connectorInvokeCommandID {
		t.Fatalf("broker capabilities = %#v", capabilities)
	}
	contextValue := cliservice.InvokeContext{ConnectorSessionCapability: "capability"}
	available, err := broker.Invoke(context.Background(), cliservice.InvokeRequest{CommandID: connectorAvailableCommandID, Context: contextValue})
	if err != nil || len(available.Value["connectors"].([]any)) != 1 {
		t.Fatalf("available = %#v err = %v", available, err)
	}
	if available.Value["connectors"].([]any)[0].(map[string]any)["name"] != "Demo Package" {
		t.Fatalf("available did not use connector.json: %#v", available.Value)
	}
	skills, err := broker.Invoke(context.Background(), cliservice.InvokeRequest{CommandID: connectorSkillsCommandID,
		Input: map[string]any{"connector": "demo"}, Context: contextValue})
	if err != nil {
		t.Fatal(err)
	}
	skillItems := skills.Value["skills"].([]any)
	if len(skillItems) != 1 || skillItems[0].(map[string]any)["title"] != "Run Diagnostic" {
		t.Fatalf("skills = %#v", skills.Value)
	}
	read, err := broker.Invoke(context.Background(), cliservice.InvokeRequest{CommandID: connectorSkillReadCommandID,
		Input: map[string]any{"connector": "demo", "skill": "run-diagnostic"}, Context: contextValue})
	if err != nil || read.Value["content"] == "" {
		t.Fatalf("read = %#v err = %v", read, err)
	}
	invoked, err := broker.Invoke(context.Background(), cliservice.InvokeRequest{CommandID: connectorInvokeCommandID,
		Input: map[string]any{"connector": "demo", "capability": "diagnostic.describe", "input-json": `{"message":"ok"}`}, Context: contextValue})
	if err != nil || invoked.Value["ok"] != true {
		t.Fatalf("invoke = %#v err = %v", invoked, err)
	}
}

func TestConnectorBrokerDeniesConnectorWithoutGrant(t *testing.T) {
	broker, err := NewConnectorBroker(NewConnectorCommandRegistry(), sessionCapabilityResolverStub{}, brokerAccessStub{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = broker.Invoke(context.Background(), cliservice.InvokeRequest{CommandID: connectorSkillsCommandID,
		Input: map[string]any{"connector": "demo"}, Context: cliservice.InvokeContext{ConnectorSessionCapability: "capability"}})
	if err == nil || cliservice.InvokeErrorReason(err) != "connector_access_denied" {
		t.Fatalf("denied error = %v", err)
	}
}
