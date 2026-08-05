import { afterEach, describe, expect, test } from "vitest";
import { cleanup, fireEvent, render, waitFor } from "@testing-library/react";
import { renderMentionRow } from "./MentionRow.tsx";

afterEach(() => cleanup());

describe("MentionRow image fallbacks", () => {
  test("replaces a failed app icon with the kind glyph", () => {
    const view = render(
      renderMentionRow({
        kind: "app",
        name: "Weather",
        iconUrl: "https://cdn.example.test/weather.png"
      })
    );

    fireEvent.error(view.container.querySelector("img") as HTMLImageElement);

    expect(view.container.querySelector("img")).toBeNull();
    expect(
      view.container.querySelector(".rich-text-at-mention-kind-icon--app")
    ).not.toBeNull();
  });

  test("replaces a failed image thumbnail with the file glyph", () => {
    const view = render(
      renderMentionRow({
        kind: "file",
        name: "diagram.png",
        visualKind: "image",
        thumbnailUrl: "https://cdn.example.test/diagram.png"
      })
    );

    fireEvent.error(view.container.querySelector("img") as HTMLImageElement);

    expect(view.container.querySelector("img")).toBeNull();
    expect(
      view.container.querySelector(
        '[data-rich-text-at-mention-file-visual-kind="image"]'
      )
    ).not.toBeNull();
  });

  test("replaces a failed user avatar and placeholder with a user glyph", async () => {
    const view = render(
      renderMentionRow({
        kind: "session",
        participant: "Agent",
        userAvatarUrl: "https://cdn.example.test/user.png",
        userAvatarPlaceholderUrl: "https://cdn.example.test/placeholder.png",
        agentIconUrl: "https://cdn.example.test/agent.png"
      })
    );

    const userAvatar = view.container.querySelector(
      "[data-rich-text-at-mention-user-avatar] img"
    ) as HTMLImageElement;
    fireEvent.error(userAvatar);
    await waitFor(() => {
      expect(
        view.container
          .querySelector("[data-rich-text-at-mention-user-avatar] img")
          ?.getAttribute("src")
      ).toBe("https://cdn.example.test/placeholder.png");
    });
    fireEvent.error(
      view.container.querySelector(
        "[data-rich-text-at-mention-user-avatar] img"
      ) as HTMLImageElement
    );
    await waitFor(() => {
      expect(
        view.container.querySelector(
          "[data-rich-text-at-mention-user-avatar] img"
        )
      ).toBeNull();
    });
    expect(
      view.container.querySelector(
        "[data-rich-text-at-mention-user-avatar] svg"
      )
    ).not.toBeNull();
  });
});
