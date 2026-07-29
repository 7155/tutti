import {
  submitPrompt,
  waitForFinalToken,
  waitForWorking
} from "./cdp-helpers.mjs";
import { waitForEvaluation } from "../agent-gui-performance-helpers.mjs";
import { codexComposerDefaults, defineRecordScenario } from "./shared.mjs";

const composerDefaults = {
  ...codexComposerDefaults,
  permissionModeId: "full-access"
};

async function assertToolCardVisible(client, timeoutMs, label) {
  await waitForEvaluation(
    client,
    `(() => {
      const toggles = [
        ...document.querySelectorAll('[data-agent-turn-work-header] button[aria-expanded]'),
        ...document.querySelectorAll('button.workspace-agents-status-panel__detail-tool-count[aria-expanded]')
      ];
      for (const toggle of toggles) {
        if (toggle.getAttribute('aria-expanded') === 'false') toggle.click();
      }
      const rows = [...document.querySelectorAll('main[data-agent-session-id] .workspace-agents-status-panel__detail-tool-row')];
      const composerShell = document.querySelector('[data-testid="agent-gui-composer-input-shell"]');
      return {
        ready:
          rows.length > 0 &&
          composerShell instanceof HTMLElement &&
          composerShell.dataset.inputDisabled !== 'true',
        toolRowCount: rows.length
      };
    })()`,
    timeoutMs,
    label,
    100
  );
}

export default defineRecordScenario({
  id: "r03",
  async prepare({ setComposerDefaults }) {
    await setComposerDefaults(composerDefaults);
  },
  async drive({ client, timeoutMs }) {
    await submitPrompt(
      client,
      "Run exactly one shell command that prints R03_TOOL_OUTPUT and do not run any other tools. Do not narrate or explain anything. After the tool call completes, reply exactly R03_TOOL_ONLY_DONE and no other text.",
      timeoutMs
    );
    await waitForWorking(client, timeoutMs, "r03");
  },
  async assert({ client, phase, timeoutMs }) {
    if (phase !== "terminal") return null;
    const settled = await waitForFinalToken(
      client,
      "R03_TOOL_ONLY_DONE",
      timeoutMs
    );
    await assertToolCardVisible(
      client,
      timeoutMs,
      "r03 tool card with recovered composer"
    );
    return settled;
  }
});
