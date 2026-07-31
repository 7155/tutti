import {
  chooseApproval,
  submitPrompt,
  waitForFinalToken,
  waitForWorking
} from "./cdp-helpers.mjs";
import { codexComposerDefaults, defineRecordScenario } from "./shared.mjs";

export default defineRecordScenario({
  id: "i01",
  async prepare({ setComposerDefaults }) {
    await setComposerDefaults(codexComposerDefaults);
  },
  async drive({ client, timeoutMs }) {
    await submitPrompt(
      client,
      "Use the shell to run exactly this command: printf 'tutti-i01-approval-accept\\n'. It must request approval before execution. After the command succeeds, reply exactly I01_APPROVAL_ACCEPTED.",
      timeoutMs
    );
    await waitForWorking(client, timeoutMs, "i01");
    await chooseApproval(client, "approve", timeoutMs);
  },
  async assert({ client, phase, timeoutMs }) {
    return phase === "terminal"
      ? waitForFinalToken(client, "I01_APPROVAL_ACCEPTED", timeoutMs)
      : null;
  }
});
