import { waitForEvaluation } from "../agent-gui-performance-helpers.mjs";
import {
  startProjectSession,
  submitPrompt,
  waitForFinalToken,
  waitForWorking
} from "./cdp-helpers.mjs";
import { codexComposerDefaults, defineRecordScenario } from "./shared.mjs";

const project = { label: "Replay Project", relativePath: "." };
const firstPrompt =
  "Reply with exactly P03_FIRST_TURN_DONE and no other text. Use no tools.";
const firstToken = "P03_FIRST_TURN_DONE";
const followUpPrompt =
  "This is a follow-up in the same project session. Use the shell to report the current working directory, then reply with exactly P03_CONTINUE_PROJECT_DONE and no other text.";
const finalToken = "P03_CONTINUE_PROJECT_DONE";

export default defineRecordScenario({
  id: "p03",
  project,
  async prepare({ selectProject, setComposerDefaults }) {
    await setComposerDefaults(codexComposerDefaults);
    return { project: await selectProject(project) };
  },
  async drive({ client, scenarioState, timeoutMs }) {
    await startProjectSession(client, scenarioState.project.id, timeoutMs);
    // Turn 1: create a completed turn inside the project session.
    await submitPrompt(client, firstPrompt, timeoutMs);
    await waitForWorking(client, timeoutMs, "p03 initial turn");
    await waitForFinalToken(client, firstToken, timeoutMs);
    // Turn 2: continue the same project session; the shell pwd exercises the
    // preserved project cwd.
    await submitPrompt(client, followUpPrompt, timeoutMs);
    await waitForWorking(client, timeoutMs, "p03 continue turn");
  },
  async assert({ client, phase, timeoutMs, verifyProjectBinding }) {
    if (phase === "recorded") {
      await verifyProjectBinding();
      return null;
    }
    if (phase !== "terminal") {
      return null;
    }
    // Terminal state: both turn tokens visible in the same project session,
    // the follow-up appended after the first turn, and no active turn left.
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
      "P03 continue-project-session terminal state",
      50
    );
  }
});
