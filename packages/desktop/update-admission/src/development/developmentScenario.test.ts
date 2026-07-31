import assert from "node:assert/strict";
import test from "node:test";
import {
  completeDevelopmentUpdateInstallation,
  createDevelopmentAppUpdateDriver,
  DevelopmentInstallSuppressedError
} from "./updaterDriver.ts";
import { createDevelopmentMinimumVersionChecker } from "./policyChecker.ts";
import {
  resolveDesktopUpdateAdmissionDevelopment,
  resolveDesktopUpdateDevelopmentScenario,
  type DesktopUpdateDevelopmentScenario
} from "./scenario.ts";
import { startDesktopUpdateDevelopmentMockServer } from "./mockServer.ts";

const baseEnvironment = {
  DESKTOP_UPDATE_ADMISSION_CURRENT_VERSION: "1.0.0",
  DESKTOP_UPDATE_ADMISSION_DEV: "1",
  DESKTOP_UPDATE_ADMISSION_LATEST_VERSION: "1.2.0",
  DESKTOP_UPDATE_ADMISSION_MINIMUM_VERSION: "1.1.0",
  DESKTOP_UPDATE_ADMISSION_POLICY: "upgradeRequired",
  DESKTOP_UPDATE_ADMISSION_UPDATER: "available"
} as const;

test("packaged resolution ignores invalid development variables", () => {
  const result = resolveDesktopUpdateAdmissionDevelopment({
    applicationVersion: "2.0.0",
    env: {
      DESKTOP_UPDATE_ADMISSION_CURRENT_VERSION: "invalid",
      DESKTOP_UPDATE_ADMISSION_DEV: "1",
      DESKTOP_UPDATE_ADMISSION_POLICY: "invalid"
    },
    isPackaged: true
  });

  assert.equal(result.scenario, null);
  assert.deepEqual(result.runtime, {
    checksEnabled: true,
    currentVersion: "2.0.0",
    development: false,
    foregroundCheckIntervalMs: 30 * 60 * 1_000
  });
});

test("development resolution rejects contradictory version outcomes", () => {
  assert.throws(
    () =>
      resolveDesktopUpdateDevelopmentScenario({
        env: {
          ...baseEnvironment,
          DESKTOP_UPDATE_ADMISSION_MINIMUM_VERSION: "0.9.0"
        },
        isPackaged: false
      }),
    /upgradeRequired requires currentVersion below minimumVersion/
  );
});

test("development resolution rejects invalid SemVer and non-loopback transport", () => {
  assert.throws(
    () =>
      resolveDesktopUpdateDevelopmentScenario({
        env: {
          ...baseEnvironment,
          DESKTOP_UPDATE_ADMISSION_CURRENT_VERSION: "01.0.0"
        },
        isPackaged: false
      }),
    /must be valid SemVer/
  );
  assert.throws(
    () =>
      resolveDesktopUpdateDevelopmentScenario({
        env: {
          ...baseEnvironment,
          DESKTOP_UPDATE_ADMISSION_MOCK_SERVER_URL: "http://localhost:43210",
          DESKTOP_UPDATE_ADMISSION_TRANSPORT: "loopback"
        },
        isPackaged: false
      }),
    /must be an http:\/\/127\.0\.0\.1 origin/
  );
});

test("target-below-minimum scenario keeps updater and policy versions coherent", () => {
  const scenario = resolveDesktopUpdateDevelopmentScenario({
    env: {
      ...baseEnvironment,
      DESKTOP_UPDATE_ADMISSION_LATEST_VERSION: "1.1.0",
      DESKTOP_UPDATE_ADMISSION_MINIMUM_VERSION: "1.2.0",
      DESKTOP_UPDATE_ADMISSION_POLICY: undefined,
      DESKTOP_UPDATE_ADMISSION_SCENARIO: "startup-target-below-minimum",
      DESKTOP_UPDATE_ADMISSION_UPDATER: undefined
    },
    isPackaged: false
  });

  assert.equal(scenario?.updater.check, "targetBelowMinimum");
});

test("named scenarios reject individually configured outcomes", () => {
  assert.throws(
    () =>
      resolveDesktopUpdateDevelopmentScenario({
        env: {
          ...baseEnvironment,
          DESKTOP_UPDATE_ADMISSION_SCENARIO: "startup-force-success"
        },
        isPackaged: false
      }),
    /are mutually exclusive/
  );
});

test("policy sequences advance and keep their final response", async () => {
  const scenario = resolveDesktopUpdateDevelopmentScenario({
    env: {
      ...baseEnvironment,
      DESKTOP_UPDATE_ADMISSION_POLICY: undefined,
      DESKTOP_UPDATE_ADMISSION_POLICY_SEQUENCE: "upgradeRequired@1.1.0,disabled"
    },
    isPackaged: false
  })!;
  const checker = createDevelopmentMinimumVersionChecker(scenario);
  const request = {
    architecture: "arm64",
    currentVersion: "1.0.0",
    platform: "macos",
    product: "tutti-desktop"
  } as const;

  assert.equal(
    (await checker(request, new AbortController().signal)).decision,
    "upgradeRequired"
  );
  assert.equal(
    (await checker(request, new AbortController().signal)).reason,
    "productDisabled"
  );
  assert.equal(
    (await checker(request, new AbortController().signal)).reason,
    "productDisabled"
  );
});

test("policy sequences advance independently for each desktop product", async () => {
  const scenario = resolveDesktopUpdateDevelopmentScenario({
    env: {
      ...baseEnvironment,
      DESKTOP_UPDATE_ADMISSION_POLICY: undefined,
      DESKTOP_UPDATE_ADMISSION_POLICY_SEQUENCE: "upgradeRequired@1.1.0,disabled"
    },
    isPackaged: false
  })!;
  const checker = createDevelopmentMinimumVersionChecker(scenario);
  const request = {
    architecture: "arm64",
    currentVersion: "1.0.0",
    platform: "macos"
  } as const;

  assert.equal(
    (
      await checker(
        { ...request, product: "tsh-desktop" },
        new AbortController().signal
      )
    ).decision,
    "upgradeRequired"
  );
  assert.equal(
    (
      await checker(
        { ...request, product: "tutti-desktop" },
        new AbortController().signal
      )
    ).decision,
    "upgradeRequired"
  );
});

test("timeout policy remains pending until the caller aborts", async () => {
  const scenario = resolveDesktopUpdateDevelopmentScenario({
    env: {
      ...baseEnvironment,
      DESKTOP_UPDATE_ADMISSION_POLICY: "timeout"
    },
    isPackaged: false
  })!;
  const checker = createDevelopmentMinimumVersionChecker(scenario);
  const controller = new AbortController();
  const pending = checker(
    {
      architecture: "arm64",
      currentVersion: "1.0.0",
      platform: "macos",
      product: "tutti-desktop"
    },
    controller.signal
  );

  controller.abort();

  await assert.rejects(pending, { name: "AbortError" });
});

test("development updater emits a deterministic successful download", async () => {
  const scenario = createScenario();
  const driver = createDevelopmentAppUpdateDriver(scenario);
  const events: string[] = [];
  driver.onCheckingForUpdate(() => events.push("checking"));
  driver.onUpdateAvailable((info) => events.push(`available:${info.version}`));
  driver.onDownloadProgress((progress) =>
    events.push(`progress:${progress.percent}`)
  );
  driver.onUpdateDownloaded((info) =>
    events.push(`downloaded:${info.version}`)
  );

  await driver.checkForUpdates();
  await driver.downloadUpdate();

  assert.deepEqual(events, [
    "checking",
    "available:1.2.0",
    "progress:100",
    "downloaded:1.2.0"
  ]);
  assert.throws(
    () => completeDevelopmentUpdateInstallation(scenario),
    DevelopmentInstallSuppressedError
  );
});

test("loopback mock server serves the public desktop-version contract", async () => {
  const server = await startDesktopUpdateDevelopmentMockServer({
    scenario: createScenario()
  });
  try {
    const response = await fetch(
      `${server.baseUrl}/api/desktop/v1/public/desktop-version/check`,
      {
        body: JSON.stringify({
          architecture: "arm64",
          currentVersion: "1.0.0",
          platform: "macos",
          product: "tsh-desktop"
        }),
        headers: { "content-type": "application/json" },
        method: "POST"
      }
    );
    assert.equal(response.status, 200);
    assert.equal(
      ((await response.json()) as { decision: string }).decision,
      "upgradeRequired"
    );
    const invalidResponse = await fetch(
      `${server.baseUrl}/api/desktop/v1/public/desktop-version/check`,
      {
        body: "{",
        headers: { "content-type": "application/json" },
        method: "POST"
      }
    );
    assert.equal(invalidResponse.status, 400);
    assert.equal(new URL(server.baseUrl).hostname, "127.0.0.1");
  } finally {
    await server.close();
  }
});

function createScenario(): DesktopUpdateDevelopmentScenario {
  return {
    currentVersion: "1.0.0",
    foregroundCheckIntervalMs: 3_000,
    mockServerUrl: null,
    policySteps: [
      {
        minimumVersion: "1.1.0",
        outcome: "upgradeRequired",
        policySource: "defaultMinimum"
      }
    ],
    transport: "in-process",
    updater: {
      check: "available",
      download: "success",
      install: "simulated",
      latestVersion: "1.2.0"
    }
  };
}
