// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ComposerDraftAttachments } from "./ComposerDraftAttachments";

describe("ComposerDraftAttachments", () => {
  it("renders selected transcript text as a removable annotation count", () => {
    const onRemoveQuotes = vi.fn();
    render(
      <ComposerDraftAttachments
        draftImages={[]}
        draftLargeTexts={[]}
        draftQuotes={[
          { type: "quote", id: "quote-1", text: "First selection" },
          { type: "quote", id: "quote-2", text: "Second selection" }
        ]}
        removeLabel="Remove reference"
        onRemoveImage={vi.fn()}
        onRemoveLargeText={vi.fn()}
        onExpandLargeText={vi.fn()}
        onRemoveQuotes={onRemoveQuotes}
      />
    );

    expect(
      screen.getByTestId("agent-gui-composer-quote-drafts")
    ).toHaveTextContent("2 annotations");
    fireEvent.click(screen.getByRole("button", { name: "Remove reference" }));
    expect(onRemoveQuotes).toHaveBeenCalledOnce();
  });
});
