import assert from "node:assert/strict";
import test from "node:test";
import {
  recordCaseCommandPlan,
  recordScenarioIdFromArgs
} from "./record-agent-session-replay-case.mjs";
import { recordScenarioIds } from "./agent-session-replay-record-scenarios/definitions.mjs";

test("each defined scenario has a full verification plan", () => {
  for (const scenarioId of recordScenarioIds) {
    const plan = recordCaseCommandPlan(scenarioId);
    assert.equal(plan.commands.length, 3);
    assert.deepEqual(
      plan.commands.map((command) => command.command),
      ["pnpm", "node", "pnpm"]
    );
    assert.equal(plan.commands[0].args.at(-1), "300000");
    assert.equal(plan.commands[2].args.at(-1), "300000");
    assert.ok(plan.commands[1].args[0].endsWith("audit-cassette.mjs"));
  }
});

test("record plans reject unknown scenarios", () => {
  assert.throws(() => recordCaseCommandPlan("unknown"), /unsupported/u);
  assert.equal(
    recordCaseCommandPlan("i01").cassetteDirectory,
    ".tmp/cassettes/i01_codex"
  );
});

test("record command requires exactly one scenario ID", () => {
  assert.equal(recordScenarioIdFromArgs(["c01"]), "c01");
  assert.throws(
    () => recordScenarioIdFromArgs([]),
    /record:agent-gui <scenario-id>/u
  );
  assert.throws(
    () => recordScenarioIdFromArgs(["c01", "i03"]),
    /record:agent-gui <scenario-id>/u
  );
});
