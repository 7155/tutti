import {
  clickGoalBannerAction,
  submitGoalPrompt,
  waitForGoalBannerAction,
  waitForGoalCleared,
  waitForWorking
} from "./cdp-helpers.mjs";
import { codexGoalComposerDefaults, defineRecordScenario } from "./shared.mjs";

export default defineRecordScenario({
  id: "l02",
  async prepare({ setComposerDefaults }) {
    await setComposerDefaults(codexGoalComposerDefaults);
  },
  async drive({ client, timeoutMs }) {
    // Long enough that clear lands while the Goal is still active.
    await submitGoalPrompt(
      client,
      "从 1 数到 30，每次数 1 个，数之间稍作停顿。不要使用其他工具。若目标被清除则立即停止。",
      timeoutMs
    );
    await waitForWorking(client, timeoutMs, "l02");
    await waitForGoalBannerAction(
      client,
      "agent-gui-goal-banner-clear",
      timeoutMs,
      "l02 goal clear control"
    );
    await clickGoalBannerAction(client, "agent-gui-goal-banner-clear");
  },
  async assert({ client, phase, timeoutMs }) {
    return phase === "terminal" ? waitForGoalCleared(client, timeoutMs) : null;
  }
});
