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
  id: "r07",
  async prepare({ setComposerDefaults }) {
    await setComposerDefaults(composerDefaults);
  },
  async drive({ client, timeoutMs }) {
    await submitPrompt(
      client,
      "Use separate shell commands to create .tmp/agent-session-replay-r07/first.txt with exactly R07_FIRST_UPDATED followed by a newline, then .tmp/agent-session-replay-r07/second.txt with exactly R07_SECOND_UPDATED followed by a newline. After both files are written, reply exactly R07_MULTI_FILE_CHANGE_COMPLETED and no other text.",
      timeoutMs
    );
    await waitForWorking(client, timeoutMs, "r07");
  },
  async assert({ client, phase, timeoutMs }) {
    return phase === "terminal"
      ? waitForFinalToken(client, "R07_MULTI_FILE_CHANGE_COMPLETED", timeoutMs)
      : null;
  }
});
