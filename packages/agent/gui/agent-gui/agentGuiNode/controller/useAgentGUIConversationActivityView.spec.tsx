import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import {
  AgentGUIRuntimeProvider,
  type AgentGUIRuntime
} from "../../../agentActivityRuntime";
import type { AgentGUIConversationSummary } from "../model/agentGuiConversationTypes";
import type { AgentGUIConversationActivityRootFact } from "./useAgentGUIConversationRailQuery";
import { useAgentGUIConversationActivityView } from "./useAgentGUIConversationActivityView";

describe("useAgentGUIConversationActivityView", () => {
  it("clears an active view when the host capability fails closed", () => {
    let capability = true;
    const runtime = {
      get conversationActivityViewEnabled() {
        return capability;
      },
      origin: "local"
    } as unknown as AgentGUIRuntime;
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <AgentGUIRuntimeProvider runtime={runtime}>
        {children}
      </AgentGUIRuntimeProvider>
    );
    const rendered = renderHook(
      () =>
        useAgentGUIConversationActivityView({
          conversations: [CONVERSATION],
          hasConversationQuery: false,
          rootFacts: new Map(),
          scopeKey: "workspace-1"
        }),
      { wrapper }
    );

    act(() => rendered.result.current.toggle());
    expect(rendered.result.current.enabled).toBe(true);

    capability = false;
    rendered.rerender();
    expect(rendered.result.current.enabled).toBe(false);
    expect(rendered.result.current.projection).toBeNull();

    capability = true;
    rendered.rerender();
    expect(rendered.result.current.enabled).toBe(false);
  });

  it("rebuilds instead of incrementally retaining idle sessions across identity changes", () => {
    const runtime = {
      conversationActivityViewEnabled: true,
      origin: "local"
    } as unknown as AgentGUIRuntime;
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <AgentGUIRuntimeProvider runtime={runtime}>
        {children}
      </AgentGUIRuntimeProvider>
    );
    const rendered = renderHook(
      ({ conversation, identityKey }) =>
        useAgentGUIConversationActivityView({
          conversations: [conversation],
          hasConversationQuery: false,
          identityKey,
          rootFacts: new Map(),
          scopeKey: "workspace-1"
        }),
      {
        initialProps: {
          conversation: CONVERSATION,
          identityKey: "user-1:codex"
        },
        wrapper
      }
    );

    act(() => rendered.result.current.toggle());
    expect(rendered.result.current.projection?.priorityIds).toEqual([
      "session-1"
    ]);

    rendered.rerender({
      conversation: {
        ...CONVERSATION,
        id: "session-2",
        status: "ready",
        title: "Session 2"
      },
      identityKey: "user-2:codex"
    });
    expect(rendered.result.current.projection?.priorityIds).toEqual([]);
    expect(rendered.result.current.projection?.recentSections[0]?.ids).toEqual([
      "session-2"
    ]);
  });

  it("keeps an existing Priority row while a rail refresh omits its summary", () => {
    const runtime = {
      conversationActivityViewEnabled: true,
      origin: "local"
    } as unknown as AgentGUIRuntime;
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <AgentGUIRuntimeProvider runtime={runtime}>
        {children}
      </AgentGUIRuntimeProvider>
    );
    const rendered = renderHook(
      ({ conversations }) =>
        useAgentGUIConversationActivityView({
          conversations,
          hasConversationQuery: false,
          rootFacts: new Map(),
          scopeKey: "workspace-1"
        }),
      {
        initialProps: { conversations: [CONVERSATION] },
        wrapper
      }
    );

    act(() => rendered.result.current.toggle());
    rendered.rerender({ conversations: [] });

    expect(rendered.result.current.projection?.priorityIds).toEqual([
      "session-1"
    ]);
    expect(rendered.result.current.conversationsById.get("session-1")).toBe(
      CONVERSATION
    );
  });

  it("does not render a cached Priority row after an Engine tombstone", () => {
    const runtime = {
      conversationActivityViewEnabled: true,
      origin: "local"
    } as unknown as AgentGUIRuntime;
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <AgentGUIRuntimeProvider runtime={runtime}>
        {children}
      </AgentGUIRuntimeProvider>
    );
    const rendered = renderHook(
      ({ conversations, deletedSessionIds }) =>
        useAgentGUIConversationActivityView({
          conversations,
          deletedSessionIds,
          hasConversationQuery: false,
          rootFacts: new Map(),
          scopeKey: "workspace-1"
        }),
      {
        initialProps: {
          conversations: [CONVERSATION],
          deletedSessionIds: {}
        },
        wrapper
      }
    );

    act(() => rendered.result.current.toggle());
    rendered.rerender({
      conversations: [],
      deletedSessionIds: { "session-1": true }
    });

    expect(rendered.result.current.projection?.priorityIds).toEqual([]);
    expect(rendered.result.current.conversationsById.has("session-1")).toBe(
      false
    );
  });

  it("preserves unchanged conversation objects when one root activity fact changes", () => {
    const runtime = {
      conversationActivityViewEnabled: true,
      origin: "local"
    } as unknown as AgentGUIRuntime;
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <AgentGUIRuntimeProvider runtime={runtime}>
        {children}
      </AgentGUIRuntimeProvider>
    );
    const conversations = [
      {
        ...CONVERSATION,
        id: "changed",
        status: "ready" as const,
        title: "Changed"
      },
      {
        ...CONVERSATION,
        id: "unchanged",
        status: "ready" as const,
        title: "Unchanged"
      }
    ];
    const initialRootFacts: ReadonlyMap<
      string,
      AgentGUIConversationActivityRootFact
    > = new Map([
      ["changed", { needsUserAction: false, status: "ready" }],
      ["unchanged", { needsUserAction: false, status: "ready" }]
    ]);
    const rendered = renderHook(
      ({ rootFacts }) =>
        useAgentGUIConversationActivityView({
          conversations,
          hasConversationQuery: false,
          rootFacts,
          scopeKey: "workspace-1"
        }),
      {
        initialProps: { rootFacts: initialRootFacts },
        wrapper
      }
    );
    const changedBefore =
      rendered.result.current.conversationsById.get("changed");
    const unchangedBefore =
      rendered.result.current.conversationsById.get("unchanged");

    rendered.rerender({
      rootFacts: new Map<string, AgentGUIConversationActivityRootFact>([
        ["changed", { needsUserAction: false, status: "working" }],
        ["unchanged", { needsUserAction: false, status: "ready" }]
      ])
    });

    expect(rendered.result.current.conversationsById.get("changed")).not.toBe(
      changedBefore
    );
    expect(rendered.result.current.conversationsById.get("unchanged")).toBe(
      unchangedBefore
    );
  });
});

const CONVERSATION: AgentGUIConversationSummary = {
  cwd: "/workspace",
  id: "session-1",
  provider: "codex",
  sortTimeUnixMs: Date.now(),
  status: "working",
  title: "Session 1",
  updatedAtUnixMs: Date.now()
};
