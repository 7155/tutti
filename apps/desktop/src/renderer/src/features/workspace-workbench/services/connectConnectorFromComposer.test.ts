import assert from "node:assert/strict";
import test from "node:test";
import type { ConnectorMarketRootService } from "@tutti-os/connector-market/services";
import {
  connectConnectorFromComposer,
  openConnectorDialogFromComposer
} from "./connectConnectorFromComposer.ts";

test("composer connector open loads the market and opens its canonical dialog", async () => {
  const calls: string[] = [];
  const root = createRoot("install", calls);

  const result = await openConnectorDialogFromComposer(root, " github ");

  assert.equal(result, "dialog-opened");
  assert.deepEqual(calls, ["load", "open:github"]);
});

test("composer connector open ignores missing catalog entries", async () => {
  const calls: string[] = [];
  const root = createRoot(undefined, calls);

  const result = await openConnectorDialogFromComposer(root, "github");

  assert.equal(result, "no-action");
  assert.deepEqual(calls, ["load"]);
});

test("composer connector connect starts authorization for an installed connector", async () => {
  const calls: string[] = [];
  const root = createRoot("authorize", calls);

  const result = await connectConnectorFromComposer(root, " notion ");

  assert.equal(result, "authorization-started");
  assert.deepEqual(calls, ["load", "authorize:notion"]);
});

test("composer connector connect installs missing and outdated connectors", async () => {
  for (const action of ["install", "update"] as const) {
    const calls: string[] = [];
    const root = createRoot(action, calls);

    const result = await connectConnectorFromComposer(root, "github");

    assert.equal(result, "installation-started");
    assert.deepEqual(calls, ["load", "install:github"]);
  }
});

test("composer connector connect leaves non-actionable connector states unchanged", async () => {
  for (const action of [
    "busy",
    "disconnect",
    "manage",
    "unavailable",
    undefined
  ] as const) {
    const calls: string[] = [];
    const root = createRoot(action, calls);

    const result = await connectConnectorFromComposer(root, "github");

    assert.equal(result, "no-action");
    assert.deepEqual(calls, ["load"]);
  }
});

function createRoot(
  action:
    | "authorize"
    | "busy"
    | "disconnect"
    | "install"
    | "manage"
    | "unavailable"
    | "update"
    | undefined,
  calls: string[]
): ConnectorMarketRootService {
  return {
    market: {
      ensureLoaded: async () => {
        calls.push("load");
      },
      beginAuthorization: async (connectorKey: string) => {
        calls.push(`authorize:${connectorKey}`);
      },
      install: async (connectorKey: string) => {
        calls.push(`install:${connectorKey}`);
      }
    },
    view: {
      dataStore: {
        cardsByKey: action
          ? {
              github: { action },
              notion: { action }
            }
          : {}
      }
    },
    uiState: {
      openConnector: (connectorKey: string) => {
        calls.push(`open:${connectorKey}`);
      }
    }
  } as unknown as ConnectorMarketRootService;
}
