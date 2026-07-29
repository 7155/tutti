import {
  confirmPlan,
  enablePlanMode,
  submitPrompt,
  waitForFinalToken,
  waitForWorking
} from "./cdp-helpers.mjs";
import { codexComposerDefaults, defineRecordScenario } from "./shared.mjs";

export default defineRecordScenario({
  id: "i06",
  async prepare({ setComposerDefaults }) {
    await setComposerDefaults(codexComposerDefaults);
  },
  async drive({ client, timeoutMs }) {
    await enablePlanMode(client, timeoutMs);
    await submitPrompt(
      client,
      "Create a short implementation plan for replying with exactly I06_PLAN_CONFIRMED after the user confirms implementation. Do not execute while planning. After implementation confirmation, use no tools and reply exactly I06_PLAN_CONFIRMED.",
      timeoutMs
    );
    await waitForWorking(client, timeoutMs, "i06");
    await confirmPlan(client, timeoutMs);
  },
  async assert({ client, phase, timeoutMs }) {
    return phase === "terminal"
      ? waitForFinalToken(client, "I06_PLAN_CONFIRMED", timeoutMs)
      : null;
  }
});
