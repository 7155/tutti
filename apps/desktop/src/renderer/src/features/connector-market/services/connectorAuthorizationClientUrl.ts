const connectorAuthorizationStartPathPrefix = "/connector-authorization/start/";
const desktopClientQuery = "desktop_client";

export function addTuttiDesktopClientToConnectorAuthorizationUrl(
  rawUrl: string
): string {
  try {
    const url = new URL(rawUrl);
    if (
      (url.protocol !== "https:" && url.protocol !== "http:") ||
      !url.pathname.startsWith(connectorAuthorizationStartPathPrefix)
    ) {
      return rawUrl;
    }
    url.searchParams.set(desktopClientQuery, "tutti");
    return url.toString();
  } catch {
    return rawUrl;
  }
}
