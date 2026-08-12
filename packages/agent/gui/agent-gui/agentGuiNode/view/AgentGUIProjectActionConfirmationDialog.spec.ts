import { describe, expect, it, vi } from "vitest";
import { executeConfirmedProjectRemoval } from "./AgentGUIProjectActionConfirmationDialog";

describe("executeConfirmedProjectRemoval", () => {
  it("removes the project only after every project session is deleted", async () => {
    const calls: string[] = [];
    const onConfirmRemoveProjectConversations = vi.fn(async () => {
      calls.push("sessions");
      return true;
    });
    const onRemoveProject = vi.fn(() => calls.push("project"));

    await expect(
      executeConfirmedProjectRemoval({
        onConfirmRemoveProjectConversations,
        onRemoveProject,
        path: "/workspace/project",
        sectionKey: "project:/workspace/project"
      })
    ).resolves.toBe(true);

    expect(onConfirmRemoveProjectConversations).toHaveBeenCalledWith(
      "project:/workspace/project"
    );
    expect(onRemoveProject).toHaveBeenCalledWith("/workspace/project");
    expect(calls).toEqual(["sessions", "project"]);
  });

  it("keeps the project when session deletion fails", async () => {
    const onConfirmRemoveProjectConversations = vi.fn(async () => false);
    const onRemoveProject = vi.fn();

    await expect(
      executeConfirmedProjectRemoval({
        onConfirmRemoveProjectConversations,
        onRemoveProject,
        path: "/workspace/project",
        sectionKey: "project:/workspace/project"
      })
    ).resolves.toBe(false);

    expect(onRemoveProject).not.toHaveBeenCalled();
  });
});
