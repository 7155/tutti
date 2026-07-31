import { waitForEvaluation } from "../agent-gui-performance-helpers.mjs";
import {
  submitGuidancePrompt,
  submitPrompt,
  waitForWorking
} from "./cdp-helpers.mjs";
import { codexComposerDefaults, defineRecordScenario } from "./shared.mjs";

const longTaskPrompt =
  "Do not use any tools. Repeat the exact line 'C06 filler line to keep this turn busy.' 250 times, one per line. If a new user message arrives while you are working, stop the filler immediately and follow the newest message instead.";
const guidancePrompt =
  "C06-STEER: stop the filler and reply with exactly C06_STEER_DONE and no other text. Use no tools.";
const finalToken = "C06_STEER_DONE";

export default defineRecordScenario({
  id: "c06",
  async prepare({ setComposerDefaults }) {
    await setComposerDefaults(codexComposerDefaults);
  },
  async drive({ client, timeoutMs }) {
    await submitPrompt(client, longTaskPrompt, timeoutMs);
    await waitForWorking(client, timeoutMs, "c06 initial turn");
    // Cmd+Enter is the composer-native guidance path. It emits a send_now
    // submit directly and does not create a visible queued-prompt row.
    await submitGuidancePrompt(client, guidancePrompt, timeoutMs);
  },
  async assert({ client, phase, timeoutMs }) {
    if (phase !== "terminal") {
      return null;
    }
    return waitForEvaluation(
      client,
      `(() => {
        const detail = document.querySelector('main[data-agent-session-id]');
        const markdown = [...(detail?.querySelectorAll('[data-workspace-agent-markdown="true"]') ?? [])];
        const finalReply = markdown.find((element) =>
          (element.textContent?.trim() ?? '').endsWith(${JSON.stringify(finalToken)})
        );
        const queuedRows = [...document.querySelectorAll(
          '[data-testid^="agent-gui-composer-queued-prompt-"][data-draining]'
        )];
        return {
          ready:
            Boolean(detail) &&
            Boolean(finalReply) &&
            queuedRows.length === 0 &&
            !document.querySelector('[data-testid="agent-gui-composer-stop-active-turn"]'),
          assistantText: finalReply ? ${JSON.stringify(finalToken)} : '',
          activeSessionId: detail?.getAttribute('data-agent-session-id') ?? null
        };
      })()`,
      timeoutMs,
      "C06 native steer terminal state",
      50
    );
  }
});
