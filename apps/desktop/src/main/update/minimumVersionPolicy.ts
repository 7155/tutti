const foregroundIntervalMs = 30 * 60 * 1_000;

export function resolveMinimumVersionRuntimeTarget(
  platform: NodeJS.Platform,
  architecture: string
): {
  platform: "macos" | "windows" | "linux";
  architecture: "arm64" | "x64";
} | null {
  const normalizedPlatform =
    platform === "darwin"
      ? "macos"
      : platform === "win32"
        ? "windows"
        : platform === "linux"
          ? "linux"
          : null;
  const normalizedArchitecture =
    architecture === "arm64" ? "arm64" : architecture === "x64" ? "x64" : null;
  if (!normalizedPlatform || !normalizedArchitecture) {
    return null;
  }
  return { platform: normalizedPlatform, architecture: normalizedArchitecture };
}

type ManagedVersion = {
  major: string;
  minor: string;
  patch: string;
  rc: string | null;
};

function parseManagedVersion(value: string): ManagedVersion | null {
  const match =
    /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-rc\.(0|[1-9]\d*))?$/u.exec(
      value.trim()
    );
  if (!match) {
    return null;
  }
  return {
    major: match[1]!,
    minor: match[2]!,
    patch: match[3]!,
    rc: match[4] === undefined ? null : match[4]
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
  for (const key of ["major", "minor", "patch"] as const) {
    if (left[key] !== right[key]) {
      return compareNumericIdentifier(left[key], right[key]);
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

export function releaseMeetsMinimum(
  releaseVersion: string | null,
  minimumVersion: string
): boolean {
  if (!releaseVersion) {
    return false;
  }
  const release = parseManagedVersion(releaseVersion);
  const minimum = parseManagedVersion(minimumVersion);
  if (!release || !minimum) {
    return false;
  }
  return compareManagedVersions(release, minimum) >= 0;
}

export function shouldCheckMinimumVersionAfterForeground(input: {
  disposed: boolean;
  packaged: boolean;
  foregroundPrompted: boolean;
  startupBlocked: boolean;
  lastCheckAt: number;
  now: number;
}): boolean {
  return !(
    input.disposed ||
    !input.packaged ||
    input.foregroundPrompted ||
    input.startupBlocked ||
    input.now - input.lastCheckAt < foregroundIntervalMs
  );
}
