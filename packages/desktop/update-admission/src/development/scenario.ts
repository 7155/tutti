import type {
  DesktopUpdateAdmissionRuntime,
  DesktopUpdateChannel
} from "../contracts/index.ts";

export const desktopUpdateAdmissionDevelopmentEnvironment = {
  currentVersion: "DESKTOP_UPDATE_ADMISSION_CURRENT_VERSION",
  development: "DESKTOP_UPDATE_ADMISSION_DEV",
  download: "DESKTOP_UPDATE_ADMISSION_DOWNLOAD",
  foregroundIntervalMs: "DESKTOP_UPDATE_ADMISSION_FOREGROUND_INTERVAL_MS",
  install: "DESKTOP_UPDATE_ADMISSION_INSTALL",
  latestVersion: "DESKTOP_UPDATE_ADMISSION_LATEST_VERSION",
  minimumVersion: "DESKTOP_UPDATE_ADMISSION_MINIMUM_VERSION",
  mockServerUrl: "DESKTOP_UPDATE_ADMISSION_MOCK_SERVER_URL",
  policy: "DESKTOP_UPDATE_ADMISSION_POLICY",
  policySequence: "DESKTOP_UPDATE_ADMISSION_POLICY_SEQUENCE",
  scenario: "DESKTOP_UPDATE_ADMISSION_SCENARIO",
  transport: "DESKTOP_UPDATE_ADMISSION_TRANSPORT",
  updater: "DESKTOP_UPDATE_ADMISSION_UPDATER"
} as const;

export type DesktopUpdateDevelopmentPolicyOutcome =
  | "allowed"
  | "upgradeRequired"
  | "disabled"
  | "unsupported"
  | "unmanagedPrerelease"
  | "error"
  | "timeout";

export type DesktopUpdateDevelopmentPolicyStep =
  | {
      outcome: "allowed" | "upgradeRequired";
      minimumVersion: string;
      policySource: "defaultMinimum" | "platformOverride";
    }
  | {
      outcome: "disabled" | "unsupported" | "unmanagedPrerelease";
    }
  | {
      outcome: "error";
      message: string;
    }
  | {
      outcome: "timeout";
    };

export type DesktopUpdateDevelopmentUpdaterScenario =
  | {
      check: "available" | "downloaded" | "targetBelowMinimum";
      latestVersion: string;
      download: "success" | "error";
      install: "simulated" | "error";
    }
  | {
      check: "unavailable" | "error";
    };

export interface DesktopUpdateDevelopmentScenario {
  currentVersion: string;
  foregroundCheckIntervalMs: number;
  policySteps: readonly DesktopUpdateDevelopmentPolicyStep[];
  transport: "in-process" | "loopback";
  mockServerUrl: string | null;
  updater: DesktopUpdateDevelopmentUpdaterScenario;
}

export interface DesktopUpdateDevelopmentResolution {
  runtime: DesktopUpdateAdmissionRuntime;
  scenario: DesktopUpdateDevelopmentScenario | null;
}

const productionForegroundCheckIntervalMs = 30 * 60 * 1_000;
const strictSemVerPattern =
  /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$/u;
const managedStablePattern =
  /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/u;
const managedRcPattern =
  /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)-rc\.(0|[1-9]\d*)$/u;

type ManagedVersion = {
  channel: DesktopUpdateChannel;
  core: [string, string, string];
  rc: string | null;
};

function invalid(message: string): never {
  throw new Error(`invalid desktop update development scenario: ${message}`);
}

function validateStrictSemVer(value: string, name: string): string {
  const normalized = value.trim();
  const match = strictSemVerPattern.exec(normalized);
  if (!match) {
    return invalid(`${name} must be valid SemVer`);
  }
  const prerelease = match[4];
  if (prerelease) {
    for (const identifier of prerelease.split(".")) {
      if (
        /^\d+$/u.test(identifier) &&
        identifier.length > 1 &&
        identifier[0] === "0"
      ) {
        return invalid(`${name} must be valid SemVer`);
      }
    }
  }
  return normalized;
}

function parseManagedVersion(value: string): ManagedVersion | null {
  const stable = managedStablePattern.exec(value);
  if (stable) {
    return {
      channel: "stable",
      core: [stable[1]!, stable[2]!, stable[3]!],
      rc: null
    };
  }
  const rc = managedRcPattern.exec(value);
  if (!rc) {
    return null;
  }
  return {
    channel: "rc",
    core: [rc[1]!, rc[2]!, rc[3]!],
    rc: rc[4]!
  };
}

function compareNumericIdentifier(left: string, right: string): number {
  if (left.length !== right.length) {
    return left.length < right.length ? -1 : 1;
  }
  return left === right ? 0 : left < right ? -1 : 1;
}

function compareManagedVersions(
  left: ManagedVersion,
  right: ManagedVersion
): number {
  for (const index of [0, 1, 2] as const) {
    const compared = compareNumericIdentifier(
      left.core[index],
      right.core[index]
    );
    if (compared !== 0) {
      return compared;
    }
  }
  if (left.rc === right.rc) {
    return 0;
  }
  if (left.rc === null) {
    return 1;
  }
  if (right.rc === null) {
    return -1;
  }
  return compareNumericIdentifier(left.rc, right.rc);
}

function readRequired(
  env: Readonly<Record<string, string | undefined>>,
  name: string
): string {
  const value = env[name]?.trim();
  if (!value) {
    return invalid(`${name} is required`);
  }
  return value;
}

function readDevelopmentEnabled(
  env: Readonly<Record<string, string | undefined>>
): boolean {
  const name = desktopUpdateAdmissionDevelopmentEnvironment.development;
  const value = env[name]?.trim().toLowerCase();
  if (!value || ["0", "false", "no", "off"].includes(value)) {
    return false;
  }
  if (["1", "true", "yes", "on"].includes(value)) {
    return true;
  }
  return invalid(`${name} must be a boolean flag`);
}

function readForegroundInterval(
  env: Readonly<Record<string, string | undefined>>
): number {
  const name =
    desktopUpdateAdmissionDevelopmentEnvironment.foregroundIntervalMs;
  const raw = env[name]?.trim();
  if (!raw) {
    return productionForegroundCheckIntervalMs;
  }
  const value = Number(raw);
  if (!Number.isSafeInteger(value) || value < 100) {
    return invalid(`${name} must be an integer greater than or equal to 100`);
  }
  return value;
}

function parseOutcome(value: string): DesktopUpdateDevelopmentPolicyOutcome {
  switch (value) {
    case "allowed":
    case "upgradeRequired":
    case "disabled":
    case "unsupported":
    case "unmanagedPrerelease":
    case "error":
    case "timeout":
      return value;
    default:
      return invalid(`unknown policy outcome ${JSON.stringify(value)}`);
  }
}

function createPolicyStep(
  token: string,
  fallbackMinimumVersion: string | undefined
): DesktopUpdateDevelopmentPolicyStep {
  const [rawOutcome, rawMinimumVersion, ...extra] = token.split("@");
  if (!rawOutcome || extra.length > 0) {
    return invalid(`invalid policy step ${JSON.stringify(token)}`);
  }
  const outcome = parseOutcome(rawOutcome.trim());
  if (outcome === "allowed" || outcome === "upgradeRequired") {
    const minimumVersion = validateStrictSemVer(
      rawMinimumVersion?.trim() || fallbackMinimumVersion || "",
      "minimumVersion"
    );
    return {
      outcome,
      minimumVersion,
      policySource: "defaultMinimum"
    };
  }
  if (rawMinimumVersion !== undefined) {
    return invalid(
      `policy outcome ${outcome} must not include a minimum version`
    );
  }
  if (outcome === "error") {
    return {
      outcome,
      message: "Development minimum-version policy check failed"
    };
  }
  return { outcome };
}

function createPresetPolicySteps(
  name: string,
  currentVersion: string,
  minimumVersion: string | undefined
): readonly DesktopUpdateDevelopmentPolicyStep[] {
  const requiredMinimum = (): string =>
    validateStrictSemVer(
      minimumVersion || "",
      desktopUpdateAdmissionDevelopmentEnvironment.minimumVersion
    );
  switch (name) {
    case "startup-force-success":
    case "startup-updater-unavailable":
    case "startup-target-below-minimum":
    case "startup-download-error":
      return [
        {
          outcome: "upgradeRequired",
          minimumVersion: requiredMinimum(),
          policySource: "defaultMinimum"
        }
      ];
    case "startup-policy-timeout":
      return [{ outcome: "timeout" }];
    case "retry-policy-released":
      return [
        {
          outcome: "upgradeRequired",
          minimumVersion: requiredMinimum(),
          policySource: "defaultMinimum"
        },
        { outcome: "disabled" }
      ];
    case "foreground-upgrade-required":
      return [
        {
          outcome: "allowed",
          minimumVersion: currentVersion,
          policySource: "defaultMinimum"
        },
        {
          outcome: "upgradeRequired",
          minimumVersion: requiredMinimum(),
          policySource: "defaultMinimum"
        }
      ];
    default:
      return invalid(`unknown named scenario ${JSON.stringify(name)}`);
  }
}

function resolvePolicySteps(
  env: Readonly<Record<string, string | undefined>>,
  currentVersion: string
): readonly DesktopUpdateDevelopmentPolicyStep[] {
  const names = desktopUpdateAdmissionDevelopmentEnvironment;
  const fallbackMinimumVersion = env[names.minimumVersion]?.trim();
  const preset = env[names.scenario]?.trim();
  if (preset) {
    for (const conflictingName of [
      names.policy,
      names.policySequence,
      names.updater,
      names.download,
      names.install
    ]) {
      if (env[conflictingName]?.trim()) {
        return invalid(
          `${names.scenario} and ${conflictingName} are mutually exclusive`
        );
      }
    }
    return createPresetPolicySteps(
      preset,
      currentVersion,
      fallbackMinimumVersion
    );
  }
  const sequence = env[names.policySequence]?.trim();
  const single = env[names.policy]?.trim();
  if (sequence && single) {
    return invalid(
      `${names.policy} and ${names.policySequence} are mutually exclusive`
    );
  }
  const rawSteps = sequence
    ? sequence.split(",").map((value) => value.trim())
    : single
      ? [single]
      : invalid(`${names.policy} or ${names.policySequence} is required`);
  if (rawSteps.some((value) => value.length === 0)) {
    return invalid(`${names.policySequence} contains an empty step`);
  }
  return rawSteps.map((token) =>
    createPolicyStep(token, fallbackMinimumVersion)
  );
}

function resolveUpdater(
  env: Readonly<Record<string, string | undefined>>,
  preset: string | undefined
): DesktopUpdateDevelopmentUpdaterScenario {
  const names = desktopUpdateAdmissionDevelopmentEnvironment;
  const configured = env[names.updater]?.trim();
  const check =
    preset === "startup-updater-unavailable"
      ? "unavailable"
      : preset === "startup-target-below-minimum"
        ? "targetBelowMinimum"
        : configured || "available";
  if (check === "unavailable" || check === "error") {
    return { check };
  }
  if (
    check !== "available" &&
    check !== "downloaded" &&
    check !== "targetBelowMinimum"
  ) {
    return invalid(`unknown updater outcome ${JSON.stringify(check)}`);
  }
  const download =
    preset === "startup-download-error"
      ? "error"
      : env[names.download]?.trim() || "success";
  if (download !== "success" && download !== "error") {
    return invalid(`${names.download} must be success or error`);
  }
  const install = env[names.install]?.trim() || "simulated";
  if (install !== "simulated" && install !== "error") {
    return invalid(`${names.install} must be simulated or error`);
  }
  return {
    check,
    latestVersion: validateStrictSemVer(
      readRequired(env, names.latestVersion),
      names.latestVersion
    ),
    download,
    install
  };
}

function validateLoopbackUrl(raw: string): string {
  let parsed: URL;
  try {
    parsed = new URL(raw);
  } catch {
    return invalid("mock server URL must be a valid URL");
  }
  if (
    parsed.protocol !== "http:" ||
    parsed.hostname !== "127.0.0.1" ||
    parsed.username ||
    parsed.password ||
    parsed.search ||
    parsed.hash ||
    (parsed.pathname !== "" && parsed.pathname !== "/")
  ) {
    return invalid("mock server URL must be an http://127.0.0.1 origin");
  }
  return parsed.origin;
}

function validateScenario(scenario: DesktopUpdateDevelopmentScenario): void {
  const current = parseManagedVersion(scenario.currentVersion);
  for (const step of scenario.policySteps) {
    if (step.outcome === "unmanagedPrerelease") {
      if (current) {
        invalid("unmanagedPrerelease requires an unmanaged currentVersion");
      }
      continue;
    }
    if (step.outcome !== "allowed" && step.outcome !== "upgradeRequired") {
      if (
        (step.outcome === "disabled" || step.outcome === "unsupported") &&
        !current
      ) {
        invalid(`${step.outcome} requires a managed currentVersion`);
      }
      continue;
    }
    if (!current) {
      invalid(`${step.outcome} requires a managed currentVersion`);
    }
    const minimum = parseManagedVersion(step.minimumVersion);
    if (!minimum || minimum.channel !== current.channel) {
      invalid(`minimumVersion must use the ${current.channel} channel`);
    }
    const compared = compareManagedVersions(current, minimum);
    if (step.outcome === "allowed" && compared < 0) {
      invalid("allowed requires currentVersion to meet minimumVersion");
    }
    if (step.outcome === "upgradeRequired" && compared >= 0) {
      invalid("upgradeRequired requires currentVersion below minimumVersion");
    }
  }

  const updater = scenario.updater;
  if (!("latestVersion" in updater)) {
    return;
  }
  if (!current) {
    invalid("an available updater requires a managed currentVersion");
  }
  const latest = parseManagedVersion(updater.latestVersion);
  if (!latest || latest.channel !== current.channel) {
    invalid(`latestVersion must use the ${current.channel} channel`);
  }
  if (compareManagedVersions(latest, current) <= 0) {
    invalid("an available updater requires latestVersion above currentVersion");
  }
  const requiredSteps = scenario.policySteps.filter(
    (
      step
    ): step is DesktopUpdateDevelopmentPolicyStep & {
      outcome: "upgradeRequired";
      minimumVersion: string;
    } => step.outcome === "upgradeRequired"
  );
  if (updater.check === "targetBelowMinimum") {
    const target = requiredSteps[0];
    if (!target) {
      invalid("targetBelowMinimum requires an upgradeRequired policy step");
    }
    const minimum = parseManagedVersion(target.minimumVersion)!;
    if (compareManagedVersions(latest, minimum) >= 0) {
      invalid("targetBelowMinimum requires latestVersion below minimumVersion");
    }
    return;
  }
  for (const step of requiredSteps) {
    const minimum = parseManagedVersion(step.minimumVersion)!;
    if (compareManagedVersions(latest, minimum) < 0) {
      invalid(
        "available latestVersion must satisfy every upgradeRequired step"
      );
    }
  }
}

export function resolveDesktopUpdateDevelopmentScenario(input: {
  env: Readonly<Record<string, string | undefined>>;
  isPackaged: boolean;
}): DesktopUpdateDevelopmentScenario | null {
  if (input.isPackaged || !readDevelopmentEnabled(input.env)) {
    return null;
  }
  const names = desktopUpdateAdmissionDevelopmentEnvironment;
  const currentVersion = validateStrictSemVer(
    readRequired(input.env, names.currentVersion),
    names.currentVersion
  );
  const preset = input.env[names.scenario]?.trim();
  const transport = input.env[names.transport]?.trim() || "in-process";
  if (transport !== "in-process" && transport !== "loopback") {
    return invalid(`${names.transport} must be in-process or loopback`);
  }
  const mockServerUrl =
    transport === "loopback"
      ? validateLoopbackUrl(readRequired(input.env, names.mockServerUrl))
      : null;
  const scenario: DesktopUpdateDevelopmentScenario = {
    currentVersion,
    foregroundCheckIntervalMs: readForegroundInterval(input.env),
    mockServerUrl,
    policySteps: resolvePolicySteps(input.env, currentVersion),
    transport,
    updater: resolveUpdater(input.env, preset)
  };
  validateScenario(scenario);
  return Object.freeze({
    ...scenario,
    policySteps: Object.freeze(
      scenario.policySteps.map((step) => Object.freeze({ ...step }))
    ),
    updater: Object.freeze({ ...scenario.updater })
  });
}

export function resolveDesktopUpdateAdmissionDevelopment(input: {
  applicationVersion: string;
  env: Readonly<Record<string, string | undefined>>;
  isPackaged: boolean;
}): DesktopUpdateDevelopmentResolution {
  const applicationVersion = input.applicationVersion.trim();
  if (!applicationVersion) {
    throw new Error("desktop application version is required");
  }
  const scenario = resolveDesktopUpdateDevelopmentScenario(input);
  if (input.isPackaged) {
    return {
      runtime: {
        checksEnabled: true,
        currentVersion: applicationVersion,
        development: false,
        foregroundCheckIntervalMs: productionForegroundCheckIntervalMs
      },
      scenario: null
    };
  }
  if (!scenario) {
    return {
      runtime: {
        checksEnabled: false,
        currentVersion: applicationVersion,
        development: false,
        foregroundCheckIntervalMs: productionForegroundCheckIntervalMs
      },
      scenario: null
    };
  }
  return {
    runtime: {
      checksEnabled: true,
      currentVersion: scenario.currentVersion,
      development: true,
      foregroundCheckIntervalMs: scenario.foregroundCheckIntervalMs
    },
    scenario
  };
}
