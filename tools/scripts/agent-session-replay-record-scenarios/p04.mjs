import { waitForEvaluation } from "../agent-gui-performance-helpers.mjs";
import { waitForTestId, waitForWorking } from "./cdp-helpers.mjs";
import { codexImageComposerDefaults, defineRecordScenario } from "./shared.mjs";

const prompt =
  "This message carries one attached image (a tiny 1x1 PNG test attachment). Reply with exactly P04_ATTACHMENT_PORTABLE_DONE and no other text.";

async function waitForFinalTokenWithDiagnostics(client, token, timeoutMs) {
  return waitForEvaluation(
    client,
    `(() => {
      const detail = document.querySelector('main[data-agent-session-id]');
      const markdown = [...(detail?.querySelectorAll('[data-workspace-agent-markdown="true"]') ?? [])];
      const assistant = markdown.find((element) =>
        element.textContent?.trim() === ${JSON.stringify(token)}
      );
      return {
        ready: Boolean(detail) && Boolean(assistant) &&
          !document.querySelector('[data-testid="agent-gui-composer-stop-active-turn"]'),
        assistantText: assistant?.textContent?.trim() ?? '',
        lastMarkdownText: markdown.at(-1)?.textContent?.trim().slice(0, 200) ?? '',
        turnActive: Boolean(document.querySelector('[data-testid="agent-gui-composer-stop-active-turn"]')),
        activeSessionId: detail?.getAttribute('data-agent-session-id') ?? null
      };
    })()`,
    timeoutMs,
    `${token} final response`,
    50
  );
}

async function submitTextAndImagePrompt(client, timeoutMs, label) {
  await evaluate(
    client,
    `(() => {
      const editor = document.querySelector('[data-testid="agent-gui-composer-editor"]');
      if (!(editor instanceof HTMLElement)) throw new Error(${JSON.stringify(`${label} composer editor is unavailable`)});
      editor.focus();
      document.execCommand('selectAll', false);
      if (!document.execCommand('insertText', false, ${JSON.stringify(prompt)})) {
        throw new Error(${JSON.stringify(`${label} could not enter composer prompt`)});
      }
      const bytes = Uint8Array.from(atob('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9WlUFOkAAAAASUVORK5CYII='), (character) => character.charCodeAt(0));
      const transfer = new DataTransfer();
      transfer.items.add(new File([bytes], 'p04-portable-attachment.png', { type: 'image/png' }));
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
        throw new Error(${JSON.stringify(`${label} send button is unavailable`)});
      }
      send.click();
      return true;
    })()`
  );
}

async function assertUserMessageAttachmentVisible(client, timeoutMs) {
  await waitForEvaluation(
    client,
    `(() => {
      const detail = document.querySelector('main[data-agent-session-id]');
      const thumbnails = [...(detail?.querySelectorAll('.agent-gui-conversation__user-image-thumbnail') ?? [])];
      const image = thumbnails
        .flatMap((thumbnail) => [...thumbnail.querySelectorAll('img')])
        .find((element) => Boolean(element.getAttribute('src')));
      return {
        ready: Boolean(detail) && thumbnails.length === 1 && Boolean(image),
        thumbnailCount: thumbnails.length,
        imageSrcKind: image?.getAttribute('src')?.split(/[:,]/)[0] ?? null
      };
    })()`,
    timeoutMs,
    "P04 user message attachment visible",
    50
  );
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

export default defineRecordScenario({
  id: "p04",
  async prepare({ setComposerDefaults }) {
    await setComposerDefaults(codexImageComposerDefaults);
  },
  async drive({ client, timeoutMs }) {
    await submitTextAndImagePrompt(client, timeoutMs, "p04");
    await waitForWorking(client, timeoutMs, "p04");
  },
  async assert({ client, phase, timeoutMs }) {
    if (phase !== "terminal") return null;
    const settled = await waitForFinalTokenWithDiagnostics(
      client,
      "P04_ATTACHMENT_PORTABLE_DONE",
      timeoutMs
    );
    await assertUserMessageAttachmentVisible(client, timeoutMs);
    return settled;
  }
});
