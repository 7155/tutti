import {
  chooseQuestionOptionByLabel,
  enablePlanMode,
  submitPrompt,
  waitForFinalToken,
  waitForWorking
} from "./cdp-helpers.mjs";
import { codexComposerDefaults, defineRecordScenario } from "./shared.mjs";

export default defineRecordScenario({
  id: "i04",
  async prepare({ setComposerDefaults }) {
    await setComposerDefaults(codexComposerDefaults);
  },
  async drive({ client, timeoutMs }) {
    await enablePlanMode(client, timeoutMs);
    // Do not demand custom option IDs: codex's request_user_input payload does
    // not echo them back, so the renderer falls back to hashed option ids.
    // Select the option by its visible label instead.
    await submitPrompt(
      client,
      "You must call request_user_input exactly once. Ask one question with header 'I04 Choice', id 'i04_choice', question 'Which deterministic path should I use?', and exactly two options labeled 'Alpha path' and 'Beta path'. After the user answers, use no other tools and reply exactly I04_OPTION_BETA.",
      timeoutMs
    );
    await waitForWorking(client, timeoutMs, "i04");
    await chooseQuestionOptionByLabel(
      client,
      "i04_choice",
      "Beta path",
      timeoutMs
    );
  },
  async assert({ client, phase, timeoutMs }) {
    return phase === "terminal"
      ? waitForFinalToken(client, "I04_OPTION_BETA", timeoutMs)
      : null;
  }
});
