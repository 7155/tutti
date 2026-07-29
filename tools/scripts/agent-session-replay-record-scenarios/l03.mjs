import { waitForEvaluation } from "../agent-gui-performance-helpers.mjs";
import {
  clickGoalBannerAction,
  submitGoalPrompt,
  waitForGoalBannerAction,
  waitForGoalCompletedToken,
  waitForGoalPaused,
  waitForGoalResumed,
  waitForWorking
} from "./cdp-helpers.mjs";
import { codexGoalComposerDefaults, defineRecordScenario } from "./shared.mjs";

async function waitForEarlyGoalProgress(client, timeoutMs) {
  // Pause on the first durable digit while a Goal turn is still live. Require
  // one-number-per-turn so the model cannot dump the whole range before pause.
  await waitForEvaluation(
    client,
    `(() => {
      const detail = document.querySelector('main[data-agent-session-id]');
      const banner = document.querySelector('[data-testid="agent-gui-goal-banner"]');
      const pause = document.querySelector(
        '[data-testid="agent-gui-goal-banner-pause"]'
      );
      const stop = document.querySelector(
        '[data-testid="agent-gui-composer-stop-active-turn"]'
      );
      const markdown = [
        ...(detail?.querySelectorAll('[data-workspace-agent-markdown="true"]') ??
          [])
      ];
      const assistantText = markdown
        .map((element) => element.textContent ?? '')
        .join('\\n');
      const numbers = [...assistantText.matchAll(/\\b(\\d{1,3})\\b/g)].map(
        (match) => Number(match[1])
      );
      const maxNumber = numbers.reduce((max, value) => Math.max(max, value), 0);
      const completedEarly =
        assistantText.includes('L03_GOAL_COMPLETED') || banner == null;
      return {
        ready:
          Boolean(detail) &&
          banner instanceof HTMLElement &&
          pause instanceof HTMLElement &&
          stop instanceof HTMLButtonElement &&
          maxNumber === 1 &&
          !completedEarly,
        maxNumber,
        completedEarly,
        hasStop: stop instanceof HTMLButtonElement,
        assistantText: assistantText.trim().slice(0, 80),
        goalBannerText: banner?.textContent?.trim().slice(0, 120) ?? ''
      };
    })()`,
    timeoutMs,
    "l03 early goal progress before pause",
    25
  );
}

export default defineRecordScenario({
  id: "l03",
  async prepare({ setComposerDefaults }) {
    await setComposerDefaults(codexGoalComposerDefaults);
  },
  async drive({ client, timeoutMs }) {
    await submitGoalPrompt(
      client,
      [
        "从 1 数到 8。",
        "每次回复只输出一个数字，不要一次输出多个数字。",
        "数完后标记目标完成，并在最后一条回复中包含 L03_GOAL_COMPLETED。",
        "不要重复已经输出过的数字。",
        "不要使用其他工具。"
      ].join(""),
      timeoutMs
    );
    await waitForWorking(client, timeoutMs, "l03");
    await waitForGoalBannerAction(
      client,
      "agent-gui-goal-banner-pause",
      timeoutMs,
      "l03 goal pause control"
    );
    await waitForEarlyGoalProgress(client, timeoutMs);
    await clickGoalBannerAction(client, "agent-gui-goal-banner-pause");
    await waitForGoalPaused(client, timeoutMs);
    await clickGoalBannerAction(client, "agent-gui-goal-banner-resume");
    await waitForGoalResumed(client, timeoutMs);
  },
  async assert({ client, phase, timeoutMs }) {
    return phase === "terminal"
      ? waitForGoalCompletedToken(client, "L03_GOAL_COMPLETED", timeoutMs)
      : null;
  }
});
