import {
  startProjectSession,
  submitPrompt,
  waitForFinalToken,
  waitForWorking
} from "./cdp-helpers.mjs";
import { codexComposerDefaults, defineRecordScenario } from "./shared.mjs";

const project = { label: "Replay Project", relativePath: "." };

export default defineRecordScenario({
  id: "p01",
  project,
  async prepare({ selectProject, setComposerDefaults }) {
    await setComposerDefaults(codexComposerDefaults);
    return { project: await selectProject(project) };
  },
  async drive({ client, scenarioState, timeoutMs }) {
    await startProjectSession(client, scenarioState.project.id, timeoutMs);
    await submitPrompt(
      client,
      "Use the shell to report the current working directory and read tools/fixtures/agent-session-replay/p01-project-marker.txt. Then reply exactly P01_PROJECT_SESSION_COMPLETE.",
      timeoutMs
    );
    await waitForWorking(client, timeoutMs, "p01");
  },
  async assert({ client, phase, timeoutMs, verifyProjectBinding }) {
    if (phase === "terminal") {
      return waitForFinalToken(
        client,
        "P01_PROJECT_SESSION_COMPLETE",
        timeoutMs
      );
    }
    if (phase === "recorded") {
      await verifyProjectBinding();
    }
    return null;
  }
});
