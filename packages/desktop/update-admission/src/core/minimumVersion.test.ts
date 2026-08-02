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
  assert.deepEqual(
    updaterTargetMeetsMinimum(
      "900719925474099312345.0.0",
      "900719925474099312344.999.999"
    ),
    true
  );
});

test("combines a validated policy response with the local request identity", () => {
  const request = {
    architecture: "arm64",
    currentVersion: "1.6.0",
    platform: "macos",
    product: "tsh-desktop"
  } as const;
  assert.deepEqual(
    validateMinimumVersionResponse(
      {
        channel: "stable",
        decision: "upgradeRequired",
        minimumVersion: "1.6.1",
        policyRevision: "revision-1",
        reason: "belowMinimum"
      },
      request
    ),
    {
      ...request,
      channel: "stable",
      decision: "upgradeRequired",
      minimumVersion: "1.6.1",
      policyRevision: "revision-1",
      reason: "belowMinimum"
    }
  );
});

test("accepts an explicitly unconfigured minimum version", () => {
  const request = {
    architecture: "arm64",
    currentVersion: "1.6.0",
    platform: "macos",
    product: "tsh-desktop"
  } as const;
  const response = validateMinimumVersionResponse(
    {
      channel: "stable",
      decision: "allowed",
      policyRevision: "revision-1",
      reason: "minimumNotConfigured"
    },
    request
  );

  assert.equal(response.reason, "minimumNotConfigured");
  assert.equal("minimumVersion" in response, false);
});

test("enforces the foreground interval and one-prompt lifecycle", () => {
  assert.equal(
    shouldCheckMinimumVersionAfterForeground({
      checksEnabled: true,
      disposed: false,
      foregroundCheckIntervalMs: 30 * 60 * 1_000,
      foregroundPrompted: false,
      lastCheckAt: 0,
      now: 30 * 60 * 1_000,
      startupBlocked: false
    }),
    true
  );
  assert.equal(
    shouldCheckMinimumVersionAfterForeground({
      checksEnabled: true,
      disposed: false,
      foregroundCheckIntervalMs: 30 * 60 * 1_000,
      foregroundPrompted: true,
      lastCheckAt: 0,
      now: 60 * 60 * 1_000,
      startupBlocked: false
    }),
    false
  );
});
