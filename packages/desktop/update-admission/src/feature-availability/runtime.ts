import { mkdir, readFile, rename, rm, writeFile } from "node:fs/promises";
import { dirname } from "node:path";
import type {
  DesktopFeatureAvailabilityRuntime,
  DesktopFeatureAvailabilitySnapshot,
  DesktopProduct,
  MinimumVersionCheckRequest
} from "../contracts/index.ts";
import {
  desktopFeatureAvailabilityIdentityMatches,
  isDesktopFeatureSupported,
  normalizeDesktopFeatureKeys,
  parseDesktopFeatureAvailability
} from "./core.ts";

const cacheSchemaVersion = "tutti.desktop-feature-availability-cache.v1";

export interface DesktopFeatureAvailabilityLogger {
  info(message: string): void;
  error(message: string): void;
}

export interface MutableDesktopFeatureAvailabilityRuntime<
  TProduct extends DesktopProduct = DesktopProduct
> extends DesktopFeatureAvailabilityRuntime<TProduct> {
  acceptRemoteResponse(response: unknown, policyRevision: string): void;
  dispose(): Promise<void>;
}

interface CacheDocument {
  schemaVersion: typeof cacheSchemaVersion;
  product: DesktopProduct;
  platform: MinimumVersionCheckRequest["platform"];
  architecture: MinimumVersionCheckRequest["architecture"];
  currentVersion: string;
  policyRevision: string;
  fetchedAt: string;
  keys: readonly string[];
}

function log(
  logger: DesktopFeatureAvailabilityLogger,
  level: "info" | "error",
  details: Record<string, unknown>
): void {
  logger[level](`[desktop-feature-availability] ${JSON.stringify(details)}`);
}

function emptySnapshot<TProduct extends DesktopProduct>(
  identity: MinimumVersionCheckRequest<TProduct>
): DesktopFeatureAvailabilitySnapshot<TProduct> {
  return Object.freeze({
    ...identity,
    fetchedAt: null,
    keys: Object.freeze([]),
    policyRevision: null,
    source: "empty"
  });
}

function parseCacheDocument<TProduct extends DesktopProduct>(
  raw: string,
  identity: MinimumVersionCheckRequest<TProduct>
): DesktopFeatureAvailabilitySnapshot<TProduct> | null {
  const value: unknown = JSON.parse(raw);
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new Error("feature availability cache must be an object");
  }
  const record = value as Record<string, unknown>;
  if (
    record.schemaVersion !== cacheSchemaVersion ||
    typeof record.product !== "string" ||
    typeof record.platform !== "string" ||
    typeof record.architecture !== "string" ||
    typeof record.currentVersion !== "string" ||
    typeof record.policyRevision !== "string" ||
    !record.policyRevision.trim() ||
    typeof record.fetchedAt !== "string" ||
    !record.fetchedAt.trim()
  ) {
    throw new Error("feature availability cache has invalid metadata");
  }
  const snapshot: DesktopFeatureAvailabilitySnapshot = {
    architecture:
      record.architecture as MinimumVersionCheckRequest["architecture"],
    currentVersion: record.currentVersion,
    fetchedAt: record.fetchedAt,
    keys: normalizeDesktopFeatureKeys(record.keys),
    platform: record.platform as MinimumVersionCheckRequest["platform"],
    policyRevision: record.policyRevision,
    product: record.product as DesktopProduct,
    source: "cache"
  };
  if (!desktopFeatureAvailabilityIdentityMatches(snapshot, identity)) {
    return null;
  }
  return Object.freeze(snapshot);
}

async function loadCache<TProduct extends DesktopProduct>(
  cacheFilePath: string,
  identity: MinimumVersionCheckRequest<TProduct>,
  logger: DesktopFeatureAvailabilityLogger
): Promise<DesktopFeatureAvailabilitySnapshot<TProduct>> {
  try {
    const raw = await readFile(cacheFilePath, "utf8");
    const cached = parseCacheDocument(raw, identity);
    if (!cached) {
      log(logger, "info", {
        result: "ignored",
        stage: "cache-read",
        reason: "identityMismatch"
      });
      return emptySnapshot(identity);
    }
    log(logger, "info", {
      count: cached.keys.length,
      policyRevision: cached.policyRevision,
      result: "success",
      stage: "cache-read"
    });
    return cached;
  } catch (error) {
    const code =
      error && typeof error === "object" && "code" in error
        ? String(error.code)
        : "";
    if (code !== "ENOENT") {
      log(logger, "error", {
        error: error instanceof Error ? error.message : String(error),
        result: "failure",
        stage: "cache-read"
      });
    }
    return emptySnapshot(identity);
  }
}

async function writeCache(
  cacheFilePath: string,
  snapshot: DesktopFeatureAvailabilitySnapshot
): Promise<void> {
  await mkdir(dirname(cacheFilePath), { recursive: true });
  const temporaryPath = `${cacheFilePath}.${process.pid}.${Date.now()}.tmp`;
  const document: CacheDocument = {
    architecture: snapshot.architecture,
    currentVersion: snapshot.currentVersion,
    fetchedAt: snapshot.fetchedAt!,
    keys: snapshot.keys,
    platform: snapshot.platform,
    policyRevision: snapshot.policyRevision!,
    product: snapshot.product,
    schemaVersion: cacheSchemaVersion
  };
  try {
    await writeFile(temporaryPath, `${JSON.stringify(document)}\n`, {
      encoding: "utf8",
      mode: 0o600
    });
    await rename(temporaryPath, cacheFilePath);
  } catch (error) {
    await rm(temporaryPath, { force: true }).catch(() => undefined);
    throw error;
  }
}

export async function createDesktopFeatureAvailabilityRuntime<
  TProduct extends DesktopProduct
>(input: {
  cacheFilePath: string;
  identity: MinimumVersionCheckRequest<TProduct>;
  logger: DesktopFeatureAvailabilityLogger;
  now?: () => Date;
}): Promise<MutableDesktopFeatureAvailabilityRuntime<TProduct>> {
  let snapshot = await loadCache(
    input.cacheFilePath,
    input.identity,
    input.logger
  );
  const listeners = new Set<
    (value: DesktopFeatureAvailabilitySnapshot<TProduct>) => void
  >();
  let pendingWrite: Promise<void> = Promise.resolve();
  let disposed = false;

  return {
    acceptRemoteResponse(response, policyRevision) {
      if (disposed) {
        return;
      }
      const availability = parseDesktopFeatureAvailability(response);
      if (!availability) {
        log(input.logger, "info", {
          result: "retained",
          stage: "remote-response",
          reason: "featureAvailabilityMissing"
        });
        return;
      }
      const next: DesktopFeatureAvailabilitySnapshot<TProduct> = Object.freeze({
        ...input.identity,
        fetchedAt: (input.now?.() ?? new Date()).toISOString(),
        keys: availability.keys,
        policyRevision,
        source: "remote"
      });
      snapshot = next;
      for (const listener of listeners) {
        try {
          listener(next);
        } catch (error) {
          log(input.logger, "error", {
            error: error instanceof Error ? error.message : String(error),
            result: "failure",
            stage: "subscriber-notify"
          });
        }
      }
      pendingWrite = pendingWrite
        .then(() => writeCache(input.cacheFilePath, next))
        .then(() => {
          log(input.logger, "info", {
            count: next.keys.length,
            policyRevision,
            result: "success",
            stage: "cache-write"
          });
        })
        .catch((error) => {
          log(input.logger, "error", {
            error: error instanceof Error ? error.message : String(error),
            result: "failure",
            stage: "cache-write"
          });
        });
    },
    async dispose() {
      disposed = true;
      listeners.clear();
      await pendingWrite;
    },
    getSnapshot() {
      return snapshot;
    },
    isSupported(key) {
      return isDesktopFeatureSupported(snapshot, key);
    },
    subscribe(listener) {
      if (disposed) {
        return () => undefined;
      }
      listeners.add(listener);
      return () => listeners.delete(listener);
    }
  };
}
