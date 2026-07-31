import { describe, expect, it } from "vitest";
import type { AgentGUIProps } from "./AgentGUI";
import type { AgentActivityRuntime } from "./agentActivityRuntime";

type LegacyLifecycleRuntimeKey =
  | "activateSession"
  | "createSession"
  | "goalControl"
  | "sendInput"
  | "submitInteractive"
  | "unactivateSession"
  | "updateSessionSettings"
  | "updateTuttiModeActivation";

type LegacyDirectoryProp = Extract<
  keyof AgentGUIProps,
  "agents" | "agentsLoading"
>;
type InternalTargetCapability = Extract<
  keyof AgentGUIProps["hostCapabilities"],
  | "agentTargets"
  | "agentTargetsLoading"
  | "providerRailAllPresentation"
  | "providerRailMode"
>;
type InternalRailSlot = Extract<
  keyof AgentGUIProps["renderSlots"],
  "providerRailEmpty"
>;
type AgentGUILegacyLifecycleRuntimeKey = Extract<
  keyof AgentGUIProps["agentActivityRuntime"],
  LegacyLifecycleRuntimeKey
>;
type MissingCompatibilityLifecycleRuntimeKey = Exclude<
  LegacyLifecycleRuntimeKey,
  keyof AgentActivityRuntime
>;

const legacyDirectoryPropsAreNotPublic: Record<LegacyDirectoryProp, never> = {};
const internalTargetCapabilitiesAreNotPublic: Record<
  InternalTargetCapability,
  never
> = {};
const internalRailSlotsAreNotPublic: Record<InternalRailSlot, never> = {};
const agentGUILegacyLifecycleRuntimeKeysAreNotPublic: Record<
  AgentGUILegacyLifecycleRuntimeKey,
  never
> = {};
const compatibilityLifecycleRuntimeKeysRemainPublic: Record<
  MissingCompatibilityLifecycleRuntimeKey,
  never
> = {};

describe("AgentGUI public contract", () => {
  it("exposes one directory snapshot without writable normalized target seams", () => {
    expect(legacyDirectoryPropsAreNotPublic).toEqual({});
    expect(internalTargetCapabilitiesAreNotPublic).toEqual({});
    expect(internalRailSlotsAreNotPublic).toEqual({});
    expect(agentGUILegacyLifecycleRuntimeKeysAreNotPublic).toEqual({});
    expect(compatibilityLifecycleRuntimeKeysRemainPublic).toEqual({});
  });
});
