package runtimeprep

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestHermesPreparerAddsExtensionSkillRootsToSessionConfig 守住 hermes
// extension 的 skill 加载根因修复。hermes-agent 从 $HERMES_HOME/config.yaml 的
// skills.external_dirs 发现外部 skill，因此 HermesPreparer 复用 extension profile
// 声明的 workspace root，把 tutti skills 稳定物化到该 root，再把 root 加入
// per-session config overlay。AGENTS.md 仍写到 cwd。
func TestHermesPreparerAddsExtensionSkillRootsToSessionConfig(t *testing.T) {
	// 模拟用户全局 hermes home（含 config + auth + .env），用 HERMES_HOME 指向它做隔离。
	globalHome := t.TempDir()
	t.Setenv("HERMES_HOME", globalHome)
	globalSkills := filepath.Join(globalHome, "skills")
	if err := os.MkdirAll(globalSkills, 0o755); err != nil {
		t.Fatalf("create global skills: %v", err)
	}
	globalFiles := map[string][]byte{
		"config.yaml": []byte("model: test-model\nproviders: {}\nskills:\n  external_dirs:\n    - \"/already-configured\"\n"),
		"auth.json":   []byte(`{"version":1,"providers":{}}`),
		".env":        []byte("OPENCODE_ZEN_API_KEY=test-key\n"),
	}
	for name, content := range globalFiles {
		if err := os.WriteFile(filepath.Join(globalHome, name), content, 0o600); err != nil {
			t.Fatalf("write global %s: %v", name, err)
		}
	}

	stateDir := t.TempDir()
	cwd := t.TempDir()
	prep := NewDefaultPreparer(stateDir)
	prep.CommandCatalog = staticCommandCatalog(nil)
	prepared, err := prep.Prepare(t.Context(), PrepareInput{
		WorkspaceID:    "workspace-1",
		AgentSessionID: "session-1",
		AgentTargetID:  "local:hermes",
		Provider:       "acp:hermes",
		Cwd:            cwd,
		ExtensionSkillRoots: []string{
			".agent_context/skills",
			".agent_context/skills",
		},
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}

	hermesHome := ""
	for _, env := range prepared.Env {
		if strings.HasPrefix(env, "HERMES_HOME=") {
			hermesHome = strings.TrimPrefix(env, "HERMES_HOME=")
		}
	}
	if hermesHome == "" {
		t.Fatalf("HERMES_HOME not set in prepared env; hermes cannot isolated-skill-load without it. env=%v", prepared.Env)
	}

	// auth.json + .env 必须从全局 home 复制进 per-session HERMES_HOME：
	// auth.json 带凭证，.env 带 provider API key。
	for name, want := range map[string][]byte{"auth.json": globalFiles["auth.json"], ".env": globalFiles[".env"]} {
		got, err := os.ReadFile(filepath.Join(hermesHome, name))
		if err != nil {
			t.Fatalf("%s not copied into per-session HERMES_HOME: %v", name, err)
		}
		if string(got) != string(want) {
			t.Fatalf("%s copy mismatch: want %q, got %q", name, want, got)
		}
	}

	workspaceSkillRoot := filepath.Join(cwd, ".agent_context", "skills")

	config, err := os.ReadFile(filepath.Join(hermesHome, "config.yaml"))
	if err != nil {
		t.Fatalf("session config.yaml missing: %v", err)
	}
	configText := string(config)
	for _, want := range []string{
		"model: test-model",
		"providers: {}",
		"- \"/already-configured\"",
		"- " + quoteYAMLString(workspaceSkillRoot),
		"- " + quoteYAMLString(globalSkills),
	} {
		if !strings.Contains(configText, want) {
			t.Fatalf("session config missing %q:\n%s", want, configText)
		}
	}
	if strings.Count(configText, workspaceSkillRoot) != 1 {
		t.Fatalf("workspace skill root should be de-duplicated in config:\n%s", configText)
	}

	// tutti skill 物化到 extension profile 声明的 workspace root，再由
	// skills.external_dirs 暴露给 hermes，而不是复制到 per-session HERMES_HOME/skills。
	for _, name := range []string{tuttiHandoffSkillName, tuttiSkillName} {
		skillPath := filepath.Join(workspaceSkillRoot, name, "SKILL.md")
		if _, err := os.Stat(skillPath); err != nil {
			t.Fatalf("skill %s SKILL.md missing in extension skill root: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(hermesHome, "skills")); !os.IsNotExist(err) {
		t.Fatalf("HERMES_HOME/skills should not be created or copied, got err=%v", err)
	}

	// AGENTS.md 仍写到 cwd（hermes 读 cwd/AGENTS.md 作为 mention routing 上下文）。
	if _, err := os.Stat(filepath.Join(cwd, "AGENTS.md")); err != nil {
		t.Fatalf("cwd AGENTS.md missing: %v", err)
	}
}

func TestHermesPreparerMaterializesBrowserUseSkillWhenEnabled(t *testing.T) {
	t.Setenv("HERMES_HOME", t.TempDir())
	stateDir := t.TempDir()
	cwd := t.TempDir()
	prep := NewDefaultPreparer(stateDir)
	prep.CommandCatalog = staticCommandCatalog(testCommandCapabilities())
	if _, err := prep.Prepare(t.Context(), PrepareInput{
		WorkspaceID:         "workspace-1",
		AgentSessionID:      "session-1",
		AgentTargetID:       "local:hermes",
		Provider:            "acp:hermes",
		Cwd:                 cwd,
		BrowserUse:          true,
		ExtensionSkillRoots: []string{".agent_context/skills"},
	}); err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	skillPath := filepath.Join(cwd, ".agent_context", "skills", browserUseSkillName, "SKILL.md")
	if _, err := os.Stat(skillPath); err != nil {
		t.Fatalf("browser-use SKILL.md missing in extension skill root: %v", err)
	}
}

func TestHermesPreparerDoesNotAdvertiseBrowserUseWithoutCommandCapabilities(t *testing.T) {
	t.Setenv("HERMES_HOME", t.TempDir())
	stateDir := t.TempDir()
	cwd := t.TempDir()
	prep := NewDefaultPreparer(stateDir)
	prep.CommandCatalog = staticCommandCatalog(nil)
	prepared, err := prep.Prepare(t.Context(), PrepareInput{
		WorkspaceID:         "workspace-1",
		AgentSessionID:      "session-1",
		AgentTargetID:       "local:hermes",
		Provider:            "acp:hermes",
		Cwd:                 cwd,
		BrowserUse:          true,
		ExtensionSkillRoots: []string{".agent_context/skills"},
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	for _, env := range prepared.Env {
		if strings.HasPrefix(env, browserUseEnabledSessionEnv+"=") {
			t.Fatalf("Hermes should not advertise browser use without browser command capabilities, env=%v", prepared.Env)
		}
	}
	skillPath := filepath.Join(cwd, ".agent_context", "skills", browserUseSkillName, "SKILL.md")
	if _, err := os.Stat(skillPath); !os.IsNotExist(err) {
		t.Fatalf("browser-use SKILL.md should not be materialized without browser command capabilities, err=%v", err)
	}
}

// TestHermesPreparerDoesNotMaterializeToAgentContextSkills 确认 hermes 的 skill
// 不再物化到 .agent_context/skills（hermes 不读该目录），避免无谓写入与重复目录。
func TestHermesPreparerDoesNotMaterializeToAgentContextSkills(t *testing.T) {
	t.Setenv("HERMES_HOME", t.TempDir())
	stateDir := t.TempDir()
	cwd := t.TempDir()
	prep := NewDefaultPreparer(stateDir)
	prep.CommandCatalog = staticCommandCatalog(nil)
	if _, err := prep.Prepare(t.Context(), PrepareInput{
		WorkspaceID:    "workspace-1",
		AgentSessionID: "session-1",
		AgentTargetID:  "local:hermes",
		Provider:       "acp:hermes",
		Cwd:            cwd,
	}); err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(cwd, ".agent_context", "skills")); !os.IsNotExist(err) {
		t.Fatalf(".agent_context/skills should not exist for hermes (hermes does not read it), got err=%v", err)
	}
}

func TestHermesPreparerPrepareTwiceIsIdempotent(t *testing.T) {
	globalHome := t.TempDir()
	t.Setenv("HERMES_HOME", globalHome)
	if err := os.WriteFile(filepath.Join(globalHome, "config.yaml"), []byte("model: test-model\n"), 0o600); err != nil {
		t.Fatalf("write global config: %v", err)
	}
	stateDir := t.TempDir()
	cwd := t.TempDir()
	input := PrepareInput{
		WorkspaceID:         "workspace-1",
		AgentSessionID:      "session-1",
		AgentTargetID:       "local:hermes",
		Provider:            "acp:hermes",
		Cwd:                 cwd,
		ExtensionSkillRoots: []string{".agent_context/skills"},
	}
	prep := NewDefaultPreparer(stateDir)
	prep.CommandCatalog = staticCommandCatalog(nil)
	if _, err := prep.Prepare(t.Context(), input); err != nil {
		t.Fatalf("first Prepare() error = %v", err)
	}
	if _, err := prep.Prepare(t.Context(), input); err != nil {
		t.Fatalf("second Prepare() error = %v", err)
	}
	root := filepath.Join(cwd, ".agent_context", "skills")
	if _, err := os.Stat(filepath.Join(root, tuttiSkillName, "SKILL.md")); err != nil {
		t.Fatalf("stable tutti skill missing: %v", err)
	}
	for _, unexpected := range []string{tuttiSkillName + "-tutti", tuttiSkillName + "-tutti-2", tuttiHandoffSkillName + "-tutti"} {
		if _, err := os.Stat(filepath.Join(root, unexpected)); !os.IsNotExist(err) {
			t.Fatalf("unexpected duplicate skill directory %s, err=%v", unexpected, err)
		}
	}
	runtimeRoot, err := LocalStore{StateDir: stateDir}.RuntimeRoot("workspace-1", "session-1")
	if err != nil {
		t.Fatalf("RuntimeRoot() error = %v", err)
	}
	config, err := os.ReadFile(filepath.Join(runtimeRoot, "hermes", "config.yaml"))
	if err != nil {
		t.Fatalf("read session config: %v", err)
	}
	if strings.Count(string(config), filepath.Join(cwd, ".agent_context", "skills")) != 1 {
		t.Fatalf("session config should contain one workspace root after retry:\n%s", config)
	}
}

func TestHermesPreparerSkipsGlobalCopiesWhenHomeUnavailable(t *testing.T) {
	t.Setenv("HERMES_HOME", "")
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	stateDir := t.TempDir()
	cwd := t.TempDir()
	for _, name := range []string{"config.yaml", "auth.json", ".env"} {
		if err := os.WriteFile(filepath.Join(cwd, name), []byte("must-not-copy"), 0o600); err != nil {
			t.Fatalf("write cwd %s: %v", name, err)
		}
	}
	if err := os.MkdirAll(filepath.Join(cwd, "skills", "native"), 0o755); err != nil {
		t.Fatalf("write cwd skills: %v", err)
	}
	prep := NewDefaultPreparer(stateDir)
	prep.CommandCatalog = staticCommandCatalog(nil)
	prepared, err := prep.Prepare(t.Context(), PrepareInput{
		WorkspaceID:    "workspace-1",
		AgentSessionID: "session-1",
		AgentTargetID:  "local:hermes",
		Provider:       "acp:hermes",
		Cwd:            cwd,
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	hermesHome := ""
	for _, env := range prepared.Env {
		if strings.HasPrefix(env, "HERMES_HOME=") {
			hermesHome = strings.TrimPrefix(env, "HERMES_HOME=")
		}
	}
	for _, name := range []string{"config.yaml", "auth.json", ".env"} {
		if _, err := os.Stat(filepath.Join(hermesHome, name)); !os.IsNotExist(err) {
			t.Fatalf("%s should not be copied from cwd when global home is unavailable, err=%v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(hermesHome, "skills")); !os.IsNotExist(err) {
		t.Fatalf("cwd skills should not be copied into hermes home, err=%v", err)
	}
}

func TestMergeHermesExternalDirsPreservesUserPrecedenceAndDedupes(t *testing.T) {
	got := mergeHermesExternalDirs(`model: test
skills:
  enabled: true
  external_dirs:
    - "/user/first"
other: value
`, []string{"/tutti/root", "/user/first", "/home/.hermes/skills", "/tutti/root"})
	want := `model: test
skills:
  enabled: true
  external_dirs:
    - "/user/first"
    - "/tutti/root"
    - "/home/.hermes/skills"
other: value
`
	if got != want {
		t.Fatalf("mergeHermesExternalDirs() mismatch:\nwant:\n%s\ngot:\n%s", want, got)
	}
}
