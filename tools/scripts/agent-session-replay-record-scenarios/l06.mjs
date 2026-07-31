import {
  submitPrompt,
  waitForFinalToken,
  waitForWorking
} from "./cdp-helpers.mjs";
import { codexComposerDefaults, defineRecordScenario } from "./shared.mjs";

export default defineRecordScenario({
  id: "l06",
  async prepare({ setComposerDefaults }) {
    await setComposerDefaults(codexComposerDefaults);
  },
  async drive({ client, timeoutMs }) {
    await submitPrompt(
      client,
      "Use spawn_agent exactly once to create one child Agent. Ask it to use no tools and reply exactly L06_CHILD_DONE. Wait for that child to finish, then reply exactly L06_CHILD_AGENT_SETTLED and no other text.",
      timeoutMs
    );
    await waitForWorking(client, timeoutMs, "l06");
  },
  async assert({ client, phase, timeoutMs }) {
    return phase === "terminal"
      ? waitForFinalToken(client, "L06_CHILD_AGENT_SETTLED", timeoutMs)
      : null;
  }
});
