import type {
  AgentProviderProbeListInput,
  AgentProbeProvider
} from "@tutti-os/agent-gui";

import { resolveCodeBuddyBillingTarget } from "./codeBuddyProviderAccount.ts";

const CODEBUDDY_PROVIDER = "acp:codebuddy";

export async function probeCodeBuddyProvider(
  input: AgentProviderProbeListInput,
  capturedAtUnixMs: number,
  provider = CODEBUDDY_PROVIDER
): Promise<AgentProbeProvider> {
  try {
    const target = await resolveCodeBuddyBillingTarget();
    return {
      attempts: [{ strategy: "codebuddy-account-config", success: true }],
      availability: {
        detailsVisible: false,
        status: "available"
      },
      provider,
      usage: input.includeUsage
        ? {
            ...target,
            capturedAtUnixMs,
            quotas: []
          }
        : undefined
    };
  } catch (error) {
    const message = errorMessage(error);
    const code = message.toLowerCase().includes("expired")
      ? "session_expired"
      : "auth_required";
    return {
      attempts: [
        {
          errorCode: code,
          errorMessage: message,
          strategy: "codebuddy-account-config",
          success: false
        }
      ],
      availability: {
        detailsVisible: false,
        status: "unavailable"
      },
      lastError: { code, message },
      provider
    };
  }
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
