import {
  enablePlanMode,
  submitCustomQuestionAnswer,
  submitPrompt,
  waitForFinalToken,
  waitForWorking
} from "./cdp-helpers.mjs";
import { codexComposerDefaults, defineRecordScenario } from "./shared.mjs";

export default defineRecordScenario({
  id: "i05",
  async prepare({ setComposerDefaults }) {
    await setComposerDefaults(codexComposerDefaults);
  },
  async drive({ client, timeoutMs }) {
    await enablePlanMode(client, timeoutMs);
    await submitPrompt(
      client,
      "You must call request_user_input exactly once. Ask one question with header 'I05 Choice', id 'i05_choice', question 'What custom deterministic answer should I record?', and exactly two options with IDs: { id: 'i05_preset_one', label: 'Preset one' } and { id: 'i05_preset_two', label: 'Preset two' }. After the user supplies a custom answer, use no other tools and reply exactly I05_CUSTOM_ANSWER_RECEIVED.",
      timeoutMs
    );
    await waitForWorking(client, timeoutMs, "i05");
    await submitCustomQuestionAnswer(
      client,
      "i05_choice",
      "Tutti I05 custom answer",
      timeoutMs
    );
  },
  async assert({ client, phase, timeoutMs }) {
    return phase === "terminal"
      ? waitForFinalToken(client, "I05_CUSTOM_ANSWER_RECEIVED", timeoutMs)
      : null;
  }
});
