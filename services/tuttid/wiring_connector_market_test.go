package main

import (
	"crypto/ed25519"
	"encoding/hex"
	"testing"
)

func TestConnectorMarketDefaultUsesDesktopGateway(t *testing.T) {
	const expected = "https://api.tutti.sh/api/desktop"
	if connectorMarketDefaultBaseURL != expected {
		t.Fatalf("connector market base URL = %q, want %q", connectorMarketDefaultBaseURL, expected)
	}
}

func TestConnectorMarketSigningKeysFromEnvironment(t *testing.T) {
	publicKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TUTTI_CONNECTOR_MARKET_SIGNING_KEYRING_JSON", `{"version":7,"keys":{"market-key":"`+hex.EncodeToString(publicKey)+`"}}`)
	version, keys, err := connectorMarketSigningKeysFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	if got := keys["market-key"]; version != 7 || len(got) != ed25519.PublicKeySize || !got.Equal(publicKey) {
		t.Fatalf("decoded key = %x", got)
	}
}

func TestConnectorMarketSigningKeysRejectsInvalidEnvironment(t *testing.T) {
	t.Setenv("TUTTI_CONNECTOR_MARKET_SIGNING_KEYRING_JSON", `{"version":1,"keys":{"market-key":"00"}}`)
	if _, _, err := connectorMarketSigningKeysFromEnvironment(); err == nil {
		t.Fatal("expected invalid trust root")
	}
}
