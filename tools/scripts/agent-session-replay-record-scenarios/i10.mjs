import {
  expandTurnWorkSections,
  sendApprovalFeedback,
  submitPrompt,
  waitForFinalToken,
  waitForSubAgentCard,
  waitForWorking
} from "./cdp-helpers.mjs";
import {
  codexProviderNativeSubagentComposerDefaults,
  defineRecordScenario
} from "./shared.mjs";

const forbiddenPath = "/tmp/tutti-i10-child-approval-feedback-must-not-exist";
const feedback =
  "Do not run the command and do not use any alternative. Report back that you received this feedback and stopped.";

export default defineRecordScenario({
  id: "i10",
  async prepare({ removePath, setComposerDefaults }) {
    await setComposerDefaults(codexProviderNativeSubagentComposerDefaults);
    await removePath(forbiddenPath);
  },
  async drive({ client, timeoutMs }) {
    await submitPrompt(
      client,
      "Use the provider-native Agent tool to spawn exactly one subagent inside this turn. Do not create or control a separate Tutti Agent session: do not run tutti, tutti-dev, or any agent start, send, wait, or respond command. The root agent must not run the requested shell command. Delegate only this task to the provider-native subagent: use the shell to run exactly touch /tmp/tutti-i10-child-approval-feedback-must-not-exist. The subagent must first request user approval with escalated permissions for that exact command and must not attempt to run it inside the sandbox before approval. If the approval is denied with feedback, the subagent must follow that feedback, must not use an alternative, and must report the feedback outcome. After the provider-native subagent settles, reply exactly I10_CHILD_APPROVAL_FEEDBACK_DONE and no other text.",
      timeoutMs
    );
    await waitForWorking(client, timeoutMs, "i10");
    await waitForSubAgentCard(
      client,
      timeoutMs,
      "running",
      "I10 running provider-native subagent card"
    );
    await sendApprovalFeedback(client, feedback, timeoutMs);
  },
  async assert({ assertPathAbsent, client, phase, timeoutMs }) {
    if (phase !== "terminal") return null;
    const settled = await waitForFinalToken(
      client,
      "I10_CHILD_APPROVAL_FEEDBACK_DONE",
      timeoutMs
    );
    await expandTurnWorkSections(
      client,
      timeoutMs,
      "I10 expanded terminal work section"
    );
    await waitForSubAgentCard(
      client,
      timeoutMs,
      "completed",
      "I10 completed provider-native subagent card"
    );
    await assertPathAbsent(forbiddenPath);
    return settled;
  }
});
