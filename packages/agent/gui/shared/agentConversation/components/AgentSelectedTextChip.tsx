import { MessageSquareText } from "lucide-react";
import type { JSX } from "react";
import { Badge } from "@tutti-os/ui-system";
import { useTranslation } from "../../../i18n/index";
import type { AgentSelectedTextVM } from "../contracts/agentMessageRowVM";

export function AgentSelectedTextChip({
  selectedText
}: {
  selectedText: AgentSelectedTextVM;
}): JSX.Element {
  "use memo";
  const { t } = useTranslation();
  const count = selectedText.count;
  const label =
    count === 1
      ? t("agentHost.agentGui.selectedTextFragment")
      : t("agentHost.agentGui.selectedTextFragments", {
          count: String(count)
        });

  return (
    <Badge
      variant="outline"
      data-testid="agent-selected-text-chip"
      data-selected-text-count={count}
      aria-label={label}
      className="max-w-full rounded-full border-[var(--line-2)] bg-transparent px-3 py-1.5 text-[13px] font-normal leading-5 text-[var(--text-primary)]"
    >
      <MessageSquareText
        size={16}
        strokeWidth={1.8}
        aria-hidden="true"
        className="shrink-0 text-[var(--text-secondary)]"
      />
      <span className="truncate">{label}</span>
    </Badge>
  );
}
