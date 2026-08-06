package agentstatus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tutti-os/tutti/packages/agent/daemon/providerregistry"
)

// authCredentialsRefreshedAfter reports whether any of the provider's credential
// marker files was modified after the given time — i.e. a login rewrote the
// credentials since the recorded auth failure.
func (s Service) authCredentialsRefreshedAfter(spec ProviderSpec, since time.Time) bool {
	paths, _ := s.resolvedAuthMarkerPaths(spec)
	for _, path := range paths {
		if mod, ok := s.fileModTime(path); ok && mod.After(since) {
			return true
		}
	}
	return false
}

func (s Service) resolveAuthFromMarkers(spec ProviderSpec) AuthInfo {
	auth, definitive := s.resolveAuthFromMarkersWithValidity(spec)
	if definitive {
		return auth
	}
	return AuthInfo{Status: AuthUnknown}
}

func (s Service) resolveAuthFromMarkersWithValidity(spec ProviderSpec) (AuthInfo, bool) {
	if len(spec.AuthMarkerPaths) == 0 {
		return AuthInfo{Status: AuthUnknown}, true
	}

	paths, complete := s.resolvedAuthMarkerPaths(spec)
	invalidMarker := false
	for _, path := range paths {
		if !s.fileExists(path) {
			continue
		}
		if auth, ok := s.authFromMarkerFile(spec, path); ok {
			return auth, true
		}
		invalidMarker = true
	}
	if invalidMarker || !complete {
		return AuthInfo{Status: AuthUnknown}, false
	}
	return AuthInfo{Status: AuthRequired}, true
}

func (s Service) resolvedAuthMarkerPaths(spec ProviderSpec) ([]string, bool) {
	markers := append([]string(nil), spec.AuthMarkerPaths...)
	if authMarkerParserKind(spec) == providerregistry.AuthMarkerParserKindOpenCode {
		if dataHome := strings.TrimSpace(s.lookupEnv("XDG_DATA_HOME")); dataHome != "" {
			markers = append([]string{filepath.Join(dataHome, "opencode", "auth.json")}, markers...)
		}
	}
	if len(markers) == 0 {
		return nil, true
	}

	home, err := s.homeDir()
	homeAvailable := err == nil && strings.TrimSpace(home) != ""
	complete := true
	paths := make([]string, 0, len(markers))
	seen := make(map[string]struct{}, len(markers))
	for _, marker := range markers {
		marker = strings.TrimSpace(marker)
		if marker == "" {
			continue
		}
		if (marker == "~" || strings.HasPrefix(marker, "~/")) && !homeAvailable {
			complete = false
			continue
		}
		path := filepath.Clean(expandHomePath(marker, home))
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	return paths, complete
}

func (s Service) authFromMarkerFile(spec ProviderSpec, path string) (AuthInfo, bool) {
	if !s.fileExists(path) {
		return AuthInfo{}, false
	}
	switch authMarkerParserKind(spec) {
	case providerregistry.AuthMarkerParserKindClaude:
		if auth, ok := parseClaudeAuthMarkerFile(path); ok {
			return auth, true
		}
		return AuthInfo{}, false
	case providerregistry.AuthMarkerParserKindOpenCode:
		if auth, ok := parseOpenCodeAuthMarkerFile(path); ok {
			return auth, true
		}
		return AuthInfo{}, false
	case providerregistry.AuthMarkerParserKindTuttiToken:
		if auth, ok := parseTuttiAgentAuthMarkerFile(path); ok {
			return auth, true
		}
		return AuthInfo{}, false
	case providerregistry.AuthMarkerParserKindFileExists:
		return AuthInfo{Status: AuthAuthenticated}, true
	default:
		return AuthInfo{}, false
	}
}

func authMarkerParserKind(spec ProviderSpec) providerregistry.AuthMarkerParserKind {
	if spec.AuthMarkerParserKind != "" {
		return spec.AuthMarkerParserKind
	}
	if status, ok := migratedProviderStatus(spec.Provider); ok {
		return status.AuthMarkerParserKind
	}
	return ""
}

func authMarkerIsAuthoritative(spec ProviderSpec) bool {
	switch authMarkerParserKind(spec) {
	case providerregistry.AuthMarkerParserKindOpenCode, providerregistry.AuthMarkerParserKindTuttiToken:
		return true
	default:
		return false
	}
}

func parseOpenCodeAuthMarkerFile(path string) (AuthInfo, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return AuthInfo{}, false
	}
	var credentials map[string]json.RawMessage
	if err := json.Unmarshal(raw, &credentials); err != nil {
		return AuthInfo{}, false
	}
	if len(credentials) == 0 {
		return AuthInfo{Status: AuthRequired}, true
	}
	return AuthInfo{Status: AuthAuthenticated}, true
}

// parseTuttiAgentAuthMarkerFile validates that the Tutti Agent auth.json holds
// a usable `tutti_llm` token bundle instead of treating file existence as
// authenticated.
func parseTuttiAgentAuthMarkerFile(path string) (AuthInfo, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return AuthInfo{}, false
	}
	var payload struct {
		TuttiLLM *struct {
			AppID                string          `json:"app_id"`
			AccessToken          string          `json:"access_token"`
			AccessTokenExpiresAt json.RawMessage `json:"access_token_expires_at"`
			RefreshToken         string          `json:"refresh_token"`
		} `json:"tutti_llm"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return AuthInfo{}, false
	}
	if payload.TuttiLLM == nil ||
		strings.TrimSpace(payload.TuttiLLM.AccessToken) == "" ||
		strings.TrimSpace(payload.TuttiLLM.RefreshToken) == "" {
		return AuthInfo{Status: AuthRequired}, true
	}
	// A stale auth marker is not a login.  The CLI's `login status` command
	// only checks that the marker exists, while the app-server will immediately
	// try to refresh an expired access token.  Treat an explicitly expired
	// access token as auth-required so the desktop can route the user back
	// through the Tutti account login flow instead of reporting a false-ready
	// provider.
	if expiresAt, ok := parseTuttiAgentTokenExpiry(payload.TuttiLLM.AccessTokenExpiresAt); ok &&
		!time.Now().UTC().Before(expiresAt) {
		return AuthInfo{Status: AuthRequired}, true
	}
	// app_id identifies the Tutti LLM application, not the signed-in user.
	// The marker does not contain user-facing account identity, so leave the
	// account label empty rather than showing the shared application ID.
	return AuthInfo{
		AuthMethod: "tutti_llm",
		Status:     AuthAuthenticated,
	}, true
}

func parseTuttiAgentTokenExpiry(raw json.RawMessage) (time.Time, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return time.Time{}, false
	}
	var numeric int64
	if err := json.Unmarshal(raw, &numeric); err == nil && numeric > 0 {
		return time.Unix(numeric, 0).UTC(), true
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return time.Time{}, false
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return time.Time{}, false
	}
	if parsed, err := time.Parse(time.RFC3339, text); err == nil {
		return parsed.UTC(), true
	}
	numeric, err := strconv.ParseInt(text, 10, 64)
	if err != nil || numeric <= 0 {
		return time.Time{}, false
	}
	return time.Unix(numeric, 0).UTC(), true
}
