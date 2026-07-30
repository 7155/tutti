import type { MinimumVersionCheckResponse } from "../../shared/contracts/ipc.ts";

export interface MinimumVersionCheckRequest {
  product: "tutti-desktop";
  platform: "macos" | "windows" | "linux";
  architecture: "arm64" | "x64";
  currentVersion: string;
}

const managedReasons = {
  allowed: "meetsMinimum",
  upgradeRequired: "belowMinimum"
} as const;

export function validateMinimumVersionResponse(
  value: unknown,
  request: MinimumVersionCheckRequest
): MinimumVersionCheckResponse {
  if (!value || typeof value !== "object") {
    throw new Error("minimum version response must be an object");
  }
  const response = value as Record<string, unknown>;
  if (
    response.product !== request.product ||
    response.platform !== request.platform ||
    response.architecture !== request.architecture ||
    response.currentVersion !== request.currentVersion
  ) {
    throw new Error("minimum version response identity does not match request");
  }
  if (
    typeof response.policyRevision !== "string" ||
    response.policyRevision.trim() === ""
  ) {
    throw new Error("minimum version response has invalid policy revision");
  }
  if (response.channel === "unmanaged") {
    if (
      response.decision !== "notApplicable" ||
      response.reason !== "unmanagedPrerelease" ||
      response.minimumVersion !== "" ||
      response.policySource !== ""
    ) {
      throw new Error("minimum version response has invalid unmanaged policy");
    }
    return response as unknown as MinimumVersionCheckResponse;
  }
  if (response.channel !== "stable" && response.channel !== "rc") {
    throw new Error("minimum version response has invalid channel");
  }
  if (response.decision === "notApplicable") {
    if (
      !["productDisabled", "unsupportedRelease"].includes(
        String(response.reason)
      ) ||
      response.minimumVersion !== "" ||
      response.policySource !== ""
    ) {
      throw new Error(
        "minimum version response has invalid non-applicable policy"
      );
    }
    return response as unknown as MinimumVersionCheckResponse;
  }
  if (
    response.decision !== "allowed" &&
    response.decision !== "upgradeRequired"
  ) {
    throw new Error("minimum version response has invalid decision");
  }
  const pattern =
    response.channel === "rc"
      ? /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)-rc\.(0|[1-9]\d*)$/u
      : /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/u;
  if (
    typeof response.minimumVersion !== "string" ||
    !pattern.test(response.minimumVersion) ||
    !["defaultMinimum", "platformOverride"].includes(
      String(response.policySource)
    ) ||
    response.reason !== managedReasons[response.decision]
  ) {
    throw new Error("minimum version response has invalid managed policy");
  }
  return response as unknown as MinimumVersionCheckResponse;
}
