import {
  sendApprovalFeedback,
  submitPrompt,
  waitForFinalToken,
  waitForWorking
} from "./cdp-helpers.mjs";
import { codexComposerDefaults, defineRecordScenario } from "./shared.mjs";

const forbiddenPath = "/tmp/tutti-i03-approval-feedback-must-not-exist";
const feedback =
  "Do not run the command. Your entire response must be exactly I03_APPROVAL_FEEDBACK_HANDLED with no punctuation.";

export default defineRecordScenario({
  id: "i03",
  async prepare({ removePath, setComposerDefaults }) {
    await setComposerDefaults(codexComposerDefaults);
    await removePath(forbiddenPath);
  },
  async drive({ client, timeoutMs }) {
    await submitPrompt(
      client,
      "Use the shell to run exactly this command: touch /tmp/tutti-i03-approval-feedback-must-not-exist. It must request approval before execution. If denied with feedback, follow that feedback and do not use an alternative.",
      timeoutMs
    );
    await waitForWorking(client, timeoutMs, "i03");
    await sendApprovalFeedback(client, feedback, timeoutMs);
  },
  async assert({ assertPathAbsent, client, phase, timeoutMs }) {
    if (phase !== "terminal") return null;
    const settled = await waitForFinalToken(
      client,
      "I03_APPROVAL_FEEDBACK_HANDLED",
      timeoutMs
    );
    await assertPathAbsent(forbiddenPath);
    return settled;
  }
});
