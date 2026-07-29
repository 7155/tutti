import {
  chooseApproval,
  expandTurnWorkSections,
  submitPrompt,
  waitForFinalToken,
  waitForSubAgentCard,
  waitForWorking
} from "./cdp-helpers.mjs";
import {
  codexProviderNativeSubagentComposerDefaults,
  defineRecordScenario
} from "./shared.mjs";

const forbiddenPath = "/tmp/tutti-i09-child-approval-reject-must-not-exist";
export default defineRecordScenario({
  id: "i09",
  async prepare({ removePath, setComposerDefaults }) {
    await setComposerDefaults(codexProviderNativeSubagentComposerDefaults);
    await removePath(forbiddenPath);
  },
  async drive({ client, timeoutMs }) {
    await submitPrompt(
      client,
      "Use the provider-native Agent tool to spawn exactly one subagent inside this turn. Do not create or control a separate Tutti Agent session: do not run tutti, tutti-dev, or any agent start, send, wait, or respond command. The root agent must not run the requested shell command. Delegate only this task to the provider-native subagent: use the shell to run exactly touch /tmp/tutti-i09-child-approval-reject-must-not-exist. The subagent must request approval before execution. If that request is rejected, the subagent must not use an alternative and must report the rejection. After the provider-native subagent settles, reply exactly I09_CHILD_APPROVAL_REJECTED and no other text.",
      timeoutMs
    );
    await waitForWorking(client, timeoutMs, "i09");
    await waitForSubAgentCard(
      client,
      timeoutMs,
      "running",
      "I09 running provider-native subagent card"
    );
    await chooseApproval(client, "deny", timeoutMs);
  },
  async assert({ assertPathAbsent, client, phase, timeoutMs }) {
    if (phase !== "terminal") return null;
    const settled = await waitForFinalToken(
      client,
      "I09_CHILD_APPROVAL_REJECTED",
      timeoutMs
    );
    await expandTurnWorkSections(
      client,
      timeoutMs,
      "I09 expanded terminal work section"
    );
    await waitForSubAgentCard(
      client,
      timeoutMs,
      "completed",
      "I09 completed provider-native subagent card"
    );
    await assertPathAbsent(forbiddenPath);
    return settled;
  }
});
