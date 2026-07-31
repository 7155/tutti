import {
  submitPrompt,
  waitForFinalToken,
  waitForWorking
} from "./cdp-helpers.mjs";
import { codexComposerDefaults, defineRecordScenario } from "./shared.mjs";

const prompts = [
  "Reply with exactly C01_TURN_1_COMPLETED and no other text.",
  "Reply with exactly C01_TURN_2_COMPLETED and no other text.",
  "Reply with exactly C01_TURN_3_COMPLETED and no other text."
];
const expectedTokens = [
  "C01_TURN_1_COMPLETED",
  "C01_TURN_2_COMPLETED",
  "C01_TURN_3_COMPLETED"
];

export default defineRecordScenario({
  id: "c01",
  async prepare({ setComposerDefaults }) {
    await setComposerDefaults(codexComposerDefaults);
  },
  async drive({ client, timeoutMs }) {
    for (const [index, prompt] of prompts.entries()) {
      await submitPrompt(client, prompt, timeoutMs);
      await waitForWorking(client, timeoutMs, `c01 turn ${index + 1}`);
      await waitForFinalToken(client, expectedTokens[index], timeoutMs);
    }
  },
  async assert({ client, phase, timeoutMs }) {
    if (phase === "terminal") {
      return waitForFinalToken(client, expectedTokens.at(-1), timeoutMs);
    }
    return null;
  }
});
