import {
  submitPrompt,
  waitForFinalToken,
  waitForWorking
} from "./cdp-helpers.mjs";
import { waitForEvaluation } from "../agent-gui-performance-helpers.mjs";
import { codexImageComposerDefaults, defineRecordScenario } from "./shared.mjs";

const composerDefaults = {
  ...codexImageComposerDefaults,
  permissionModeId: "full-access"
};

async function assertGeneratedImageArtifact(client, timeoutMs, label) {
  await waitForEvaluation(
    client,
    `(() => {
      const artifact = document.querySelector('main[data-agent-session-id] [data-testid="agent-generated-image-artifact"]');
      const image = artifact?.querySelector('img');
      return {
        ready:
          artifact instanceof HTMLElement &&
          image instanceof HTMLImageElement &&
          Boolean(image.getAttribute('src')),
        imageSource: image?.getAttribute('src') ?? null
      };
    })()`,
    timeoutMs,
    label,
    100
  );
  await waitForEvaluation(
    client,
    `(() => {
      const image = document.querySelector('main[data-agent-session-id] [data-testid="agent-generated-image-artifact"] img');
      if (!(image instanceof HTMLImageElement)) return { ready: false };
      image.click();
      return { ready: true };
    })()`,
    timeoutMs,
    `${label} open zoom preview`,
    25
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
  id: "r06",
  async prepare({ setComposerDefaults }) {
    await setComposerDefaults(composerDefaults);
  },
  async drive({ client, timeoutMs }) {
    await submitPrompt(
      client,
      "Use the built-in image generation tool to create a simple square image of a red circle centered on a white background. Do not use shell commands, Python, SVG, canvas, or another substitute. After the image generation tool completes, reply exactly R06_GENERATED_IMAGE_DONE and no other text.",
      timeoutMs
    );
    await waitForWorking(client, timeoutMs, "r06");
  },
  async assert({ client, phase, timeoutMs }) {
    if (phase !== "terminal") return null;
    const settled = await waitForFinalToken(
      client,
      "R06_GENERATED_IMAGE_DONE",
      timeoutMs
    );
    await assertGeneratedImageArtifact(
      client,
      timeoutMs,
      "r06 generated-image artifact"
    );
    return settled;
  }
});
