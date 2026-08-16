import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { addTuttiDesktopClientToConnectorAuthorizationUrl } from "./connectorAuthorizationClientUrl.ts";

describe("addTuttiDesktopClientToConnectorAuthorizationUrl", () => {
  it("marks the server-owned authorization bridge URL", () => {
    assert.equal(
      addTuttiDesktopClientToConnectorAuthorizationUrl(
        "https://tutti.sh/connector-authorization/start/nonce?existing=value#step"
      ),
      "https://tutti.sh/connector-authorization/start/nonce?existing=value&desktop_client=tutti#step"
    );
  });

  it("does not modify provider or invalid URLs", () => {
    assert.equal(
      addTuttiDesktopClientToConnectorAuthorizationUrl(
        "https://accounts.google.com/o/oauth2/auth"
      ),
      "https://accounts.google.com/o/oauth2/auth"
    );
    assert.equal(
      addTuttiDesktopClientToConnectorAuthorizationUrl("not-a-url"),
      "not-a-url"
    );
  });
});
