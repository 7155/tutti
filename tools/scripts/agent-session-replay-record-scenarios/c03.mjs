import {
  cancelActiveTurn,
  submitPrompt,
  waitForFinalToken,
  waitForWorking
} from "./cdp-helpers.mjs";
import { codexComposerDefaults, defineRecordScenario } from "./shared.mjs";

const composerDefaults = {
  ...codexComposerDefaults,
  permissionModeId: "full-access"
};
const resendPrompt =
  "Reply with exactly C03_RESEND_COMPLETED and no other text. Use no tools.";

export default defineRecordScenario({
  id: "c03",
  async prepare({ setComposerDefaults }) {
    await setComposerDefaults(composerDefaults);
  },
  async drive({ client, timeoutMs }) {
    await submitPrompt(
      client,
      "Run exactly this shell command and do not send a final response until it ends: sleep 60.",
      timeoutMs
    );
    await waitForWorking(client, timeoutMs, "c03 initial turn");
    await cancelActiveTurn(client, timeoutMs, "c03");
    await submitPrompt(client, resendPrompt, timeoutMs);
    await waitForWorking(client, timeoutMs, "c03 resend turn");
  },
  async assert({ client, phase, timeoutMs }) {
    return phase === "terminal"
      ? waitForFinalToken(client, "C03_RESEND_COMPLETED", timeoutMs)
      : null;
  }
});
