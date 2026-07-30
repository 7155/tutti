import { existsSync } from "node:fs";
import { readFile } from "node:fs/promises";
import { homedir } from "node:os";
import { join } from "node:path";
import type {
  AgentProviderProbeListInput,
  AgentProbeProvider,
  AgentUsageQuota
} from "@tutti-os/agent-gui";

import {
  kimiTokenStorageName,
  projectKimiConfig
} from "./kimiCodeConfigProjection.ts";
import { outboundFetch } from "./net/outboundFetch.ts";

const KIMI_CODE_PROVIDER = "acp:kimi-code";
const KIMI_MANAGED_PROVIDER = "managed:kimi-code";
const KIMI_DEFAULT_BASE_URL = "https://api.kimi.com/coding/v1";
const KIMI_HTTP_TIMEOUT_MS = 8_000;

interface KimiProviderMode {
  home: string;
  kind: "api-key" | "coding-plan";
  baseUrl: string;
  tokenStorageName: string;
}

interface KimiUsageRow {
  detail: Record<string, unknown>;
  item: Record<string, unknown>;
  window: Record<string, unknown>;
}

class KimiProbeError extends Error {
  readonly code: string;

  constructor(code: string, message: string) {
    super(message);
    this.name = "KimiProbeError";
    this.code = code;
  }
}

export async function probeKimiCodeProvider(
  input: AgentProviderProbeListInput,
  capturedAtUnixMs: number
): Promise<AgentProbeProvider> {
  const attempts: AgentProbeProvider["attempts"] = [];
  let mode: KimiProviderMode;
  try {
    mode = await resolveKimiProviderMode();
    attempts.push({
      strategy: `kimi-code-${mode.kind}-config`,
      success: true
    });
  } catch (error) {
    return failedKimiProbe(
      attempts,
      "kimi-code-config",
      kimiProbeErrorCode(error),
      errorMessage(error),
      false
    );
  }

  if (mode.kind === "api-key") {
    return {
      attempts,
      availability: availableKimiStatus(),
      provider: KIMI_CODE_PROVIDER,
      usage: input.includeUsage
        ? {
            accountTier: "API Usage Billing",
            capturedAtUnixMs,
            quotas: []
          }
        : undefined
    };
  }

  let accessToken: string;
  try {
    accessToken = await loadKimiAccessToken(mode.home, mode.tokenStorageName);
    attempts.push({ strategy: "kimi-code-oauth", success: true });
  } catch (error) {
    return failedKimiProbe(
      attempts,
      "kimi-code-oauth",
      kimiProbeErrorCode(error),
      errorMessage(error),
      false
    );
  }

  if (!input.includeUsage) {
    return {
      attempts,
      availability: availableKimiStatus(),
      provider: KIMI_CODE_PROVIDER
    };
  }

  try {
    const payload = await fetchKimiManagedUsage(mode.baseUrl, accessToken);
    attempts.push({ strategy: "kimi-code-managed-usage", success: true });
    const quotas = kimiManagedUsageQuotas(payload, capturedAtUnixMs);
    const exhausted = quotas.some(
      (quota) =>
        typeof quota.percentRemaining === "number" &&
        quota.percentRemaining <= 0
    );
    return {
      attempts,
      availability: availableKimiStatus(),
      ...(exhausted
        ? {
            lastError: {
              code: "quota_exhausted"
            }
          }
        : {}),
      provider: KIMI_CODE_PROVIDER,
      usage: {
        accountTier: "Coding Plan",
        capturedAtUnixMs,
        quotas
      }
    };
  } catch (error) {
    return failedKimiProbe(
      attempts,
      "kimi-code-managed-usage",
      kimiProbeErrorCode(error),
      errorMessage(error),
      true
    );
  }
}

function availableKimiStatus(): AgentProbeProvider["availability"] {
  return {
    checks: [{ name: "auth", passed: true }],
    detailsVisible: false,
    status: "available"
  };
}

function failedKimiProbe(
  attempts: NonNullable<AgentProbeProvider["attempts"]>,
  strategy: string,
  code: string,
  message: string,
  authWasResolved: boolean
): AgentProbeProvider {
  attempts.push({
    errorCode: code,
    errorMessage: message,
    strategy,
    success: false
  });
  return {
    attempts,
    availability: authWasResolved
      ? availableKimiStatus()
      : {
          checks: [{ detail: message, name: "auth", passed: false }],
          detailsVisible: true,
          status: "unavailable"
        },
    lastError: { code, message },
    provider: KIMI_CODE_PROVIDER
  };
}

async function resolveKimiProviderMode(): Promise<KimiProviderMode> {
  const home = kimiCodeHome();
  // Kimi's KIMI_MODEL_* overlay synthesizes an API-key provider and wins over
  // config.toml. A model without its paired key is not a usable configuration.
  if (stringValue(process.env.KIMI_MODEL_NAME)) {
    if (!stringValue(process.env.KIMI_MODEL_API_KEY)) {
      throw new KimiProbeError(
        "auth_required",
        "Kimi API-key mode is missing KIMI_MODEL_API_KEY."
      );
    }
    return apiKeyMode(home);
  }

  let content: string;
  try {
    content = await readFile(join(home, "config.toml"), "utf8");
  } catch {
    throw new KimiProbeError(
      "auth_required",
      "Kimi config.toml was not found."
    );
  }
  const config = projectKimiConfig(content);
  if (!config.defaultModel) {
    throw new KimiProbeError(
      "parse_failed",
      "Kimi config.toml has no default_model."
    );
  }
  const activeProvider = config.modelProviders.get(config.defaultModel);
  if (!activeProvider) {
    throw new KimiProbeError(
      "parse_failed",
      `Kimi default model "${config.defaultModel}" has no provider mapping.`
    );
  }
  if (activeProvider !== KIMI_MANAGED_PROVIDER) {
    if (!config.providersWithApiKeys.has(activeProvider)) {
      throw new KimiProbeError(
        "auth_required",
        `Kimi provider "${activeProvider}" has no API key.`
      );
    }
    return apiKeyMode(home);
  }

  const oauthKey =
    config.providerOAuthKeys.get(activeProvider) || "oauth/kimi-code";
  const tokenStorageName = kimiTokenStorageName(oauthKey);
  if (!tokenStorageName) {
    throw new KimiProbeError(
      "parse_failed",
      "Kimi OAuth credential key is invalid."
    );
  }
  return {
    baseUrl: (
      stringValue(process.env.KIMI_CODE_BASE_URL) ||
      config.providerBaseUrls.get(activeProvider) ||
      KIMI_DEFAULT_BASE_URL
    ).replace(/\/+$/u, ""),
    home,
    kind: "coding-plan",
    tokenStorageName
  };
}

function apiKeyMode(home: string): KimiProviderMode {
  return {
    baseUrl: "",
    home,
    kind: "api-key",
    tokenStorageName: ""
  };
}

async function loadKimiAccessToken(
  home: string,
  tokenStorageName: string
): Promise<string> {
  const path = join(home, "credentials", `${tokenStorageName}.json`);
  let parsed: Record<string, unknown>;
  try {
    parsed = JSON.parse(await readFile(path, "utf8")) as Record<
      string,
      unknown
    >;
  } catch {
    throw new KimiProbeError(
      "auth_required",
      "Kimi Coding Plan credentials were not found."
    );
  }
  const accessToken = stringValue(parsed.access_token);
  if (!accessToken) {
    throw new KimiProbeError(
      "session_expired",
      "Kimi Coding Plan credentials require sign-in."
    );
  }
  return accessToken;
}

async function fetchKimiManagedUsage(
  baseUrl: string,
  accessToken: string
): Promise<Record<string, unknown>> {
  const response = await outboundFetch(`${baseUrl}/usages`, {
    headers: {
      Accept: "application/json",
      Authorization: `Bearer ${accessToken}`,
      "User-Agent": "Tutti"
    },
    signal: AbortSignal.timeout(KIMI_HTTP_TIMEOUT_MS)
  });
  if (response.status === 401) {
    throw new KimiProbeError(
      "session_expired",
      "Kimi Coding Plan credentials are expired or unauthorized."
    );
  }
  if (response.status === 402) {
    throw new KimiProbeError(
      "subscription_required",
      "Kimi Coding Plan subscription is required."
    );
  }
  if (response.status === 403) {
    throw new KimiProbeError(
      "execution_failed",
      "Kimi Coding Plan usage request was denied."
    );
  }
  if (response.status === 429) {
    throw new KimiProbeError(
      "execution_failed",
      "Kimi Coding Plan usage API is rate limited."
    );
  }
  if (!response.ok) {
    throw new KimiProbeError(
      "execution_failed",
      `Kimi Coding Plan usage API returned HTTP ${response.status}.`
    );
  }
  return responseJson(response, "Kimi Coding Plan usage API");
}

async function responseJson(
  response: Response,
  label: string
): Promise<Record<string, unknown>> {
  const text = await response.text();
  try {
    const parsed = JSON.parse(text) as unknown;
    if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
      return parsed as Record<string, unknown>;
    }
  } catch {
    // Project one stable parse error below.
  }
  throw new KimiProbeError("parse_failed", `${label} returned invalid JSON.`);
}

function kimiManagedUsageQuotas(
  payload: Record<string, unknown>,
  capturedAtUnixMs: number
): AgentUsageQuota[] {
  const limits = Array.isArray(payload.limits) ? payload.limits : [];
  const quotas = limits
    .map((value) => kimiUsageRow(value))
    .filter((row): row is KimiUsageRow => row !== null)
    .map((row) => kimiUsageRowToQuota(row, capturedAtUnixMs))
    .filter((quota): quota is AgentUsageQuota => quota !== null);
  if (quotas.length > 0) return quotas;
  const summary = objectValue(payload.usage);
  if (!summary) return [];
  const quota = kimiUsageRowToQuota(
    { detail: summary, item: summary, window: {} },
    capturedAtUnixMs,
    "weekly"
  );
  return quota ? [quota] : [];
}

function kimiUsageRow(value: unknown): KimiUsageRow | null {
  const item = objectValue(value);
  if (!item) return null;
  return {
    detail: objectValue(item.detail) ?? item,
    item,
    window: objectValue(item.window) ?? {}
  };
}

function kimiUsageRowToQuota(
  row: KimiUsageRow,
  capturedAtUnixMs: number,
  fallbackType?: AgentUsageQuota["quotaType"]
): AgentUsageQuota | null {
  const limit = firstNumber(row.detail.limit, row.item.limit);
  const remaining = firstNumber(row.detail.remaining, row.item.remaining);
  const used = firstNumber(row.detail.used, row.item.used);
  if (limit === null || limit <= 0 || (remaining === null && used === null)) {
    return null;
  }
  const remainingValue = remaining ?? limit - (used ?? 0);
  const quota: AgentUsageQuota = {
    percentRemaining: Math.max(
      0,
      Math.min(100, Math.round((remainingValue / limit) * 100))
    ),
    quotaType: fallbackType ?? kimiQuotaType(row)
  };
  const resetAt = firstValue(
    row.detail.reset_at,
    row.detail.resetAt,
    row.item.reset_at,
    row.item.resetAt
  );
  const resetUnixMs = unixMsValue(resetAt);
  if (resetUnixMs !== null) {
    quota.resetsAtUnixMs = resetUnixMs;
  } else {
    const resetIn = firstNumber(
      row.detail.reset_in,
      row.detail.resetIn,
      row.item.reset_in,
      row.item.resetIn
    );
    if (resetIn !== null && resetIn > 0) {
      quota.resetsAtUnixMs = capturedAtUnixMs + resetIn * 1000;
    }
  }
  return quota;
}

function kimiQuotaType(row: KimiUsageRow): AgentUsageQuota["quotaType"] {
  const label = stringValue(
    firstValue(
      row.item.name,
      row.item.title,
      row.item.scope,
      row.detail.name,
      row.detail.title,
      row.detail.scope
    )
  ).toLowerCase();
  if (label.includes("month")) return "monthly";
  if (label.includes("week") || /\b7d\b/u.test(label)) return "weekly";
  if (label.includes("day") || /\b1d\b/u.test(label)) return "daily";
  if (label.includes("hour") || /\bh\b/u.test(label)) return "session";

  const duration = firstNumber(
    row.window.duration,
    row.item.duration,
    row.detail.duration
  );
  const unit = stringValue(
    firstValue(
      row.window.timeUnit,
      row.window.time_unit,
      row.item.timeUnit,
      row.item.time_unit,
      row.detail.timeUnit,
      row.detail.time_unit
    )
  ).toUpperCase();
  const seconds = durationSeconds(duration, unit);
  if (seconds !== null) {
    if (seconds >= 28 * 86_400) return "monthly";
    if (seconds >= 7 * 86_400) return "weekly";
    if (seconds >= 86_400) return "daily";
  }
  return "session";
}

function durationSeconds(duration: number | null, unit: string): number | null {
  if (duration === null || duration <= 0) return null;
  if (unit.includes("DAY")) return duration * 86_400;
  if (unit.includes("HOUR")) return duration * 3_600;
  if (unit.includes("MINUTE")) return duration * 60;
  return duration;
}

function unixMsValue(value: unknown): number | null {
  if (typeof value === "string" && value.trim()) {
    const parsed = Date.parse(value);
    if (Number.isFinite(parsed)) return parsed;
  }
  const numeric = numberValue(value);
  if (numeric === null) return null;
  return numeric > 10_000_000_000 ? numeric : numeric * 1000;
}

function kimiCodeHome(): string {
  const explicitHome =
    stringValue(process.env.KIMI_CODE_HOME) ||
    stringValue(process.env.KIMI_SHARE_DIR);
  if (explicitHome) return explicitHome;

  const userHome = homedir();
  const standaloneHome = join(userHome, ".kimi-code");
  return existsSync(join(standaloneHome, "bin", "kimi"))
    ? standaloneHome
    : join(userHome, ".kimi");
}

function objectValue(value: unknown): Record<string, unknown> | null {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
}

function stringValue(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

function numberValue(value: unknown): number | null {
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value === "string" && value.trim()) {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : null;
  }
  return null;
}

function firstNumber(...values: unknown[]): number | null {
  for (const value of values) {
    const parsed = numberValue(value);
    if (parsed !== null) return parsed;
  }
  return null;
}

function firstValue(...values: unknown[]): unknown {
  return values.find((value) => value !== undefined && value !== null);
}

function kimiProbeErrorCode(error: unknown): string {
  if (error instanceof KimiProbeError) return error.code;
  if (
    error instanceof Error &&
    (error.name === "AbortError" || error.name === "TimeoutError")
  ) {
    return "timeout";
  }
  return "execution_failed";
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
