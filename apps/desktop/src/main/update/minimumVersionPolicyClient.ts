import { app } from "electron";
import type { MinimumVersionCheckRequest } from "@tutti-os/desktop-update-admission/contracts";
import { outboundFetch } from "../net/outboundFetch.ts";

const productionControlPlaneBaseUrl = "https://tutti.sh/api/desktop/v1";

export async function checkTuttiMinimumVersion(
  request: MinimumVersionCheckRequest<"tutti-desktop">,
  signal: AbortSignal
): Promise<unknown> {
  const configuredDevelopmentBaseUrl =
    !app.isPackaged && process.env.TUTTI_DESKTOP_CONTROL_PLANE_BASE_URL
      ? process.env.TUTTI_DESKTOP_CONTROL_PLANE_BASE_URL
      : productionControlPlaneBaseUrl;
  const baseUrl = new URL(configuredDevelopmentBaseUrl);
  if (
    app.isPackaged &&
    (baseUrl.protocol !== "https:" ||
      baseUrl.origin !== new URL(productionControlPlaneBaseUrl).origin)
  ) {
    throw new Error("packaged minimum-version control plane origin is invalid");
  }
  const endpoint = `${baseUrl.toString().replace(/\/+$/u, "")}/public/desktop-version/check`;
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
}
