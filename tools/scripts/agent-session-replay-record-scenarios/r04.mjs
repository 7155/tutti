import { readFile } from "node:fs/promises";
import { waitForFinalToken, waitForWorking } from "./cdp-helpers.mjs";
import { waitForEvaluation } from "../agent-gui-performance-helpers.mjs";
import { codexImageComposerDefaults, defineRecordScenario } from "./shared.mjs";

const PROMPT_TEXT =
  "Inspect the attached image. If it shows a stylized feline character wearing round glasses, reply exactly R04_IMAGE_TEXT_DONE. Otherwise reply exactly R04_IMAGE_TEXT_MISMATCH. Do not add any other text.";
const IMAGE_FIXTURE_BASE64 = (
  await readFile(
    new URL(
      "../../fixtures/agent-session-replay/r04-r05-image-input.png",
      import.meta.url
    )
  )
).toString("base64");

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

async function submitImageAndTextPrompt(client, timeoutMs, label) {
  // Enter the prompt text before pasting the image: the editor text-change
  // path publishes the composer draft from a React snapshot, while the paste
  // path reads the live prompt ref. Typing after the paste can publish a
  // draft without the image and silently drop it from the submit.
  await evaluate(
    client,
    `(() => {
      const editor = document.querySelector('[data-testid="agent-gui-composer-editor"]');
      if (!(editor instanceof HTMLElement)) throw new Error(${JSON.stringify(`${label} composer editor is unavailable`)});
      editor.focus();
      document.execCommand('selectAll', false);
      if (!document.execCommand('insertText', false, ${JSON.stringify(PROMPT_TEXT)})) {
        throw new Error(${JSON.stringify(`${label} could not enter composer prompt text`)});
      }
      return true;
    })()`
  );
  await evaluate(
    client,
    `(() => {
      const editor = document.querySelector('[data-testid="agent-gui-composer-editor"]');
      if (!(editor instanceof HTMLElement)) throw new Error(${JSON.stringify(`${label} composer editor is unavailable`)});
      const bytes = Uint8Array.from(atob(${JSON.stringify(IMAGE_FIXTURE_BASE64)}), (character) => character.charCodeAt(0));
      const transfer = new DataTransfer();
      transfer.items.add(new File([bytes], 'r04-image-input.png', { type: 'image/png' }));
      editor.dispatchEvent(new ClipboardEvent('paste', {
        bubbles: true,
        cancelable: true,
        clipboardData: transfer
      }));
      return true;
    })()`
  );
  await waitForEvaluation(
    client,
    `({ ready: document.querySelector('[data-testid="agent-gui-composer-image-drafts"]') instanceof HTMLElement })`,
    timeoutMs,
    `${label} image preview`,
    25
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
      const drafts = document.querySelector('[data-testid="agent-gui-composer-image-drafts"]');
      const previewCount = drafts ? drafts.querySelectorAll('img').length : 0;
      if (!drafts || previewCount === 0) {
        throw new Error(${JSON.stringify(`${label} draft image disappeared before send`)} + ' previews=' + previewCount);
      }
      const send = document.querySelector('[data-testid="agent-gui-composer-send"]');
      if (!(send instanceof HTMLButtonElement) || send.disabled) {
        throw new Error(${JSON.stringify(`${label} image-and-text send button is unavailable`)});
      }
      send.click();
      return true;
    })()`
  );
}

async function assertUserMessageHasImageAndText(client, timeoutMs, label) {
  // The user image grid renders as a sibling of the user text bubble inside
  // the message flow, outside the [data-agent-message-speaker="user"] layout
  // element, so query the transcript root for the uploaded image.
  await waitForEvaluation(
    client,
    `(() => {
      const detail = document.querySelector('main[data-agent-session-id]');
      const userMessages = [...(detail?.querySelectorAll('[data-agent-message-speaker="user"]') ?? [])];
      const userText = userMessages.map((element) => element.textContent ?? '').join(' ');
      const loadedImage = detail?.querySelector('.tsh-zoomable-image img');
      return {
        ready:
          userMessages.length > 0 &&
          userText.includes('R04_IMAGE_TEXT_DONE') &&
          loadedImage instanceof HTMLImageElement &&
          Boolean(loadedImage.getAttribute('src')),
        userMessageCount: userMessages.length,
        hasLoadedImage: Boolean(loadedImage)
      };
    })()`,
    timeoutMs,
    label,
    100
  );
  await evaluate(
    client,
    `(() => {
      const image = document.querySelector('main[data-agent-session-id] .tsh-zoomable-image img');
      if (!(image instanceof HTMLImageElement)) throw new Error(${JSON.stringify(`${label} image is unavailable`)});
      image.click();
      return true;
    })()`
  );
  await waitForEvaluation(
    client,
    `({ ready: document.querySelector('[data-rmiz-modal][role="dialog"]') instanceof HTMLElement })`,
    timeoutMs,
    `${label} zoom preview`,
    25
  );
}

export default defineRecordScenario({
  id: "r04",
  async prepare({ setComposerDefaults }) {
    await setComposerDefaults(codexImageComposerDefaults);
  },
  async drive({ client, timeoutMs }) {
    await submitImageAndTextPrompt(client, timeoutMs, "r04");
    await waitForWorking(client, timeoutMs, "r04 image-and-text turn");
  },
  async assert({ client, phase, timeoutMs }) {
    if (phase !== "terminal") return null;
    const settled = await waitForFinalToken(
      client,
      "R04_IMAGE_TEXT_DONE",
      timeoutMs
    );
    await assertUserMessageHasImageAndText(
      client,
      timeoutMs,
      "r04 user message with image and text"
    );
    return settled;
  }
});
