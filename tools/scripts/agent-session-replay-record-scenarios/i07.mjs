import {
  enablePlanMode,
  sendPlanFeedbackAndConfirm,
  submitPrompt,
  waitForFinalToken,
  waitForWorking
} from "./cdp-helpers.mjs";
import { codexComposerDefaults, defineRecordScenario } from "./shared.mjs";

const feedback = "Add a verification step named I07_FEEDBACK_APPLIED";

export default defineRecordScenario({
  id: "i07",
  async prepare({ setComposerDefaults }) {
    await setComposerDefaults(codexComposerDefaults);
  },
  async drive({ client, timeoutMs }) {
    await enablePlanMode(client, timeoutMs);
    await submitPrompt(
      client,
      "Create a short implementation plan for replying with exactly I07_PLAN_FEEDBACK_CONFIRMED after the user confirms implementation. Revise the plan if the user gives feedback. Do not execute while planning. After final implementation confirmation, use no tools and reply exactly I07_PLAN_FEEDBACK_CONFIRMED.",
      timeoutMs
    );
    await waitForWorking(client, timeoutMs, "i07");
    await sendPlanFeedbackAndConfirm(client, feedback, timeoutMs);
  },
  async assert({ client, phase, timeoutMs }) {
    return phase === "terminal"
      ? waitForFinalToken(client, "I07_PLAN_FEEDBACK_CONFIRMED", timeoutMs)
      : null;
  }
});
