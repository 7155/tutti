import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { tmpdir } from "node:os";
import test from "node:test";
import { createDesktopFeatureAvailabilityRuntime } from "./runtime.ts";

const identity = {
  architecture: "arm64",
  currentVersion: "1.2.3",
  platform: "macos",
  product: "tutti-desktop"
} as const;

const logger = {
  error() {},
  info() {}
};

test("persists remote keys and restores an exact-identity cache", async () => {
  const directory = await mkdtemp(join(tmpdir(), "feature-availability-"));
  const cacheFilePath = join(directory, "cache.json");
  try {
    const runtime = await createDesktopFeatureAvailabilityRuntime({
      cacheFilePath,
      identity,
      logger,
      now: () => new Date("2026-08-02T00:00:00.000Z")
    });
    runtime.acceptRemoteResponse(
      {
        featureAvailability: {
          keys: ["workspace.example", "agent.preview"]
        }
      },
      "revision-1"
    );
    assert.equal(runtime.isSupported("agent.preview"), true);
    assert.equal(runtime.getSnapshot().source, "remote");
    await runtime.dispose();

    const restored = await createDesktopFeatureAvailabilityRuntime({
      cacheFilePath,
      identity,
      logger
    });
    assert.equal(restored.getSnapshot().source, "cache");
    assert.deepEqual(restored.getSnapshot().keys, [
      "agent.preview",
      "workspace.example"
    ]);
    await restored.dispose();
  } finally {
    await rm(directory, { force: true, recursive: true });
  }
});

test("does not reuse a cache across application versions", async () => {
  const directory = await mkdtemp(join(tmpdir(), "feature-availability-"));
  const cacheFilePath = join(directory, "cache.json");
  try {
    const runtime = await createDesktopFeatureAvailabilityRuntime({
      cacheFilePath,
      identity,
      logger
    });
    runtime.acceptRemoteResponse(
      { featureAvailability: { keys: ["workspace.example"] } },
      "revision-1"
    );
    await runtime.dispose();

    const upgraded = await createDesktopFeatureAvailabilityRuntime({
      cacheFilePath,
      identity: { ...identity, currentVersion: "1.2.4" },
      logger
    });
    assert.equal(upgraded.getSnapshot().source, "empty");
    assert.equal(upgraded.isSupported("workspace.example"), false);
    await upgraded.dispose();
  } finally {
    await rm(directory, { force: true, recursive: true });
  }
});

test("retains cache for a missing envelope and overwrites it for empty keys", async () => {
  const directory = await mkdtemp(join(tmpdir(), "feature-availability-"));
  const cacheFilePath = join(directory, "cache.json");
  try {
    const runtime = await createDesktopFeatureAvailabilityRuntime({
      cacheFilePath,
      identity,
      logger
    });
    runtime.acceptRemoteResponse(
      { featureAvailability: { keys: ["workspace.example"] } },
      "revision-1"
    );
    runtime.acceptRemoteResponse({}, "revision-2");
    assert.equal(runtime.isSupported("workspace.example"), true);
    runtime.acceptRemoteResponse(
      { featureAvailability: { keys: [] } },
      "revision-3"
    );
    assert.equal(runtime.isSupported("workspace.example"), false);
    await runtime.dispose();
    const persisted = JSON.parse(await readFile(cacheFilePath, "utf8")) as {
      keys: string[];
      policyRevision: string;
    };
    assert.deepEqual(persisted.keys, []);
    assert.equal(persisted.policyRevision, "revision-3");
  } finally {
    await rm(directory, { force: true, recursive: true });
  }
});

test("ignores a corrupt cache and fails closed", async () => {
  const directory = await mkdtemp(join(tmpdir(), "feature-availability-"));
  const cacheFilePath = join(directory, "cache.json");
  try {
    await writeFile(cacheFilePath, "{not-json", "utf8");
    const runtime = await createDesktopFeatureAvailabilityRuntime({
      cacheFilePath,
      identity,
      logger
    });
    assert.equal(runtime.getSnapshot().source, "empty");
    assert.deepEqual(runtime.getSnapshot().keys, []);
    await runtime.dispose();
  } finally {
    await rm(directory, { force: true, recursive: true });
  }
});

test("persists and notifies remaining subscribers when one subscriber throws", async () => {
  const directory = await mkdtemp(join(tmpdir(), "feature-availability-"));
  const cacheFilePath = join(directory, "cache.json");
  try {
    const runtime = await createDesktopFeatureAvailabilityRuntime({
      cacheFilePath,
      identity,
      logger
    });
    let notified = false;
    runtime.subscribe(() => {
      throw new Error("subscriber failed");
    });
    runtime.subscribe(() => {
      notified = true;
    });

    runtime.acceptRemoteResponse(
      { featureAvailability: { keys: ["workspace.example"] } },
      "revision-1"
    );
    await runtime.dispose();

    assert.equal(notified, true);
    const persisted = JSON.parse(await readFile(cacheFilePath, "utf8")) as {
      keys: string[];
    };
    assert.deepEqual(persisted.keys, ["workspace.example"]);
  } finally {
    await rm(directory, { force: true, recursive: true });
  }
});

test("keeps the remote in-memory snapshot when the atomic cache write fails", async () => {
  const directory = await mkdtemp(join(tmpdir(), "feature-availability-"));
  const blockingPath = join(directory, "not-a-directory");
  const cacheFilePath = join(blockingPath, "cache.json");
  try {
    await writeFile(blockingPath, "file", "utf8");
    const runtime = await createDesktopFeatureAvailabilityRuntime({
      cacheFilePath,
      identity,
      logger
    });
    runtime.acceptRemoteResponse(
      { featureAvailability: { keys: ["workspace.example"] } },
      "revision-1"
    );
    await runtime.dispose();
    assert.equal(runtime.getSnapshot().source, "remote");
    assert.equal(runtime.isSupported("workspace.example"), true);
  } finally {
    await rm(directory, { force: true, recursive: true });
  }
});

test("restores an exact-identity cache without a time expiry", async () => {
  const directory = await mkdtemp(join(tmpdir(), "feature-availability-"));
  const cacheFilePath = join(directory, "cache.json");
  try {
    await writeFile(
      cacheFilePath,
      JSON.stringify({
        ...identity,
        fetchedAt: "2000-01-01T00:00:00.000Z",
        keys: ["workspace.example"],
        policyRevision: "revision-old",
        schemaVersion: "tutti.desktop-feature-availability-cache.v1"
      }),
      "utf8"
    );
    const runtime = await createDesktopFeatureAvailabilityRuntime({
      cacheFilePath,
      identity,
      logger
    });
    assert.equal(runtime.getSnapshot().source, "cache");
    assert.equal(runtime.isSupported("workspace.example"), true);
    await runtime.dispose();
  } finally {
    await rm(directory, { force: true, recursive: true });
  }
});
