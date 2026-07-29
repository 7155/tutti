import assert from "node:assert/strict";
import test from "node:test";
import {
  recordScenarioDefinitions,
  recordScenarioIds
} from "./agent-session-replay-record-scenarios/definitions.mjs";

test("record scenarios declare complete behavior without runner dispatch metadata", () => {
  assert.deepEqual(recordScenarioIds, [
    "c01",
    "c02",
    "c03",
    "c04",
    "c05",
    "c06",
    "i01",
    "i02",
    "i03",
    "i04",
    "i05",
    "i06",
    "i07",
    "i08",
    "i09",
    "i10",
    "r01",
    "r02",
    "r03",
    "r04",
    "r05",
    "r06",
    "r07",
    "l01",
    "l02",
    "l03",
    "l04",
    "l05",
    "l06",
    "p01",
    "p02",
    "p03",
    "p04"
  ]);
  assert.equal(
    new Set(
      Object.values(recordScenarioDefinitions).map(
        (scenario) => scenario.cassetteName
      )
    ).size,
    recordScenarioIds.length
  );
  for (const [id, scenario] of Object.entries(recordScenarioDefinitions)) {
    assert.equal(scenario.id, id);
    assert.equal(scenario.provider, "codex");
    assert.equal(scenario.cassetteName, `${id}_codex`);
    assert.equal(typeof scenario.prepare, "function");
    assert.equal(typeof scenario.drive, "function");
    assert.equal(typeof scenario.assert, "function");
    assert.equal(
      scenario.expectedRecordingMode,
      id === "c02" ? "continue-session" : "create-session"
    );
    assert.equal(
      typeof scenario.setupInitialState,
      id === "c02" ? "function" : "undefined"
    );
    assert.equal(Object.hasOwn(scenario, "kind"), false);
  }
});
