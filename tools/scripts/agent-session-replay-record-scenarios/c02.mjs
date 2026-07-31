import { waitForEvaluation } from "../agent-gui-performance-helpers.mjs";
import {
  submitPrompt,
  waitForFinalToken,
  waitForWorking
} from "./cdp-helpers.mjs";
import { codexComposerDefaults, defineRecordScenario } from "./shared.mjs";

const firstPrompt =
  "Reply with exactly C02_FIRST_TURN_COMPLETED and no other text. Use no tools.";
const firstToken = "C02_FIRST_TURN_COMPLETED";
const followUpPrompt =
  "This is a follow-up question in the same conversation. Reply with exactly C02_CONTINUE_SESSION_DONE and no other text. Use no tools.";
const finalToken = "C02_CONTINUE_SESSION_DONE";

export default defineRecordScenario({
  id: "c02",
  expectedRecordingMode: "continue-session",
  async prepare({ setComposerDefaults }) {
    await setComposerDefaults(codexComposerDefaults);
  },
  async setupInitialState({ client, timeoutMs }) {
    await submitPrompt(client, firstPrompt, timeoutMs);
    await waitForWorking(client, timeoutMs, "c02 initial turn");
    await waitForFinalToken(client, firstToken, timeoutMs);
  },
  async drive({ client, timeoutMs }) {
    await submitPrompt(client, followUpPrompt, timeoutMs);
    await waitForWorking(client, timeoutMs, "c02 continue turn");
  },
  async assert({ client, phase, timeoutMs }) {
    if (phase !== "terminal") {
      return null;
    }
    // Terminal state: both assistant replies visible in the same session,
    // the follow-up appended after the original, and no active turn left.
    return waitForEvaluation(
      client,
      `(() => {
        const detail = document.querySelector('main[data-agent-session-id]');
        const markdown = [...(detail?.querySelectorAll('[data-workspace-agent-markdown="true"]') ?? [])];
        const firstIndex = markdown.findIndex((element) =>
          element.textContent?.trim() === ${JSON.stringify(firstToken)}
        );
        const finalIndex = markdown.findIndex((element) =>
          element.textContent?.trim() === ${JSON.stringify(finalToken)}
        );
        return {
          ready: Boolean(detail) && firstIndex >= 0 && finalIndex > firstIndex &&
            !document.querySelector('[data-testid="agent-gui-composer-stop-active-turn"]'),
          assistantText: finalIndex >= 0 ? markdown[finalIndex].textContent?.trim() ?? '' : '',
          activeSessionId: detail?.getAttribute('data-agent-session-id') ?? null
        };
      })()`,
      timeoutMs,
      "C02 continue-session terminal state",
      50
    );
  }
});
