import {
  submitGoalPrompt,
  waitForGoalCompletedToken,
  waitForWorking
} from "./cdp-helpers.mjs";
import { codexGoalComposerDefaults, defineRecordScenario } from "./shared.mjs";

export default defineRecordScenario({
  id: "l01",
  async prepare({ setComposerDefaults }) {
    await setComposerDefaults(codexGoalComposerDefaults);
  },
  async drive({ client, timeoutMs }) {
    await submitGoalPrompt(
      client,
      "从 1 数到 3，每次数 1 个。数完后标记目标完成，并在最后一条回复中包含 L01_GOAL_COMPLETED。不要使用其他工具。",
      timeoutMs
    );
    await waitForWorking(client, timeoutMs, "l01");
  },
  async assert({ client, phase, timeoutMs }) {
    return phase === "terminal"
      ? waitForGoalCompletedToken(client, "L01_GOAL_COMPLETED", timeoutMs)
      : null;
  }
});
