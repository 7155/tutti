import {
  submitPrompt,
  waitForFinalToken,
  waitForWorking
} from "./cdp-helpers.mjs";
import { codexComposerDefaults, defineRecordScenario } from "./shared.mjs";

const composerDefaults = {
  ...codexComposerDefaults,
  permissionModeId: "full-access"
};

export default defineRecordScenario({
  id: "r01",
  async prepare({ setComposerDefaults }) {
    await setComposerDefaults(composerDefaults);
  },
  async drive({ client, timeoutMs }) {
    await submitPrompt(
      client,
      "First use a shell command to read tools/fixtures/agent-session-replay/r01-file-lifecycle.txt. Then use a second shell command to replace its content with exactly R01_UPDATED followed by a newline. After both tool calls complete, reply exactly R01_TOOL_FILE_LIFECYCLE_COMPLETE.",
      timeoutMs
    );
    await waitForWorking(client, timeoutMs, "r01");
  },
  async assert({ client, phase, timeoutMs }) {
    return phase === "terminal"
      ? waitForFinalToken(client, "R01_TOOL_FILE_LIFECYCLE_COMPLETE", timeoutMs)
      : null;
  }
});
