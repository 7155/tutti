import {
  submitPrompt,
  triggerCompaction,
  waitForCompactionStatus,
  waitForFinalToken,
  waitForIdleComposer,
  waitForWorking
} from "./cdp-helpers.mjs";
import { codexComposerDefaults, defineRecordScenario } from "./shared.mjs";

export default defineRecordScenario({
  id: "l04",
  async prepare({ setComposerDefaults }) {
    await setComposerDefaults(codexComposerDefaults);
  },
  async drive({ client, timeoutMs }) {
    // Accumulate compactable context with one completed turn first: the
    // `/compact` palette command requires hasCompactableContext and an idle
    // composer (compact is disabled mid-turn).
    await submitPrompt(
      client,
      "Reply with exactly L04_TURN_1_COMPLETED and no other text. Use no tools.",
      timeoutMs
    );
    await waitForWorking(client, timeoutMs, "l04 turn 1");
    await waitForFinalToken(client, "L04_TURN_1_COMPLETED", timeoutMs);
    await triggerCompaction(client, timeoutMs, "l04");
    await waitForCompactionStatus(
      client,
      "completed",
      timeoutMs,
      "l04 compaction completed divider"
    );
    await waitForIdleComposer(client, timeoutMs, "l04 post-compaction");
    // The session must remain usable after compaction.
    await submitPrompt(
      client,
      "Reply with exactly L04_COMPACTION_DONE and no other text. Use no tools.",
      timeoutMs
    );
    await waitForWorking(client, timeoutMs, "l04 post-compaction turn");
  },
  async assert({ client, phase, timeoutMs }) {
    if (phase !== "terminal") {
      return null;
    }
    await waitForCompactionStatus(
      client,
      "completed",
      timeoutMs,
      "l04 compaction completed marker"
    );
    return waitForFinalToken(client, "L04_COMPACTION_DONE", timeoutMs);
  }
});
