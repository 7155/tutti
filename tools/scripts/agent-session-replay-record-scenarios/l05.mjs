import {
  clickTestId,
  submitPrompt,
  triggerCompaction,
  waitForCompactionStatus,
  waitForFinalToken,
  waitForIdleComposer,
  waitForTestId,
  waitForWorking
} from "./cdp-helpers.mjs";
import { codexComposerDefaults, defineRecordScenario } from "./shared.mjs";

// Small windows for the mid-compaction cancel race: compaction on a short
// session can finish in a few seconds, so the cancel is best-effort. When the
// race is lost the scenario degrades to compact->compact (both completed) and
// reports it, per the L05 fallback policy.
const CANCEL_RACE_TIMEOUT_MS = 20000;
const CANCEL_SETTLE_TIMEOUT_MS = 60000;

export default defineRecordScenario({
  id: "l05",
  async prepare({ setComposerDefaults }) {
    await setComposerDefaults(codexComposerDefaults);
  },
  async drive({ client, timeoutMs }) {
    await submitPrompt(
      client,
      "Reply with exactly L05_TURN_1_COMPLETED and no other text. Use no tools.",
      timeoutMs
    );
    await waitForWorking(client, timeoutMs, "l05 turn 1");
    await waitForFinalToken(client, "L05_TURN_1_COMPLETED", timeoutMs);

    await triggerCompaction(client, timeoutMs, "l05 first compaction");
    let canceled = false;
    try {
      await waitForCompactionStatus(
        client,
        "running",
        CANCEL_RACE_TIMEOUT_MS,
        "l05 first compaction running divider"
      );
      await waitForTestId(
        client,
        "agent-gui-composer-stop-active-turn",
        CANCEL_RACE_TIMEOUT_MS,
        "l05 compaction stop button"
      );
      await clickTestId(client, "agent-gui-composer-stop-active-turn");
      await waitForCompactionStatus(
        client,
        "interrupted",
        CANCEL_SETTLE_TIMEOUT_MS,
        "l05 compaction interrupted divider"
      );
      canceled = true;
    } catch (error) {
      console.warn(
        `l05: best-effort mid-compaction cancel failed (${error.message}); ` +
          "degrading to compact->compact; scenario needs fixture support for a deterministic cancel"
      );
    }
    if (!canceled) {
      // Lost the race: let the first compaction reach its terminal state
      // before retrying so the retry palette command is available.
      await waitForCompactionStatus(
        client,
        "completed",
        timeoutMs,
        "l05 first compaction completed divider"
      );
    }
    await waitForIdleComposer(client, timeoutMs, "l05 pre-retry");

    await triggerCompaction(client, timeoutMs, "l05 retry compaction");
    await waitForCompactionStatus(
      client,
      "completed",
      timeoutMs,
      "l05 retry compaction completed divider",
      canceled ? 1 : 2
    );
    await waitForIdleComposer(client, timeoutMs, "l05 post-retry");

    await submitPrompt(
      client,
      "Reply with exactly L05_COMPACTION_RETRY_DONE and no other text. Use no tools.",
      timeoutMs
    );
    await waitForWorking(client, timeoutMs, "l05 post-retry turn");
  },
  async assert({ client, phase, timeoutMs }) {
    if (phase !== "terminal") {
      return null;
    }
    // Whether the first compaction was actually canceled is decided by a
    // record-time race, so the deterministic assertion only requires a
    // completed compaction marker plus the final token.
    await waitForCompactionStatus(
      client,
      "completed",
      timeoutMs,
      "l05 compaction completed marker"
    );
    return waitForFinalToken(client, "L05_COMPACTION_RETRY_DONE", timeoutMs);
  }
});
