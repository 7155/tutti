import { waitForEvaluation } from "../agent-gui-performance-helpers.mjs";
import { enterAndSubmitComposerPrompt } from "../agent-gui-layout-performance-scenarios.mjs";

export { enterAndSubmitComposerPrompt as submitPrompt };

const ACTIVE_TURN_STOP_TEST_ID = "agent-gui-composer-stop-active-turn";
const ACTIVE_TURN_COMPOSER_TEST_ID = "agent-gui-composer-active-turn";

export async function submitGuidancePrompt(client, prompt, timeoutMs) {
  await evaluate(
    client,
    `(() => {
      const editor = document.querySelector('[data-testid="agent-gui-composer-editor"]');
      if (!(editor instanceof HTMLElement)) throw new Error('composer editor is unavailable');
      editor.focus();
      document.execCommand('selectAll', false);
      if (!document.execCommand('insertText', false, ${JSON.stringify(prompt)})) {
        throw new Error('could not enter guidance prompt');
      }
      return true;
    })()`
  );
  await waitForEvaluation(
    client,
    `(() => {
      const editor = document.querySelector('[data-testid="agent-gui-composer-editor"]');
      const activeTurnComposer = document.querySelector(
        '[data-testid="agent-gui-composer-active-turn"]'
      );
      return {
        ready:
          editor instanceof HTMLElement &&
          editor.getAttribute('aria-disabled') !== 'true' &&
          activeTurnComposer instanceof HTMLElement &&
          (editor.textContent ?? '').includes(${JSON.stringify(prompt)})
      };
    })()`,
    timeoutMs,
    "enabled composer guidance submission",
    25
  );
  await evaluate(
    client,
    `(() => {
      const editor = document.querySelector('[data-testid="agent-gui-composer-editor"]');
      if (!(editor instanceof HTMLElement)) throw new Error('composer editor is unavailable');
      const accepted = !editor.dispatchEvent(new KeyboardEvent('keydown', {
        key: 'Enter',
        code: 'Enter',
        metaKey: true,
        bubbles: true,
        cancelable: true
      }));
      if (!accepted) throw new Error('guidance keyboard submission was not handled');
      return true;
    })()`
  );
}

export async function enablePlanMode(client, timeoutMs) {
  await enableComposerMode(client, timeoutMs, "plan", "Agent plan mode");
}

// `/goal` is a prefix syntax, not a palette command: typing the full token
// flips the composer into goal draft mode and closes the command palette
// (`isGoalModeActive` nulls the slash query), so waiting for a palette
// option stalls forever. Type the whole `/goal <objective>` draft, wait for
// the goal badge to confirm the mode, then send in one step.
export async function submitGoalPrompt(client, objective, timeoutMs) {
  const goalDraft = `/goal ${objective}`;
  await evaluate(
    client,
    `(() => {
      const editor = document.querySelector('[data-testid="agent-gui-composer-editor"]');
      if (!(editor instanceof HTMLElement)) throw new Error('composer editor is unavailable');
      editor.focus();
      document.execCommand('selectAll', false);
      if (!document.execCommand('insertText', false, ${JSON.stringify(goalDraft)})) {
        throw new Error('could not enter goal draft');
      }
      return true;
    })()`
  );
  await waitForEvaluation(
    client,
    `(() => {
      const badge = document.querySelector('[data-agent-goal-badge="true"]');
      const send = document.querySelector('[data-testid="agent-gui-composer-send"]');
      return {
        ready:
          Boolean(badge) &&
          send instanceof HTMLButtonElement &&
          !send.disabled
      };
    })()`,
    timeoutMs,
    "Agent goal mode badge with enabled send button",
    25
  );
  await evaluate(
    client,
    `(() => {
      const send = document.querySelector('[data-testid="agent-gui-composer-send"]');
      if (!(send instanceof HTMLButtonElement) || send.disabled) {
        throw new Error('goal send button is unavailable');
      }
      send.click();
      return true;
    })()`
  );
}

export async function waitForWorking(client, timeoutMs, scenarioId) {
  await waitForEvaluation(
    client,
    `(() => {
      const stop = document.querySelector('[data-testid="agent-gui-composer-stop-active-turn"]');
      const activeTurnId = stop?.getAttribute('data-agent-turn-id')?.trim() ?? '';
      return {
        ready: stop instanceof HTMLButtonElement && !stop.disabled && Boolean(activeTurnId),
        activeTurnId: activeTurnId || null
      };
    })()`,
    timeoutMs,
    `${scenarioId.toUpperCase()} working state`,
    25
  );
}

export async function waitForSubAgentCard(
  client,
  timeoutMs,
  expectedStatus,
  label
) {
  return waitForEvaluation(
    client,
    `(() => {
      const detail = document.querySelector('main[data-agent-session-id]');
      const cards = [
        ...(detail?.querySelectorAll(
          '.workspace-agents-status-panel__detail-tool-row--subagent'
        ) ?? [])
      ];
      const card = cards.find((candidate) => {
        const trigger = candidate.querySelector('button[aria-label]');
        return (
          candidate instanceof HTMLElement &&
          trigger instanceof HTMLButtonElement &&
          trigger.getAttribute('aria-label')?.trim()
        );
      });
      const status = card?.getAttribute('data-status')?.trim() ?? '';
      return {
        ready:
          Boolean(detail) &&
          cards.length === 1 &&
          status === ${JSON.stringify(expectedStatus)},
        activeSessionId:
          detail?.getAttribute('data-agent-session-id') ?? null,
        cardCount: cards.length,
        status: status || null,
        ariaLabel:
          card?.querySelector('button[aria-label]')?.getAttribute('aria-label') ??
          null
      };
    })()`,
    timeoutMs,
    label,
    25
  );
}

export async function expandTurnWorkSections(client, timeoutMs, label) {
  return waitForEvaluation(
    client,
    `(() => {
      const detail = document.querySelector('main[data-agent-session-id]');
      const toggles = [
        ...(detail?.querySelectorAll(
          '[data-agent-turn-work-header] button[aria-expanded]'
        ) ?? [])
      ];
      for (const toggle of toggles) {
        if (
          toggle instanceof HTMLButtonElement &&
          toggle.getAttribute('aria-expanded') === 'false'
        ) {
          toggle.click();
        }
      }
      return {
        ready:
          Boolean(detail) &&
          toggles.length > 0 &&
          toggles.every(
            (toggle) => toggle.getAttribute('aria-expanded') === 'true'
          ),
        activeSessionId:
          detail?.getAttribute('data-agent-session-id') ?? null,
        toggleCount: toggles.length,
        expandedCount: toggles.filter(
          (toggle) => toggle.getAttribute('aria-expanded') === 'true'
        ).length
      };
    })()`,
    timeoutMs,
    label,
    25
  );
}

export async function startProjectSession(client, projectId, timeoutMs) {
  const testId = `agent-gui-project-${projectId}-new-session`;
  await waitForTestId(client, testId, timeoutMs, "project session action");
  await clickTestId(client, testId);
  await waitForEvaluation(
    client,
    `(() => {
      const shell = document.querySelector('[data-testid="agent-gui-composer-input-shell"]');
      return {
        ready: shell instanceof HTMLElement && shell.dataset.inputDisabled !== 'true'
      };
    })()`,
    timeoutMs,
    "project session composer",
    25
  );
}

export async function chooseApproval(client, action, timeoutMs) {
  const interactionId = await waitForInteractionId(
    client,
    "approval",
    timeoutMs
  );
  const testId = `agent-approval-${interactionId}-${action}`;
  await waitForTestId(client, testId, timeoutMs, `approval ${action}`);
  await clickTestId(client, testId);
}

export async function sendApprovalFeedback(client, feedback, timeoutMs) {
  const interactionId = await waitForInteractionId(
    client,
    "approval",
    timeoutMs
  );
  const testIdPrefix = `agent-approval-${interactionId}-feedback`;
  await waitForTestId(client, testIdPrefix, timeoutMs, "approval feedback");
  await clickTestId(client, testIdPrefix);
  await waitForTestId(
    client,
    `${testIdPrefix}-input`,
    timeoutMs,
    "approval feedback input"
  );
  await setTestIdTextareaValue(client, `${testIdPrefix}-input`, feedback);
  await waitForTestId(
    client,
    `${testIdPrefix}-submit`,
    timeoutMs,
    "approval feedback submit"
  );
  await clickTestId(client, `${testIdPrefix}-submit`);
}

export async function chooseQuestionOption(
  client,
  questionId,
  optionId,
  timeoutMs
) {
  const interactionId = await waitForInteractionId(
    client,
    "ask-user",
    timeoutMs
  );
  const testIdPrefix = `agent-question-${interactionId}-${questionId}`;
  await waitForTestId(client, testIdPrefix, timeoutMs, "question card");
  await clickTestId(client, `${testIdPrefix}-option-${optionId}`);
  await waitForTestId(
    client,
    `${testIdPrefix}-submit`,
    timeoutMs,
    "answer submit"
  );
  await clickTestId(client, `${testIdPrefix}-submit`);
}

// codex's request_user_input payload does not echo custom option ids back to
// the renderer: askUserQuestions.ts normalizes missing ids into
// `contract-option-<hash>` fallbacks, so option testids cannot be predicted
// from the recorded prompt. Match the rendered option by its visible label
// under the question card's `-option-` testid prefix instead.
export async function chooseQuestionOptionByLabel(
  client,
  questionId,
  optionLabel,
  timeoutMs
) {
  const interactionId = await waitForInteractionId(
    client,
    "ask-user",
    timeoutMs
  );
  const testIdPrefix = `agent-question-${interactionId}-${questionId}`;
  await waitForTestId(client, testIdPrefix, timeoutMs, "question card");
  const optionSelector = `[data-testid^="${testIdPrefix}-option-"]`;
  const optionResult = await waitForEvaluation(
    client,
    `(() => {
      const options = [...document.querySelectorAll(${JSON.stringify(optionSelector)})];
      const match = options.find((element) =>
        (element.textContent ?? '').includes(${JSON.stringify(optionLabel)})
      );
      return {
        ready: match instanceof HTMLElement && Boolean(match.getAttribute('data-testid')),
        optionTestId: match?.getAttribute('data-testid') ?? null,
        optionCount: options.length
      };
    })()`,
    timeoutMs,
    `question option labeled ${optionLabel}`,
    25
  );
  await clickTestId(client, optionResult.optionTestId);
  await waitForTestId(
    client,
    `${testIdPrefix}-submit`,
    timeoutMs,
    "answer submit"
  );
  await clickTestId(client, `${testIdPrefix}-submit`);
}

export async function submitCustomQuestionAnswer(
  client,
  questionId,
  answer,
  timeoutMs
) {
  const interactionId = await waitForInteractionId(
    client,
    "ask-user",
    timeoutMs
  );
  const testIdPrefix = `agent-question-${interactionId}-${questionId}`;
  await waitForTestId(client, testIdPrefix, timeoutMs, "question card");
  await setTestIdTextareaValue(client, `${testIdPrefix}-custom-answer`, answer);
  await waitForTestId(
    client,
    `${testIdPrefix}-submit`,
    timeoutMs,
    "answer submit"
  );
  await clickTestId(client, `${testIdPrefix}-submit`);
}

export async function confirmPlan(client, timeoutMs) {
  await waitForTestId(
    client,
    "agent-plan-implementation-implement",
    timeoutMs,
    "implementation plan"
  );
  await clickTestId(client, "agent-plan-implementation-implement");
}

export async function sendPlanFeedbackAndConfirm(client, feedback, timeoutMs) {
  await waitForTestId(
    client,
    "agent-plan-implementation-feedback",
    timeoutMs,
    "initial implementation plan"
  );
  await setTestIdTextareaValue(
    client,
    "agent-plan-implementation-feedback",
    feedback
  );
  await clickTestId(client, "agent-plan-implementation-continue");
  await waitForEvaluation(
    client,
    `({
      ready: Boolean(document.querySelector('[data-testid="agent-gui-composer-stop-active-turn"]')) ||
        !document.querySelector('[data-testid="agent-plan-implementation-implement"]')
    })`,
    timeoutMs,
    "plan feedback submission",
    25
  );
  await waitForTestId(
    client,
    "agent-plan-implementation-feedback",
    timeoutMs,
    "revised implementation plan"
  );
  await clickTestId(client, "agent-plan-implementation-implement");
}

export async function waitForTestId(client, testId, timeoutMs, label = testId) {
  return waitForEvaluation(
    client,
    `({ ready: document.querySelector('[data-testid=${JSON.stringify(testId)}]') instanceof HTMLElement })`,
    timeoutMs,
    label,
    25
  );
}

export async function clickTestId(client, testId) {
  // Interactive targets are not always <button> elements: slash-command
  // palette options render as <div role="option">. Accept any HTMLElement and
  // honor both the native and ARIA disabled contracts.
  await evaluate(
    client,
    `(() => {
      const element = document.querySelector('[data-testid=${JSON.stringify(testId)}]');
      if (
        !(element instanceof HTMLElement) ||
        (element instanceof HTMLButtonElement && element.disabled) ||
        element.getAttribute('aria-disabled') === 'true' ||
        element.dataset.disabled === 'true'
      ) {
        throw new Error(${JSON.stringify(`${testId} is unavailable`)});
      }
      element.click();
      return true;
    })()`
  );
}

export async function setTestIdTextareaValue(client, testId, value) {
  await evaluate(
    client,
    `(() => {
      const textarea = document.querySelector('[data-testid=${JSON.stringify(testId)}]');
      if (!(textarea instanceof HTMLTextAreaElement)) {
        throw new Error(${JSON.stringify(`${testId} is unavailable`)});
      }
      const setter = Object.getOwnPropertyDescriptor(
        HTMLTextAreaElement.prototype,
        'value'
      )?.set;
      setter?.call(textarea, ${JSON.stringify(value)});
      textarea.dispatchEvent(new Event('input', { bubbles: true }));
      textarea.dispatchEvent(new Event('change', { bubbles: true }));
      return true;
    })()`
  );
}

export async function waitForFinalToken(client, expectedToken, timeoutMs) {
  return waitForEvaluation(
    client,
    `(() => {
      const detail = document.querySelector('main[data-agent-session-id]');
      const markdown = [...(detail?.querySelectorAll('[data-workspace-agent-markdown="true"]') ?? [])];
      const token = ${JSON.stringify(expectedToken)};
      const assistant = markdown.find((element) =>
        element.textContent?.includes(token)
      );
      return {
        ready: Boolean(detail) && Boolean(assistant) &&
          !document.querySelector('[data-testid="agent-gui-composer-stop-active-turn"]'),
        assistantText: assistant?.textContent?.trim() ?? '',
        activeSessionId: detail?.getAttribute('data-agent-session-id') ?? null
      };
    })()`,
    timeoutMs,
    `${expectedToken} final response`,
    50
  );
}

// Goal completion hides the banner (`isGoalBannerVisible` excludes terminal
// statuses). Wait for that plus the marker token and a fully idle composer so
// seal does not race a brief inter-continuation idle while the Goal is still
// active.
export async function waitForGoalCompletedToken(
  client,
  expectedToken,
  timeoutMs
) {
  return waitForEvaluation(
    client,
    `(() => {
      const detail = document.querySelector('main[data-agent-session-id]');
      const banner = document.querySelector('[data-testid="agent-gui-goal-banner"]');
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
      const token = ${JSON.stringify(expectedToken)};
      const hasToken = assistantText.includes(token);
      const numbers = [...assistantText.matchAll(/\\b(\\d{1,3})\\b/g)].map(
        (match) => Number(match[1])
      );
      const maxNumber = numbers.reduce((max, value) => Math.max(max, value), 0);
      return {
        ready:
          Boolean(detail) &&
          hasToken &&
          banner == null &&
          !(stop instanceof HTMLButtonElement),
        hasToken,
        maxNumber,
        markdownCount: markdown.length,
        hasStop: stop instanceof HTMLButtonElement,
        assistantText: hasToken
          ? assistantText.trim().slice(-120)
          : '',
        goalBannerVisible: banner != null,
        activeSessionId: detail?.getAttribute('data-agent-session-id') ?? null
      };
    })()`,
    timeoutMs,
    `${expectedToken} goal-completed response`,
    50
  );
}

export async function waitForGoalBannerAction(
  client,
  actionTestId,
  timeoutMs,
  label
) {
  await waitForTestId(client, "agent-gui-goal-banner", timeoutMs, label);
  await waitForTestId(client, actionTestId, timeoutMs, label);
}

export async function clickGoalBannerAction(client, actionTestId) {
  await clickTestId(client, actionTestId);
}

// Cleared goals drop the banner (`goalObjective` becomes null) and leave the
// composer idle. Do not require a final assistant token: clear can interrupt
// mid-continuation before any marker text lands.
export async function waitForGoalCleared(client, timeoutMs) {
  return waitForEvaluation(
    client,
    `(() => {
      const detail = document.querySelector('main[data-agent-session-id]');
      const banner = document.querySelector('[data-testid="agent-gui-goal-banner"]');
      const stop = document.querySelector(
        '[data-testid="agent-gui-composer-stop-active-turn"]'
      );
      const clear = document.querySelector(
        '[data-testid="agent-gui-goal-banner-clear"]'
      );
      const clearHint = document.querySelector(
        '[data-testid="agent-gui-goal-banner-clear-hint"]'
      );
      const pause = document.querySelector(
        '[data-testid="agent-gui-goal-banner-pause"]'
      );
      const resume = document.querySelector(
        '[data-testid="agent-gui-goal-banner-resume"]'
      );
      const markdown = [
        ...(detail?.querySelectorAll('[data-workspace-agent-markdown="true"]') ??
          [])
      ];
      const assistant = markdown.at(-1);
      const goalControlRows = [
        ...(detail?.querySelectorAll('[data-agent-interaction-kind]') ?? [])
      ]
        .map((element) => ({
          kind: element.getAttribute('data-agent-interaction-kind') ?? '',
          text: (element.textContent ?? '').trim().slice(0, 80)
        }))
        .filter((row) => /goal/i.test(row.kind) || /\\/goal\\s+clear/i.test(row.text));
      return {
        ready:
          Boolean(detail) &&
          banner == null &&
          !(stop instanceof HTMLButtonElement),
        assistantText: assistant?.textContent?.trim() ?? '',
        goalBannerVisible: banner != null,
        goalBannerText: banner?.textContent?.trim().slice(0, 120) ?? '',
        hasClear: clear instanceof HTMLElement,
        hasClearHint: clearHint instanceof HTMLElement,
        hasPause: pause instanceof HTMLElement,
        hasResume: resume instanceof HTMLElement,
        goalControlRows,
        activeSessionId: detail?.getAttribute('data-agent-session-id') ?? null
      };
    })()`,
    timeoutMs,
    "goal cleared idle state",
    50
  );
}

// Paused goals keep the banner but swap pause→resume and stop the active turn.
export async function waitForGoalPaused(client, timeoutMs) {
  return waitForEvaluation(
    client,
    `(() => {
      const detail = document.querySelector('main[data-agent-session-id]');
      const banner = document.querySelector('[data-testid="agent-gui-goal-banner"]');
      const resume = document.querySelector(
        '[data-testid="agent-gui-goal-banner-resume"]'
      );
      const pause = document.querySelector(
        '[data-testid="agent-gui-goal-banner-pause"]'
      );
      const stop = document.querySelector(
        '[data-testid="agent-gui-composer-stop-active-turn"]'
      );
      return {
        ready:
          Boolean(detail) &&
          banner instanceof HTMLElement &&
          resume instanceof HTMLElement &&
          !(pause instanceof HTMLElement) &&
          !(stop instanceof HTMLButtonElement),
        goalBannerVisible: banner != null,
        goalBannerText: banner?.textContent?.trim().slice(0, 120) ?? '',
        hasPause: pause instanceof HTMLElement,
        hasResume: resume instanceof HTMLElement,
        hasStop: stop instanceof HTMLButtonElement,
        activeSessionId: detail?.getAttribute('data-agent-session-id') ?? null
      };
    })()`,
    timeoutMs,
    "goal paused state",
    50
  );
}

export async function waitForGoalResumed(client, timeoutMs) {
  return waitForEvaluation(
    client,
    `(() => {
      const detail = document.querySelector('main[data-agent-session-id]');
      const banner = document.querySelector('[data-testid="agent-gui-goal-banner"]');
      const pause = document.querySelector(
        '[data-testid="agent-gui-goal-banner-pause"]'
      );
      const resume = document.querySelector(
        '[data-testid="agent-gui-goal-banner-resume"]'
      );
      const stop = document.querySelector(
        '[data-testid="agent-gui-composer-stop-active-turn"]'
      );
      const activeTurnId = stop?.getAttribute('data-agent-turn-id')?.trim() ?? '';
      return {
        ready:
          Boolean(detail) &&
          banner instanceof HTMLElement &&
          pause instanceof HTMLElement &&
          !(resume instanceof HTMLElement) &&
          stop instanceof HTMLButtonElement &&
          !stop.disabled &&
          Boolean(activeTurnId),
        activeTurnId: activeTurnId || null,
        goalBannerVisible: banner != null,
        goalBannerText: banner?.textContent?.trim().slice(0, 120) ?? '',
        hasPause: pause instanceof HTMLElement,
        hasResume: resume instanceof HTMLElement,
        hasStop: stop instanceof HTMLButtonElement,
        activeSessionId: detail?.getAttribute('data-agent-session-id') ?? null
      };
    })()`,
    timeoutMs,
    "goal resumed working state",
    50
  );
}

export async function waitForTerminalResponse(
  client,
  timeoutMs,
  scenarioToken
) {
  return waitForEvaluation(
    client,
    `(() => {
      const detail = document.querySelector('main[data-agent-session-id]');
      const markdown = [...(detail?.querySelectorAll('[data-workspace-agent-markdown="true"]') ?? [])];
      const assistant = markdown.at(-1);
      return {
        ready: Boolean(detail) && Boolean(assistant?.textContent?.trim()) &&
          !document.querySelector('[data-testid="agent-gui-composer-stop-active-turn"]'),
        assistantText: assistant?.textContent?.trim() ?? '',
        activeSessionId: detail?.getAttribute('data-agent-session-id') ?? null,
        scenarioToken: ${JSON.stringify(scenarioToken)}
      };
    })()`,
    timeoutMs,
    `${scenarioToken} terminal response`,
    50
  );
}

// `/compact` is a plain palette command (commandEffect=submitImmediate for
// codex, see agentSlashCommandProviderPolicy): typing a prefix keeps the
// palette open and clicking the palette option submits the compaction command
// immediately. Unlike `/goal` (prefix syntax that closes the palette), waiting
// for the palette option here is safe.
export async function triggerCompaction(client, timeoutMs, label) {
  await evaluate(
    client,
    `(() => {
      const editor = document.querySelector('[data-testid="agent-gui-composer-editor"]');
      if (!(editor instanceof HTMLElement)) throw new Error('composer editor is unavailable');
      editor.focus();
      document.execCommand('selectAll', false);
      if (!document.execCommand('insertText', false, '/compac')) {
        throw new Error('could not enter /compact prefix');
      }
      return true;
    })()`
  );
  const testId = "agent-gui-composer-slash-command-compact";
  await waitForTestId(
    client,
    testId,
    timeoutMs,
    `${label} compact slash command`
  );
  await clickTestId(client, testId);
}

// Compaction progress/terminal state renders as ContextCompactionDivider rows
// (AgentMessageBlock.tsx) with role="status" and localized text but no
// dedicated testid; match the localized labels from both bundled locales.
// The daemon replaces the running notice in place with the terminal notice.
const COMPACTION_STATUS_TEXTS = Object.freeze({
  running: ["Compacting context", "正在压缩上下文"],
  completed: ["Context compacted.", "已压缩上下文"],
  interrupted: ["Context compaction interrupted.", "上下文压缩已中断"]
});

export async function waitForCompactionStatus(
  client,
  status,
  timeoutMs,
  label,
  minCount = 1
) {
  const texts = COMPACTION_STATUS_TEXTS[status];
  if (!texts) {
    throw new Error(`unknown compaction status ${status}`);
  }
  return waitForEvaluation(
    client,
    `(() => {
      const rows = [...document.querySelectorAll('main[data-agent-session-id] [role="status"]')];
      const texts = ${JSON.stringify(texts)};
      const matches = rows.filter((element) => {
        const text = element.textContent ?? '';
        return texts.some((candidate) => text.includes(candidate));
      });
      return {
        ready: matches.length >= ${minCount},
        count: matches.length,
        lastText: matches.at(-1)?.textContent?.trim() ?? null
      };
    })()`,
    timeoutMs,
    label,
    25
  );
}

export async function waitForIdleComposer(client, timeoutMs, label) {
  await waitForEvaluation(
    client,
    `({ ready: !document.querySelector('[data-testid="agent-gui-composer-stop-active-turn"]') })`,
    timeoutMs,
    `${label} idle composer`,
    25
  );
}

export async function cancelActiveTurn(client, timeoutMs, label) {
  await clickTestId(client, ACTIVE_TURN_STOP_TEST_ID);
  await waitForEvaluation(
    client,
    `({ ready: !document.querySelector('[data-testid=${JSON.stringify(ACTIVE_TURN_COMPOSER_TEST_ID)}]') })`,
    timeoutMs,
    `${label} canceled turn settled`,
    25
  );
}

export async function submitImageOnlyPrompt(
  client,
  timeoutMs,
  label,
  imageBase64,
  imageName
) {
  if (!imageBase64 || !imageName) {
    throw new Error(`${label} image fixture is unavailable`);
  }
  await evaluate(
    client,
    `(() => {
      const editor = document.querySelector('[data-testid="agent-gui-composer-editor"]');
      if (!(editor instanceof HTMLElement)) throw new Error(${JSON.stringify(`${label} composer editor is unavailable`)});
      const bytes = Uint8Array.from(atob(${JSON.stringify(imageBase64)}), (character) => character.charCodeAt(0));
      const transfer = new DataTransfer();
      transfer.items.add(new File([bytes], ${JSON.stringify(imageName)}, { type: 'image/png' }));
      editor.dispatchEvent(new ClipboardEvent('paste', {
        bubbles: true,
        cancelable: true,
        clipboardData: transfer
      }));
      return true;
    })()`
  );
  await waitForTestId(
    client,
    "agent-gui-composer-image-drafts",
    timeoutMs,
    `${label} image preview`
  );
  // The send button is not disabled while the image is still uploading; a
  // click in that window is silently dropped by the composer submit gate.
  // Wait until every draft image finished uploading before clicking.
  await waitForEvaluation(
    client,
    `(() => {
      const drafts = document.querySelector('[data-testid="agent-gui-composer-image-drafts"]');
      const uploading = drafts?.querySelector('[data-uploading="true"]');
      const send = document.querySelector('[data-testid="agent-gui-composer-send"]');
      return {
        ready:
          Boolean(drafts) &&
          !uploading &&
          send instanceof HTMLButtonElement &&
          !send.disabled
      };
    })()`,
    timeoutMs,
    `${label} uploaded image with enabled send button`,
    25
  );
  await evaluate(
    client,
    `(() => {
      const send = document.querySelector('[data-testid="agent-gui-composer-send"]');
      if (!(send instanceof HTMLButtonElement) || send.disabled) {
        throw new Error(${JSON.stringify(`${label} image-only send button is unavailable`)});
      }
      send.click();
      return true;
    })()`
  );
}

async function enableComposerMode(client, timeoutMs, mode, label) {
  await evaluate(
    client,
    `(() => {
      const editor = document.querySelector('[data-testid="agent-gui-composer-editor"]');
      if (!(editor instanceof HTMLElement)) throw new Error('composer editor is unavailable');
      editor.focus();
      document.execCommand('selectAll', false);
      if (!document.execCommand('insertText', false, ${JSON.stringify(`/${mode}`)})) {
        throw new Error(${JSON.stringify(`could not enter /${mode}`)});
      }
      return true;
    })()`
  );
  const testId = `agent-gui-composer-slash-command-${mode}`;
  await waitForTestId(client, testId, timeoutMs, `${label} slash command`);
  await clickTestId(client, testId);
  await waitForEvaluation(
    client,
    `({
      ready: Boolean(document.querySelector(${JSON.stringify(
        mode === "plan"
          ? '[data-agent-plan-mode-badge="true"]'
          : '[data-agent-goal-badge="true"]'
      )}))
    })`,
    timeoutMs,
    label,
    25
  );
}

async function waitForInteractionId(client, kind, timeoutMs) {
  const result = await waitForEvaluation(
    client,
    `(() => {
      const interactions = [...document.querySelectorAll(${JSON.stringify(`[data-agent-interaction-kind="${kind}"]`)})];
      const interaction = interactions.length === 1 ? interactions[0] : null;
      const interactionId = interaction?.getAttribute('data-agent-interaction-id')?.trim() ?? '';
      return { ready: Boolean(interactionId), interactionId };
    })()`,
    timeoutMs,
    `${kind} interaction`,
    25
  );
  return result.interactionId;
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
