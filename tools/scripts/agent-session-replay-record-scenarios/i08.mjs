import { access } from "node:fs/promises";
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

const markerPath = "/tmp/tutti-i08-child-approval-accept-marker";

export default defineRecordScenario({
  id: "i08",
  async prepare({ removePath, setComposerDefaults }) {
    await setComposerDefaults(codexProviderNativeSubagentComposerDefaults);
    await removePath(markerPath);
  },
  async drive({ client, timeoutMs }) {
    await submitPrompt(
      client,
      "Use the provider-native Agent tool to spawn exactly one subagent inside this turn. Do not create or control a separate Tutti Agent session: do not run tutti, tutti-dev, or any agent start, send, wait, or respond command. The root agent must not run the requested shell command. Delegate only this task to the provider-native subagent: use the shell to run exactly touch /tmp/tutti-i08-child-approval-accept-marker. The subagent must request approval before execution and run the command exactly once after approval is granted. After the provider-native subagent settles, reply exactly I08_CHILD_APPROVAL_ACCEPTED and no other text.",
      timeoutMs
    );
    await waitForWorking(client, timeoutMs, "i08");
    await waitForSubAgentCard(
      client,
      timeoutMs,
      "running",
      "I08 running provider-native subagent card"
    );
    await chooseApproval(client, "approve", timeoutMs);
  },
  async assert({ client, phase, timeoutMs }) {
    if (phase !== "terminal") return null;
    const settled = await waitForFinalToken(
      client,
      "I08_CHILD_APPROVAL_ACCEPTED",
      timeoutMs
    );
    await expandTurnWorkSections(
      client,
      timeoutMs,
      "I08 expanded terminal work section"
    );
    await waitForSubAgentCard(
      client,
      timeoutMs,
      "completed",
      "I08 completed provider-native subagent card"
    );
    await access(markerPath).catch(() => {
      throw new Error(
        `record scenario i08 expected approved child command to create ${markerPath}`
      );
    });
    return settled;
  }
});
