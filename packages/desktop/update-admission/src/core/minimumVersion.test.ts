import assert from "node:assert/strict";
import test from "node:test";
import {
  shouldCheckMinimumVersionAfterForeground,
  updaterTargetMeetsMinimum,
  validateMinimumVersionResponse
} from "./minimumVersion.ts";

test("compares updater targets without numeric precision loss", () => {
  assert.equal(updaterTargetMeetsMinimum("1.6.0", "1.6.0"), true);
  assert.equal(updaterTargetMeetsMinimum("1.7.0", "1.7.0-rc.2"), true);
  assert.equal(updaterTargetMeetsMinimum("1.7.0-rc.1", "1.7.0-rc.2"), false);
  assert.equal(updaterTargetMeetsMinimum("1.7.0-beta.1", "1.7.0"), false);
  assert.equal(
    updaterTargetMeetsMinimum(
      "900719925474099312345.0.0",
      "900719925474099312344.999.999"
    ),
    true
  );
});

test("validates a response against the exact request identity", () => {
  const request = {
    architecture: "arm64",
    currentVersion: "1.6.0",
    platform: "macos",
    product: "tsh-desktop"
  } as const;
  assert.equal(
    validateMinimumVersionResponse(
      {
        ...request,
        channel: "stable",
        decision: "upgradeRequired",
        minimumVersion: "1.6.1",
        policyRevision: "revision-1",
        policySource: "defaultMinimum",
        reason: "belowMinimum"
      },
      request
    ).decision,
    "upgradeRequired"
  );
  assert.throws(() =>
    validateMinimumVersionResponse(
      {
        ...request,
        product: "tutti-desktop"
      },
      request
    )
  );
});

test("enforces the foreground interval and one-prompt lifecycle", () => {
  assert.equal(
    shouldCheckMinimumVersionAfterForeground({
      disposed: false,
      foregroundPrompted: false,
      lastCheckAt: 0,
      now: 30 * 60 * 1_000,
      packaged: true,
      startupBlocked: false
    }),
    true
  );
  assert.equal(
    shouldCheckMinimumVersionAfterForeground({
      disposed: false,
      foregroundPrompted: true,
      lastCheckAt: 0,
      now: 60 * 60 * 1_000,
      packaged: true,
      startupBlocked: false
    }),
    false
  );
});
