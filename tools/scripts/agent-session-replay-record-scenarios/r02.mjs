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

async function expandTurnWorkSections(client, timeoutMs, label) {
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
      const row = document.querySelector('main[data-agent-session-id] .workspace-agents-status-panel__detail-tool-row');
      return { ready: Boolean(row), toggleCount: toggles.length };
    })()`,
    timeoutMs,
    label,
    100
  );
}

async function expandToolCardAndVerifyCommand(client, timeoutMs, label) {
  await waitForEvaluation(
    client,
    `(() => {
      const heads = [...document.querySelectorAll('main[data-agent-session-id] button.workspace-agents-status-panel__detail-tool-row-head--button')];
      const head = heads[0] ?? null;
      if (head && head.getAttribute('aria-expanded') === 'false') head.click();
      const command = document.querySelector('[data-agent-terminal-command="true"]');
      const commandText = command?.textContent?.trim() ?? '';
      const terminal = command?.closest('.workspace-agents-status-panel__detail-tool-terminal');
      const output = terminal?.querySelector('pre code')?.textContent ?? '';
      return {
        ready:
          Boolean(head) &&
          head.getAttribute('aria-expanded') === 'true' &&
          commandText.length > 0 &&
          output.includes('R02_LINE_1') &&
          output.includes('R02_LINE_40'),
        commandText: commandText.slice(0, 160),
        outputLength: output.length,
        headCount: heads.length
      };
    })()`,
    timeoutMs,
    label,
    100
  );
}

export default defineRecordScenario({
  id: "r02",
  async prepare({ setComposerDefaults }) {
    await setComposerDefaults(composerDefaults);
  },
  async drive({ client, timeoutMs }) {
    await submitPrompt(
      client,
      "Run exactly one shell command that prints 40 lines, where line number n is exactly R02_LINE_n for n from 1 to 40 (for example the first line is R02_LINE_1 and the last line is R02_LINE_40). Use a single shell tool call and do not run any other tools. After the command completes, reply exactly R02_LONG_COMMAND_DONE and no other text.",
      timeoutMs
    );
    await waitForWorking(client, timeoutMs, "r02");
  },
  async assert({ client, phase, timeoutMs }) {
    if (phase !== "terminal") return null;
    const settled = await waitForFinalToken(
      client,
      "R02_LONG_COMMAND_DONE",
      timeoutMs
    );
    await expandTurnWorkSections(client, timeoutMs, "r02 tool row visible");
    await expandToolCardAndVerifyCommand(
      client,
      timeoutMs,
      "r02 expanded long command details"
    );
    return settled;
  }
});
