package runtimeprep

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// hermesHomeEnv 是 hermes-agent 的数据根环境变量。hermes 从 $HERMES_HOME/skills/
// 与 $HERMES_HOME/config.yaml 的 skills.external_dirs 发现 skill，并把 config.yaml
// （model/provider 接线）与 auth.json（凭证）锚定在 HERMES_HOME 下。
const hermesHomeEnv = "HERMES_HOME"

// hermesGlobalHomeFiles 是必须从用户全局 hermes home 复制进 per-session home 的
// 文件：缺 config.yaml 复现 "No LLM provider configured"，缺 auth.json 则无凭证，
// 缺 .env 则 provider 的 API key（如 OPENCODE_ZEN_API_KEY，hermes 从 HERMES_HOME/.env
// 加载）丢失，复现 "no API key was found"。复制为 opaque 字节，不解析、不日志，凭证不进
// manifest。SOUL.md（persona）与 state.db/sessions/memories（状态）不复制，per-session
// 保持干净；hermes 缺 SOUL.md 不报错，仅用默认 persona。
var hermesGlobalHomeFiles = []string{"config.yaml", "auth.json", ".env"}

// HermesPreparer 为 hermes agent extension（provider id "acp:hermes"）准备 per-session
// HERMES_HOME。hermes-agent 只从 $HERMES_HOME/skills/ 和 config.yaml 的
// skills.external_dirs 发现 skill，因此 Tutti 将 extension profile 声明的 workspace
// skill roots 写入 config overlay，让 hermes 读取已经物化的 workspace skills，同时
// 继续用 per-session HERMES_HOME 隔离 sessions/memories。AGENTS.md 仍写到 cwd，与其它
// instruction-file provider 一致。
type HermesPreparer struct{}

func (HermesPreparer) Provider() string {
	return "acp:hermes"
}

func (HermesPreparer) Prepare(_ context.Context, input ProviderPrepareInput) (ProviderPrepareResult, error) {
	agentsPath := filepath.Join(input.Cwd, "AGENTS.md")
	policy, err := tuttiCLIPolicy(input.PrepareInput)
	if err != nil {
		return ProviderPrepareResult{}, err
	}
	writeResult, err := input.Store.WriteManagedBlock(agentsPath, policy)
	if err != nil {
		return ProviderPrepareResult{}, err
	}
	if input.Manifest != nil {
		input.Manifest.RecordManagedFile(agentsPath, "provider-instructions", writeResult.Created)
	}

	hermesHome := filepath.Join(input.RuntimeRoot, "hermes")
	if err := os.MkdirAll(hermesHome, 0o700); err != nil {
		return ProviderPrepareResult{}, fmt.Errorf("create per-session hermes home: %w", err)
	}

	// 从用户全局 hermes home 复制 config/auth/env，使 hermes 能接上 LLM provider。
	// globalHome 为空时跳过复制，避免把 daemon cwd 下的相对文件误当成 ~/.hermes。
	globalHome := resolveGlobalHermesHome()
	var userConfig []byte
	if globalHome != "" {
		for _, name := range hermesGlobalHomeFiles {
			src := filepath.Join(globalHome, name)
			if name == "config.yaml" {
				var readErr error
				userConfig, readErr = os.ReadFile(src)
				if readErr != nil && !os.IsNotExist(readErr) {
					return ProviderPrepareResult{}, fmt.Errorf("read hermes %s: %w", name, readErr)
				}
				continue
			}
			if err := copyHermesHomeFile(src, filepath.Join(hermesHome, name)); err != nil {
				return ProviderPrepareResult{}, err
			}
		}
	}

	externalDirs := make([]string, 0, len(input.ExtensionSkillRoots)+1)
	skillPaths := []string{}
	for _, root := range input.ExtensionSkillRoots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		if !filepath.IsAbs(root) {
			root = filepath.Join(input.Cwd, root)
		}
		paths, err := installProviderNativeSkillsStable(root, input.PrepareInput)
		if err != nil {
			return ProviderPrepareResult{}, err
		}
		skillPaths = append(skillPaths, paths...)
		externalDirs = appendUniquePath(externalDirs, root)
	}
	if globalHome != "" {
		globalSkills := filepath.Join(globalHome, "skills")
		if info, err := os.Stat(globalSkills); err == nil && info.IsDir() {
			externalDirs = appendUniquePath(externalDirs, globalSkills)
		} else if err != nil && !os.IsNotExist(err) {
			return ProviderPrepareResult{}, fmt.Errorf("inspect hermes native skills: %w", err)
		}
	}
	if err := writeHermesSessionConfig(filepath.Join(hermesHome, "config.yaml"), userConfig, externalDirs); err != nil {
		return ProviderPrepareResult{}, err
	}

	if input.Manifest != nil {
		input.Manifest.RecordManagedFile(hermesHome, "provider-hermes-home", true)
		for _, skillPath := range skillPaths {
			input.Manifest.RecordManagedFile(skillPath, "provider-skill", true)
		}
	}

	return ProviderPrepareResult{
		Cwd: input.Cwd,
		Env: []string{hermesHomeEnv + "=" + hermesHome},
	}, nil
}

// resolveGlobalHermesHome 返回未被 tutti 覆盖时 hermes 会用的 home 目录。daemon 进程
// 通常未设 HERMES_HOME，故回退 ~/.hermes。
func resolveGlobalHermesHome() string {
	if v := strings.TrimSpace(os.Getenv(hermesHomeEnv)); v != "" {
		return v
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".hermes")
	}
	return ""
}

// copyHermesHomeFile 以 opaque 字节复制单个 hermes home 文件。源不存在时跳过
// （用户尚未 setup hermes，交由 hermes 自身报错，不由 tutti 掩盖）。
func copyHermesHomeFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read hermes %s: %w", filepath.Base(src), err)
	}
	return os.WriteFile(dst, data, 0o600)
}

// appendUniquePath preserves the first occurrence. Hermes config merging uses
// that as the precedence rule: existing user config roots stay first, then
// Tutti-managed extension workspace roots, then the user's native Hermes skill
// directory as a read-only external root.
func appendUniquePath(paths []string, path string) []string {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "." || path == "" {
		return paths
	}
	if slices.Contains(paths, path) {
		return paths
	}
	return append(paths, path)
}

func writeHermesSessionConfig(path string, userConfig []byte, externalDirs []string) error {
	config := mergeHermesExternalDirs(string(userConfig), externalDirs)
	if strings.TrimSpace(config) == "" {
		return nil
	}
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		return fmt.Errorf("write hermes config.yaml: %w", err)
	}
	return nil
}

func mergeHermesExternalDirs(config string, externalDirs []string) string {
	dirs := dedupeHermesExternalDirs(externalDirs)
	if len(dirs) == 0 {
		return config
	}
	lines := strings.Split(strings.ReplaceAll(config, "\r\n", "\n"), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	skillsStart, skillsEnd := topLevelYAMLBlock(lines, "skills")
	if skillsStart < 0 {
		lines = appendNonEmptyLine(lines, "skills:")
		lines = append(lines, "  external_dirs:")
		for _, dir := range dirs {
			lines = append(lines, "    - "+quoteYAMLString(dir))
		}
		return strings.Join(lines, "\n") + "\n"
	}
	externalStart, externalEnd := nestedYAMLBlock(lines, skillsStart+1, skillsEnd, 2, "external_dirs")
	if externalStart < 0 {
		insert := []string{"  external_dirs:"}
		for _, dir := range dirs {
			insert = append(insert, "    - "+quoteYAMLString(dir))
		}
		lines = append(lines[:skillsEnd], append(insert, lines[skillsEnd:]...)...)
		return strings.Join(lines, "\n") + "\n"
	}
	existing := hermesExternalDirsFromLines(lines[externalStart+1 : externalEnd])
	appendLines := []string{}
	for _, dir := range dirs {
		if slices.Contains(existing, dir) {
			continue
		}
		appendLines = append(appendLines, "    - "+quoteYAMLString(dir))
	}
	if len(appendLines) == 0 {
		return strings.Join(lines, "\n") + "\n"
	}
	lines = append(lines[:externalEnd], append(appendLines, lines[externalEnd:]...)...)
	return strings.Join(lines, "\n") + "\n"
}

func dedupeHermesExternalDirs(paths []string) []string {
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		result = appendUniquePath(result, path)
	}
	return result
}

func topLevelYAMLBlock(lines []string, key string) (int, int) {
	prefix := key + ":"
	for index, line := range lines {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if yamlLineIndent(line) == 0 && strings.HasPrefix(strings.TrimSpace(line), prefix) {
			end := len(lines)
			for next := index + 1; next < len(lines); next++ {
				trimmed := strings.TrimSpace(lines[next])
				if trimmed == "" || strings.HasPrefix(trimmed, "#") {
					continue
				}
				if yamlLineIndent(lines[next]) == 0 {
					end = next
					break
				}
			}
			return index, end
		}
	}
	return -1, -1
}

func nestedYAMLBlock(lines []string, start int, end int, indent int, key string) (int, int) {
	prefix := key + ":"
	for index := start; index < end; index++ {
		trimmed := strings.TrimSpace(lines[index])
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if yamlLineIndent(lines[index]) == indent && strings.HasPrefix(trimmed, prefix) {
			blockEnd := end
			for next := index + 1; next < end; next++ {
				nextTrimmed := strings.TrimSpace(lines[next])
				if nextTrimmed == "" || strings.HasPrefix(nextTrimmed, "#") {
					continue
				}
				if yamlLineIndent(lines[next]) <= indent {
					blockEnd = next
					break
				}
			}
			return index, blockEnd
		}
	}
	return -1, -1
}

func hermesExternalDirsFromLines(lines []string) []string {
	result := []string{}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "-") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
		value = strings.Trim(value, "\"'")
		result = appendUniquePath(result, value)
	}
	return result
}

func yamlLineIndent(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}

func appendNonEmptyLine(lines []string, line string) []string {
	if len(lines) == 0 {
		return []string{line}
	}
	if strings.TrimSpace(lines[len(lines)-1]) != "" {
		lines = append(lines, "")
	}
	return append(lines, line)
}

func quoteYAMLString(value string) string {
	return fmt.Sprintf("%q", value)
}
