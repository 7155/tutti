import {
  chooseApproval,
  submitPrompt,
  waitForFinalToken,
  waitForWorking
} from "./cdp-helpers.mjs";
import { codexComposerDefaults, defineRecordScenario } from "./shared.mjs";

const forbiddenPath = "/tmp/tutti-i02-approval-reject-must-not-exist";

export default defineRecordScenario({
  id: "i02",
  async prepare({ removePath, setComposerDefaults }) {
    await setComposerDefaults(codexComposerDefaults);
    await removePath(forbiddenPath);
  },
  async drive({ client, timeoutMs }) {
    await submitPrompt(
      client,
      "Use the shell to run exactly this command: touch /tmp/tutti-i02-approval-reject-must-not-exist. It must request approval before execution. If the user rejects it, do not use an alternative and reply exactly I02_APPROVAL_REJECTED.",
      timeoutMs
    );
    await waitForWorking(client, timeoutMs, "i02");
    await chooseApproval(client, "deny", timeoutMs);
  },
  async assert({ assertPathAbsent, client, phase, timeoutMs }) {
    if (phase !== "terminal") return null;
    const settled = await waitForFinalToken(
      client,
      "I02_APPROVAL_REJECTED",
      timeoutMs
    );
    await assertPathAbsent(forbiddenPath);
    return settled;
  }
});
