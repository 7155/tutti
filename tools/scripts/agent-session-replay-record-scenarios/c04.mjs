import { setTimeout as delay } from "node:timers/promises";
import { waitForEvaluation } from "../agent-gui-performance-helpers.mjs";
import { submitPrompt, waitForWorking } from "./cdp-helpers.mjs";
import { codexComposerDefaults, defineRecordScenario } from "./shared.mjs";

// C04 is intentionally queue-only. Codex maps Send now to native guidance,
// which keeps the prompt in the active Turn and belongs to the separate C06
// steer scenario. Here the edited queue head waits for the active Turn to
// settle, then drains as a new Turn.
const composerDefaults = codexComposerDefaults;
const visibleQueueStateMs = 1_200;

const longTaskPrompt =
  "Do not use any tools. Repeat the exact line 'C04 filler line to keep this turn busy.' 250 times, one per line. If a new user message arrives while you are working, stop the filler immediately and follow the newest message instead.";
const firstReplyMarker = "C04 filler line to keep this turn busy.";
const queuedPromptAOriginal =
  "C04-QUEUED-A-ORIGINAL: reply with exactly C04_QUEUE_A_ORIGINAL_TOKEN and no other text. Use no tools.";
const queuedPromptAEdited =
  "C04-QUEUED-A-EDITED: reply with exactly C04_QUEUE_AUTO_DRAIN_DONE and no other text. Use no tools.";
const queuedPromptB =
  "C04-QUEUED-B: reply with exactly C04_QUEUE_B_TOKEN and no other text. Use no tools.";
const finalToken = "C04_QUEUE_AUTO_DRAIN_DONE";
const forbiddenBToken = "C04_QUEUE_B_TOKEN";

// Queued prompt rows carry data-testid="agent-gui-composer-queued-prompt-<id>"
// plus a data-draining attribute; the expand cue shares the prefix but has no
// data-draining attribute, so the selector below excludes it.
const queuedRowsExpression = `[...document.querySelectorAll('[data-testid^="agent-gui-composer-queued-prompt-"][data-draining]')]`;

function rowByMarkerExpression(marker) {
  return `${queuedRowsExpression}.find((row) => (row.textContent ?? '').includes(${JSON.stringify(marker)}))`;
}

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

async function waitForQueuedRowCount(client, expectedCount, timeoutMs, label) {
  await waitForEvaluation(
    client,
    `(() => {
      const rows = ${queuedRowsExpression};
      return {
        ready: rows.length === ${expectedCount},
        rowCount: rows.length,
        rowTexts: rows.map((row) => (row.textContent ?? '').slice(0, 60))
      };
    })()`,
    timeoutMs,
    label,
    25
  );
}

// The queued-prompt action buttons expose no test ids and localized
// aria-labels; address them by their stable JSX order inside the trailing
// actions container: [0] send now, [1] delete, [2] more actions (edit menu).
function actionButtonExpression(marker, buttonIndex) {
  return `(() => {
    const row = ${rowByMarkerExpression(marker)};
    if (!row) throw new Error('queued prompt row for ${marker} is unavailable');
    const actions = row.lastElementChild;
    const button = actions?.querySelectorAll('button')?.[${buttonIndex}];
    if (!(button instanceof HTMLButtonElement) || button.disabled) {
      throw new Error('queued prompt action ${buttonIndex} for ${marker} is unavailable');
    }
    return button;
  })()`;
}

async function clickQueuedRowAction(client, marker, buttonIndex) {
  await evaluate(
    client,
    `(() => {
      const button = ${actionButtonExpression(marker, buttonIndex)};
      button.click();
      return true;
    })()`
  );
}

export default defineRecordScenario({
  id: "c04",
  async prepare({ setComposerDefaults }) {
    await setComposerDefaults(composerDefaults);
  },
  async drive({ client, timeoutMs }) {
    // Start a long-running task so the queue stays busy.
    await submitPrompt(client, longTaskPrompt, timeoutMs);
    await waitForWorking(client, timeoutMs, "c04 long task turn");
    // Enqueue A, then B, while the long task is still executing.
    await submitPrompt(client, queuedPromptAOriginal, timeoutMs);
    await waitForQueuedRowCount(client, 1, timeoutMs, "C04 queued prompt A");
    await delay(visibleQueueStateMs);
    await submitPrompt(client, queuedPromptB, timeoutMs);
    await waitForQueuedRowCount(client, 2, timeoutMs, "C04 queued prompt B");
    await delay(visibleQueueStateMs);
    // Edit A: the more-actions dropdown (Radix trigger opens on pointerdown)
    // holds the single Edit item, which applies on pointerdown as well.
    await evaluate(
      client,
      `(() => {
        const button = ${actionButtonExpression("C04-QUEUED-A-ORIGINAL", 2)};
        button.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, button: 0, pointerId: 1 }));
        button.dispatchEvent(new PointerEvent('pointerup', { bubbles: true, button: 0, pointerId: 1 }));
        return true;
      })()`
    );
    await waitForEvaluation(
      client,
      `(() => {
        const items = [...document.querySelectorAll('[role="menuitem"]')];
        return { ready: items.length > 0, itemCount: items.length };
      })()`,
      timeoutMs,
      "C04 queued prompt A edit menu",
      25
    );
    await evaluate(
      client,
      `(() => {
        const item = [...document.querySelectorAll('[role="menuitem"]')].at(-1);
        if (!(item instanceof HTMLElement)) throw new Error('edit menu item is unavailable');
        item.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, button: 0, pointerId: 1 }));
        item.dispatchEvent(new PointerEvent('pointerup', { bubbles: true, button: 0, pointerId: 1 }));
        item.click();
        return true;
      })()`
    );
    // Editing moves A back into the composer, leaving only B queued.
    await waitForEvaluation(
      client,
      `(() => {
        const rows = ${queuedRowsExpression};
        const editor = document.querySelector('[data-testid="agent-gui-composer-editor"]');
        const draft = editor?.textContent ?? '';
        return {
          ready: rows.length === 1 && draft.includes('C04-QUEUED-A-ORIGINAL'),
          rowCount: rows.length,
          draftPreview: draft.slice(0, 60)
        };
      })()`,
      timeoutMs,
      "C04 queued prompt A moved to composer for edit",
      25
    );
    await delay(visibleQueueStateMs);
    // Re-submit the edited A (submitPrompt replaces the whole draft).
    await submitPrompt(client, queuedPromptAEdited, timeoutMs);
    await waitForQueuedRowCount(
      client,
      2,
      timeoutMs,
      "C04 edited queued prompt A"
    );
    await delay(visibleQueueStateMs);
    // Delete B.
    await clickQueuedRowAction(client, "C04-QUEUED-B", 1);
    await waitForEvaluation(
      client,
      `(() => {
        const rows = ${queuedRowsExpression};
        return {
          ready: rows.length === 1 &&
            (rows[0].textContent ?? '').includes('C04-QUEUED-A-EDITED'),
          rowCount: rows.length
        };
      })()`,
      timeoutMs,
      "C04 queued prompt B removed",
      25
    );
    // Leave edited A queued. Once the current Turn settles, normal queue
    // draining sends it as a distinct Turn.
    await delay(visibleQueueStateMs);
  },
  async assert({ client, phase, timeoutMs }) {
    if (phase !== "terminal") {
      return null;
    }
    // Terminal: the long task replied before edited A, edited A completed as
    // the next Turn, B never produced a turn, and the queue is empty.
    return waitForEvaluation(
      client,
      `(() => {
        const detail = document.querySelector('main[data-agent-session-id]');
        const markdown = [...(detail?.querySelectorAll('[data-workspace-agent-markdown="true"]') ?? [])];
        const firstReplyIndex = markdown.findIndex((element) =>
          (element.textContent ?? '').includes(${JSON.stringify(firstReplyMarker)})
        );
        const finalReplyIndex = markdown.findIndex((element) =>
          element.textContent?.trim() === ${JSON.stringify(finalToken)}
        );
        const forbiddenReply = markdown.some((element) =>
          (element.textContent ?? '').includes(${JSON.stringify(forbiddenBToken)})
        );
        const queuedRows = ${queuedRowsExpression};
        return {
          ready: Boolean(detail) &&
            firstReplyIndex >= 0 &&
            finalReplyIndex > firstReplyIndex &&
            !forbiddenReply &&
            queuedRows.length === 0 &&
            !document.querySelector('[data-testid="agent-gui-composer-stop-active-turn"]'),
          assistantText: finalReplyIndex >= 0 ? ${JSON.stringify(finalToken)} : '',
          activeSessionId: detail?.getAttribute('data-agent-session-id') ?? null
        };
      })()`,
      timeoutMs,
      "C04 queue auto-drain terminal state",
      50
    );
  }
});
