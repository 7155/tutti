import type { MinimumVersionCheckRequest } from "@tutti-os/desktop-update-admission/contracts";
import { outboundFetch } from "../net/outboundFetch.ts";

const productionControlPlaneBaseUrl = "https://tutti.sh/api/desktop/v1";

export function createTuttiMinimumVersionChecker(
  configuredBaseUrl = productionControlPlaneBaseUrl
): (
  request: MinimumVersionCheckRequest<"tutti-desktop">,
  signal: AbortSignal
) => Promise<unknown> {
  const baseUrl = new URL(configuredBaseUrl);
  const endpoint = `${baseUrl.toString().replace(/\/+$/u, "")}/public/desktop-version/check`;
  return async (request, signal) => {
    const response = await outboundFetch(endpoint, {
      body: JSON.stringify(request),
      headers: {
        Accept: "application/json",
        "Content-Type": "application/json",
        "User-Agent": "Tutti Desktop"
      },
      method: "POST",
      signal
    });
    if (!response.ok) {
      throw new Error(`minimum version check returned HTTP ${response.status}`);
    }
    return await response.json();
  };
}
