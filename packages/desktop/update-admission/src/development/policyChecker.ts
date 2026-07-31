import type {
  DesktopProduct,
  MinimumVersionCheckRequest,
  MinimumVersionCheckResponse
} from "../contracts/index.ts";
import { validateMinimumVersionResponse } from "../core/index.ts";
import type {
  DesktopUpdateDevelopmentPolicyStep,
  DesktopUpdateDevelopmentScenario
} from "./scenario.ts";

function developmentChannel(
  currentVersion: string
): "stable" | "rc" | "unmanaged" {
  if (
    /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)-rc\.(0|[1-9]\d*)$/u.test(
      currentVersion
    )
  ) {
    return "rc";
  }
  if (
    /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/u.test(
      currentVersion
    )
  ) {
    return "stable";
  }
  return "unmanaged";
}

function abortError(): Error {
  const error = new Error("development minimum-version policy check aborted");
  error.name = "AbortError";
  return error;
}

function waitForAbort(signal: AbortSignal): Promise<never> {
  if (signal.aborted) {
    return Promise.reject(abortError());
  }
  return new Promise<never>((_resolve, reject) => {
    signal.addEventListener("abort", () => reject(abortError()), {
      once: true
    });
  });
}

function responseForStep<TProduct extends DesktopProduct>(
  request: MinimumVersionCheckRequest<TProduct>,
  step: DesktopUpdateDevelopmentPolicyStep,
  revision: number
): MinimumVersionCheckResponse<TProduct> {
  const channel = developmentChannel(request.currentVersion);
  const base = {
    ...request,
    channel,
    minimumVersion: "",
    policyRevision: `development-policy-${revision}`,
    policySource: ""
  } as const;
  switch (step.outcome) {
    case "allowed":
      return validateMinimumVersionResponse(
        {
          ...base,
          channel,
          decision: "allowed",
          minimumVersion: step.minimumVersion,
          policySource: step.policySource,
          reason: "meetsMinimum"
        },
        request
      );
    case "upgradeRequired":
      return validateMinimumVersionResponse(
        {
          ...base,
          channel,
          decision: "upgradeRequired",
          minimumVersion: step.minimumVersion,
          policySource: step.policySource,
          reason: "belowMinimum"
        },
        request
      );
    case "disabled":
      return validateMinimumVersionResponse(
        {
          ...base,
          decision: "notApplicable",
          reason: "productDisabled"
        },
        request
      );
    case "unsupported":
      return validateMinimumVersionResponse(
        {
          ...base,
          decision: "notApplicable",
          reason: "unsupportedRelease"
        },
        request
      );
    case "unmanagedPrerelease":
      return validateMinimumVersionResponse(
        {
          ...base,
          channel: "unmanaged",
          decision: "notApplicable",
          reason: "unmanagedPrerelease"
        },
        request
      );
    case "error":
    case "timeout":
      throw new Error(`cannot synthesize ${step.outcome} as a policy response`);
  }
}

export function createDevelopmentMinimumVersionChecker(
  scenario: DesktopUpdateDevelopmentScenario
): <TProduct extends DesktopProduct>(
  request: MinimumVersionCheckRequest<TProduct>,
  signal: AbortSignal
) => Promise<MinimumVersionCheckResponse<TProduct>> {
  const checkIndexes = new Map<string, number>();
  return async <TProduct extends DesktopProduct>(
    request: MinimumVersionCheckRequest<TProduct>,
    signal: AbortSignal
  ): Promise<MinimumVersionCheckResponse<TProduct>> => {
    if (request.currentVersion !== scenario.currentVersion) {
      throw new Error(
        `development policy expected currentVersion ${scenario.currentVersion}, received ${request.currentVersion}`
      );
    }
    const requestIdentity = [
      request.product,
      request.platform,
      request.architecture
    ].join(":");
    const checkIndex = checkIndexes.get(requestIdentity) ?? 0;
    const stepIndex = Math.min(checkIndex, scenario.policySteps.length - 1);
    const step = scenario.policySteps[stepIndex]!;
    checkIndexes.set(requestIdentity, checkIndex + 1);
    if (step.outcome === "timeout") {
      return await waitForAbort(signal);
    }
    if (step.outcome === "error") {
      throw new Error(step.message);
    }
    return responseForStep(request, step, stepIndex + 1);
  };
}
