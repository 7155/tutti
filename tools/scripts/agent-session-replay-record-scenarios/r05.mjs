import { readFile } from "node:fs/promises";
import { submitImageOnlyPrompt, waitForWorking } from "./cdp-helpers.mjs";
import { waitForEvaluation } from "../agent-gui-performance-helpers.mjs";
import { codexImageComposerDefaults, defineRecordScenario } from "./shared.mjs";

const IMAGE_FIXTURE_BASE64 = (
  await readFile(
    new URL(
      "../../fixtures/agent-session-replay/r04-r05-image-input.png",
      import.meta.url
    )
  )
).toString("base64");

async function assertImageOnlyResult(client, timeoutMs) {
  const result = await waitForEvaluation(
    client,
    `(() => {
      const detail = document.querySelector('main[data-agent-session-id]');
      const userMessages = [...(detail?.querySelectorAll('[data-agent-message-speaker="user"]') ?? [])];
      const userText = userMessages.map((element) => element.textContent ?? '').join('').trim();
      const image = detail?.querySelector('.tsh-zoomable-image img');
      const markdown = [...(detail?.querySelectorAll('[data-workspace-agent-markdown="true"]') ?? [])];
      const assistantText = markdown.at(-1)?.textContent?.trim() ?? '';
      const acknowledgesImage = /\\bimage\\b/iu.test(assistantText);
      const reportsMissingImage = /(?:can't|cannot|unable to) (?:inspect|see)|omitted image|reattach/iu.test(assistantText);
      return {
        ready:
          Boolean(detail) &&
          userMessages.length === 0 &&
          userText === '' &&
          image instanceof HTMLImageElement &&
          Boolean(image.getAttribute('src')) &&
          acknowledgesImage &&
          !reportsMissingImage &&
          !document.querySelector('[data-testid="agent-gui-composer-stop-active-turn"]'),
        activeSessionId:
          detail?.getAttribute('data-agent-session-id') ?? null,
        userText,
        assistantText,
        acknowledgesImage,
        reportsMissingImage
      };
    })()`,
    timeoutMs,
    "r05 image-only semantic response",
    50
  );
  await waitForEvaluation(
    client,
    `(() => {
      const image = document.querySelector('main[data-agent-session-id] .tsh-zoomable-image img');
      if (!(image instanceof HTMLImageElement)) return { ready: false };
      image.click();
      return { ready: true };
    })()`,
    timeoutMs,
    "r05 open image preview",
    25
  );
  await waitForEvaluation(
    client,
    `({ ready: document.querySelector('[data-rmiz-modal][role="dialog"]') instanceof HTMLElement })`,
    timeoutMs,
    "r05 image zoom preview",
    25
  );
  return result;
}

export default defineRecordScenario({
  id: "r05",
  async prepare({ setComposerDefaults }) {
    await setComposerDefaults(codexImageComposerDefaults);
  },
  async drive({ client, timeoutMs }) {
    await submitImageOnlyPrompt(
      client,
      timeoutMs,
      "r05",
      IMAGE_FIXTURE_BASE64,
      "r05-image-only.png"
    );
    await waitForWorking(client, timeoutMs, "r05 image-only turn");
  },
  async assert({ client, phase, timeoutMs }) {
    return phase === "terminal"
      ? assertImageOnlyResult(client, timeoutMs)
      : null;
  }
});
