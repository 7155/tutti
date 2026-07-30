import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { hostname, tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import {
  getElectronCacheLockPath,
  getElectronCacheRoot,
  withElectronCacheLock
} from "./prewarm-electron.mjs";

test("resolves Electron's platform cache locations", () => {
  assert.equal(
    getElectronCacheRoot({
      env: {},
      homeDirectory: "/Users/developer",
      platform: "darwin"
    }),
    "/Users/developer/Library/Caches/electron"
  );
  assert.equal(
    getElectronCacheRoot({
      env: { XDG_CACHE_HOME: "/cache" },
      homeDirectory: "/home/developer",
      platform: "linux"
    }),
    "/cache/electron"
  );
  assert.equal(
    getElectronCacheRoot({
      env: { electron_config_cache: "/custom/electron-cache" },
      platform: "darwin"
    }),
    "/custom/electron-cache"
  );
});

test("serializes prewarm calls for the same Electron artifact", async () => {
  const cacheRoot = mkdtempSync(join(tmpdir(), "tutti-electron-cache-"));
  const lockInput = {
    arch: "arm64",
    cacheRoot,
    electronVersion: "43.2.0",
    platform: "darwin",
    pollIntervalMilliseconds: 1
  };
  let releaseFirstOperation;
  let secondOperationStarted = false;
  let signalFirstOperation;
  const firstOperationStarted = new Promise((resolvePromise) => {
    signalFirstOperation = resolvePromise;
  });

  try {
    const first = withElectronCacheLock(lockInput, async () => {
      signalFirstOperation();
      await new Promise((resolvePromise) => {
        releaseFirstOperation = resolvePromise;
      });
    });
    await firstOperationStarted;

    const second = withElectronCacheLock(lockInput, async () => {
      secondOperationStarted = true;
    });
    await new Promise((resolvePromise) => setTimeout(resolvePromise, 20));
    assert.equal(secondOperationStarted, false);

    releaseFirstOperation();
    await Promise.all([first, second]);
    assert.equal(secondOperationStarted, true);
  } finally {
    rmSync(cacheRoot, { force: true, recursive: true });
  }
});

test("reclaims a prewarm lock left by a dead local process", async () => {
  const cacheRoot = mkdtempSync(join(tmpdir(), "tutti-electron-cache-"));
  const lockInput = {
    arch: "arm64",
    cacheRoot,
    electronVersion: "43.2.0",
    platform: "darwin",
    pollIntervalMilliseconds: 1
  };
  const lockPath = getElectronCacheLockPath(lockInput);
  mkdirSync(cacheRoot, { recursive: true });
  writeFileSync(
    lockPath,
    `${JSON.stringify({ hostName: hostname(), processId: 99_999_999 })}\n`
  );

  try {
    const result = await withElectronCacheLock(lockInput, () => "ready");
    assert.equal(result, "ready");
  } finally {
    rmSync(cacheRoot, { force: true, recursive: true });
  }
});
