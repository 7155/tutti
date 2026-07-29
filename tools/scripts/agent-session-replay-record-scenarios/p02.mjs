import { waitForEvaluation } from "../agent-gui-performance-helpers.mjs";
import {
  submitPrompt,
  waitForFinalToken,
  waitForWorking
} from "./cdp-helpers.mjs";
import { codexComposerDefaults, defineRecordScenario } from "./shared.mjs";

async function assertSessionHasNoProjectGrouping(client, timeoutMs) {
  await waitForEvaluation(
    client,
    `(() => {
      const detail = document.querySelector('main[data-agent-session-id]');
      const sessionId = detail?.getAttribute('data-agent-session-id')?.trim() ?? '';
      const item = sessionId
        ? document.querySelector('[data-testid="agent-gui-conversation-item-' + sessionId + '"]')
        : null;
      const section = item?.closest('[data-kind]');
      const sectionKind = section?.getAttribute('data-kind') ?? '';
      const projectSection = item?.closest('[data-testid^="agent-gui-project-section-"]');
      return {
        ready:
          Boolean(sessionId) &&
          Boolean(item) &&
          Boolean(sectionKind) &&
          sectionKind !== 'project' &&
          !projectSection,
        sectionKind: sectionKind || null,
        sessionId: sessionId || null
      };
    })()`,
    timeoutMs,
    "P02 session listed outside any project section",
    50
  );
}

export default defineRecordScenario({
  id: "p02",
  async prepare({ setComposerDefaults }) {
    await setComposerDefaults(codexComposerDefaults);
  },
  async drive({ client, timeoutMs }) {
    await submitPrompt(
      client,
      "Reply with exactly P02_NO_PROJECT_DONE and no other text.",
      timeoutMs
    );
    await waitForWorking(client, timeoutMs, "p02");
  },
  async assert({ client, phase, timeoutMs }) {
    if (phase !== "terminal") return null;
    const settled = await waitForFinalToken(
      client,
      "P02_NO_PROJECT_DONE",
      timeoutMs
    );
    await assertSessionHasNoProjectGrouping(client, timeoutMs);
    return settled;
  }
});
