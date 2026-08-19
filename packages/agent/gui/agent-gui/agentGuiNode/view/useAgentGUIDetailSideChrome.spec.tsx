// @vitest-environment jsdom

import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { AgentComposerProps } from "../AgentComposer";
import {
  agentComposerDraftQuotes,
  emptyAgentComposerDraft
} from "../model/agentComposerDraft";
import { useAgentGUIDetailSideChrome } from "./useAgentGUIDetailSideChrome";
import type { useAgentGUIDetailSideConversation } from "./useAgentGUIDetailSideConversation";

describe("useAgentGUIDetailSideChrome", () => {
  it("stages a selected transcript quote in the main draft without submitting", () => {
    const onDraftContentChange = vi.fn();
    const onSubmit = vi.fn();
    const onRequestComposerFocus = vi.fn();
    const baseComposerProps = {
      agentSessionId: "main-session",
      composerSettings: {},
      draftContent: emptyAgentComposerDraft(),
      isActive: true,
      isSendingTurn: false,
      isSubmittingPrompt: false,
      onDraftContentChange,
      onSubmit,
      selectedAgentTarget: null
    } as unknown as AgentComposerProps;
    const controller = {
      active: null,
      canOpen: true,
      close: vi.fn(),
      draftContent: emptyAgentComposerDraft(),
      entryError: null,
      focused: false,
      focusRequestSequence: null,
      interactionSubmitting: false,
      interactivePrompt: null,
      interrupt: vi.fn(),
      setDraftContent: vi.fn(),
      setFocused: vi.fn(),
      sourceAgentSessionId: "main-session",
      stageSelection: vi.fn(async () => {}),
      submitInteraction: vi.fn(async () => {}),
      submitSide: vi.fn()
    } as unknown as ReturnType<typeof useAgentGUIDetailSideConversation>;

    const rendered = renderHook(() =>
      useAgentGUIDetailSideChrome({
        availableSkills: [],
        baseComposerProps,
        controller,
        conversationFlowLabels: {} as never,
        isVisible: true,
        textSelectionActionsEnabled: true,
        onRequestComposerFocus,
        renderComposerFooterAccessory: undefined
      })
    );

    act(() => {
      rendered.result.current.selectionProps.onAddSelectionToConversation(
        "Selected main answer"
      );
    });

    expect(onDraftContentChange).toHaveBeenCalledOnce();
    expect(
      agentComposerDraftQuotes(onDraftContentChange.mock.calls[0]?.[0])
    ).toEqual([
      expect.objectContaining({
        type: "quote",
        text: "Selected main answer"
      })
    ]);
    expect(onSubmit).not.toHaveBeenCalled();
    expect(onRequestComposerFocus).toHaveBeenCalledOnce();
  });
});
