import { ConfirmationDialog } from "@tutti-os/ui-system";
import { useRef, useState } from "react";
import type { AgentGUIConversationRailLabels } from "./agentGUIConversationRailLabels";
import type { AgentGUIProjectActionDialog } from "./agentGUIConversationRailTypes";

const DIALOG_CLASS_NAME =
  "nodrag tsh-desktop-no-drag [-webkit-app-region:no-drag]";

export async function executeConfirmedProjectRemoval(input: {
  onConfirmRemoveProjectConversations: (sectionKey: string) => Promise<boolean>;
  onRemoveProject: (path: string) => void;
  path: string;
  sectionKey: string;
}): Promise<boolean> {
  const deleted = await input.onConfirmRemoveProjectConversations(
    input.sectionKey
  );
  if (!deleted) return false;
  input.onRemoveProject(input.path);
  return true;
}

export function AgentGUIProjectActionConfirmationDialog(props: {
  action: AgentGUIProjectActionDialog | null;
  isDeletingProjectConversations: boolean;
  isInteractionLocked: () => boolean;
  labels: AgentGUIConversationRailLabels;
  onConfirmDeleteConversations: (sessionIds: string[]) => Promise<boolean>;
  onConfirmRemoveProjectConversations: (sectionKey: string) => Promise<boolean>;
  onRemoveProject: (path: string) => void;
  setAction: (action: AgentGUIProjectActionDialog | null) => void;
}): React.JSX.Element {
  const { action, labels } = props;
  const projectRemovalInFlightRef = useRef(false);
  const [projectRemovalInFlight, setProjectRemovalInFlight] = useState(false);
  return (
    <ConfirmationDialog
      cancelLabel={labels.cancel}
      className={DIALOG_CLASS_NAME}
      confirmBusy={
        (action?.kind === "batch-delete" ||
          action?.kind === "batch-delete-conversations" ||
          action?.kind === "remove") &&
        (props.isDeletingProjectConversations || projectRemovalInFlight)
      }
      confirmDisabled={props.isInteractionLocked()}
      confirmLabel={
        action?.kind === "batch-delete"
          ? labels.batchDeleteProjectSessionsConfirm
          : action?.kind === "batch-delete-conversations"
            ? labels.batchDeleteConversationsConfirm
            : labels.removeProject
      }
      description={
        action?.kind === "batch-delete"
          ? labels.batchDeleteProjectSessionsBody(
              action.conversationCount,
              action.label
            )
          : action?.kind === "batch-delete-conversations"
            ? labels.batchDeleteConversationsBody(action.conversationCount)
            : action
              ? labels.removeProjectConfirmDescription(action.label)
              : undefined
      }
      onCancel={() => {
        if (!projectRemovalInFlightRef.current) props.setAction(null);
      }}
      onConfirm={() => {
        if (props.isInteractionLocked()) return;
        if (!action) return;
        if (
          action.kind === "batch-delete" ||
          action.kind === "batch-delete-conversations"
        ) {
          props.setAction(null);
          void props.onConfirmDeleteConversations(action.sessionIds);
          return;
        }
        if (projectRemovalInFlightRef.current) return;
        projectRemovalInFlightRef.current = true;
        setProjectRemovalInFlight(true);
        void (async () => {
          try {
            const removed = await executeConfirmedProjectRemoval({
              onConfirmRemoveProjectConversations:
                props.onConfirmRemoveProjectConversations,
              onRemoveProject: props.onRemoveProject,
              path: action.path,
              sectionKey: action.sectionKey
            });
            if (removed) props.setAction(null);
          } finally {
            projectRemovalInFlightRef.current = false;
            setProjectRemovalInFlight(false);
          }
        })();
      }}
      onOpenChange={(open) => {
        if (!open && !projectRemovalInFlightRef.current) props.setAction(null);
      }}
      open={action !== null}
      overlayClassName={DIALOG_CLASS_NAME}
      title={
        action?.kind === "batch-delete"
          ? labels.batchDeleteProjectSessionsTitle
          : action?.kind === "batch-delete-conversations"
            ? labels.batchDeleteConversationsTitle
            : labels.removeProjectConfirmTitle
      }
      tone="destructive"
    />
  );
}
