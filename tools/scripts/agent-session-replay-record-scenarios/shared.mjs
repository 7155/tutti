export const codexComposerDefaults = Object.freeze({
  model: "gpt-5.3-codex-spark",
  permissionModeId: "read-only",
  reasoningEffort: "medium",
  speed: "standard"
});

export const codexImageComposerDefaults = Object.freeze({
  ...codexComposerDefaults,
  model: "gpt-5.5",
  reasoningEffort: "medium"
});

export const codexGoalComposerDefaults = Object.freeze({
  ...codexComposerDefaults,
  model: "gpt-5.5",
  reasoningEffort: "medium"
});

export const codexProviderNativeSubagentComposerDefaults = Object.freeze({
  ...codexComposerDefaults,
  model: "gpt-5.5",
  reasoningEffort: "high"
});

export function defineRecordScenario(definition) {
  const requiredFunctions = ["prepare", "drive", "assert"];
  for (const key of ["id", ...requiredFunctions]) {
    if (!definition?.[key]) {
      throw new Error(`record scenario is missing ${key}`);
    }
  }
  for (const key of requiredFunctions) {
    if (typeof definition[key] !== "function") {
      throw new Error(`record scenario ${definition.id} has invalid ${key}`);
    }
  }
  if (
    definition.setupInitialState !== undefined &&
    typeof definition.setupInitialState !== "function"
  ) {
    throw new Error(
      `record scenario ${definition.id} has invalid setupInitialState`
    );
  }
  const provider =
    typeof definition.provider === "string" && definition.provider.trim()
      ? definition.provider.trim()
      : "codex";
  if (!/^[a-z][a-z0-9-]*$/u.test(provider)) {
    throw new Error(
      `record scenario ${definition.id} has invalid provider ${provider}`
    );
  }
  const cassetteName = `${definition.id}_${provider}`;
  if (
    definition.cassetteName !== undefined &&
    definition.cassetteName !== cassetteName
  ) {
    throw new Error(
      `record scenario ${definition.id} cassetteName must be ${cassetteName}`
    );
  }
  const expectedRecordingMode =
    definition.expectedRecordingMode ?? "create-session";
  if (
    expectedRecordingMode !== "create-session" &&
    expectedRecordingMode !== "continue-session"
  ) {
    throw new Error(
      `record scenario ${definition.id} has invalid expectedRecordingMode`
    );
  }
  return Object.freeze({
    ...definition,
    provider,
    cassetteName,
    expectedRecordingMode
  });
}
