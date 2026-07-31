import { waitForEvaluation } from "../agent-gui-performance-helpers.mjs";
import {
  submitPrompt,
  waitForFinalToken,
  waitForWorking
} from "./cdp-helpers.mjs";
import { codexComposerDefaults, defineRecordScenario } from "./shared.mjs";

const firstPrompt =
  "Reply with exactly C05_FIRST_TURN_COMPLETED and no other text. Use no tools.";
const firstToken = "C05_FIRST_TURN_COMPLETED";
const secondPrompt =
  "Reply with exactly C05_SETTINGS_NEXT_TURN_DONE and no other text. Use no tools.";
const finalToken = "C05_SETTINGS_NEXT_TURN_DONE";
// The reasoning option applied between the turns. The menu renders localized
// labels only, so match both the English and Chinese label for "high".
const reasoningOptionLabels = ["High", "高"];

async function evaluate(client, expression) {
  const result = await client.send("Runtime.evaluate", {
    expression,
    awaitPromise: true,
    returnByValue: true
  });
  if (result.exceptionDetails) {
    throw new Error(
      result.exceptionDetails.exception?.description ??
        result.exceptionDetails.text
    );
  }
  return result.result?.value;
}

export default defineRecordScenario({
  id: "c05",
  async prepare({ setComposerDefaults }) {
    // Baseline settings: reasoningEffort medium. The scenario raises it to
    // high through the composer settings menu before the second turn.
    await setComposerDefaults(codexComposerDefaults);
  },
  async drive({ client, timeoutMs }) {
    // Turn 1 with the default settings.
    await submitPrompt(client, firstPrompt, timeoutMs);
    await waitForWorking(client, timeoutMs, "c05 initial turn");
    await waitForFinalToken(client, firstToken, timeoutMs);
    // Open the model/reasoning dropdown (Radix trigger opens on pointerdown).
    await evaluate(
      client,
      `(() => {
        const trigger = document.querySelector('[data-agent-model-reasoning-trigger="true"]');
        if (!(trigger instanceof HTMLButtonElement) || trigger.disabled) {
          throw new Error('model/reasoning dropdown trigger is unavailable');
        }
        trigger.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, button: 0, pointerId: 1 }));
        trigger.dispatchEvent(new PointerEvent('pointerup', { bubbles: true, button: 0, pointerId: 1 }));
        return true;
      })()`
    );
    await waitForEvaluation(
      client,
      `({ ready: document.querySelector('[data-agent-reasoning-submenu-trigger="true"]') instanceof HTMLElement })`,
      timeoutMs,
      "C05 reasoning submenu trigger",
      25
    );
    // Open the reasoning submenu (Radix sub trigger opens on click).
    await evaluate(
      client,
      `(() => {
        const subTrigger = document.querySelector('[data-agent-reasoning-submenu-trigger="true"]');
        if (!(subTrigger instanceof HTMLElement)) {
          throw new Error('reasoning submenu trigger is unavailable');
        }
        subTrigger.dispatchEvent(new PointerEvent('pointermove', { bubbles: true, pointerType: 'mouse' }));
        subTrigger.click();
        return true;
      })()`
    );
    const optionProbe = `(() => {
      const submenus = [...document.querySelectorAll('[data-agent-composer-settings-layout="model-submenu"]')];
      const items = submenus.flatMap((submenu) => [...submenu.querySelectorAll('[role="menuitem"]')]);
      const labels = items.map((item) => item.textContent?.trim() ?? '');
      const target = items.find((item) =>
        ${JSON.stringify(reasoningOptionLabels)}.includes(item.textContent?.trim() ?? '')
      );
      return { ready: Boolean(target), labels };
    })()`;
    await waitForEvaluation(
      client,
      optionProbe,
      timeoutMs,
      "C05 reasoning high option",
      25
    );
    // Apply the high reasoning option (menu items apply on pointerdown).
    await evaluate(
      client,
      `(() => {
        const submenus = [...document.querySelectorAll('[data-agent-composer-settings-layout="model-submenu"]')];
        const items = submenus.flatMap((submenu) => [...submenu.querySelectorAll('[role="menuitem"]')]);
        const target = items.find((item) =>
          ${JSON.stringify(reasoningOptionLabels)}.includes(item.textContent?.trim() ?? '')
        );
        if (!(target instanceof HTMLElement)) {
          throw new Error('reasoning high option is unavailable');
        }
        target.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, button: 0, pointerId: 1 }));
        target.dispatchEvent(new PointerEvent('pointerup', { bubbles: true, button: 0, pointerId: 1 }));
        return true;
      })()`
    );
    // The trigger echoes the new reasoning selection once the update lands.
    await waitForEvaluation(
      client,
      `(() => {
        const trigger = document.querySelector('[data-agent-model-reasoning-trigger="true"]');
        const text = trigger?.textContent ?? '';
        return {
          ready: ${JSON.stringify(reasoningOptionLabels)}.some((label) => text.includes(label)),
          triggerText: text.slice(0, 80)
        };
      })()`,
      timeoutMs,
      "C05 reasoning selection echoed in trigger",
      25
    );
    // Turn 2 runs with the updated settings.
    await submitPrompt(client, secondPrompt, timeoutMs);
    await waitForWorking(client, timeoutMs, "c05 settings turn");
  },
  async assert({ client, phase, timeoutMs }) {
    if (phase !== "terminal") {
      return null;
    }
    // Terminal: both turns finished in order, the new reasoning selection is
    // still echoed by the settings trigger, and no turn is active.
    return waitForEvaluation(
      client,
      `(() => {
        const detail = document.querySelector('main[data-agent-session-id]');
        const markdown = [...(detail?.querySelectorAll('[data-workspace-agent-markdown="true"]') ?? [])];
        const firstIndex = markdown.findIndex((element) =>
          element.textContent?.trim() === ${JSON.stringify(firstToken)}
        );
        const finalIndex = markdown.findIndex((element) =>
          element.textContent?.trim() === ${JSON.stringify(finalToken)}
        );
        const trigger = document.querySelector('[data-agent-model-reasoning-trigger="true"]');
        const triggerText = trigger?.textContent ?? '';
        const reasoningEchoed = ${JSON.stringify(reasoningOptionLabels)}.some((label) =>
          triggerText.includes(label)
        );
        return {
          ready: Boolean(detail) && firstIndex >= 0 && finalIndex > firstIndex &&
            reasoningEchoed &&
            !document.querySelector('[data-testid="agent-gui-composer-stop-active-turn"]'),
          assistantText: finalIndex >= 0 ? markdown[finalIndex].textContent?.trim() ?? '' : '',
          activeSessionId: detail?.getAttribute('data-agent-session-id') ?? null
        };
      })()`,
      timeoutMs,
      "C05 settings next-turn terminal state",
      50
    );
  }
});
