import type { TranslateFn } from "../../../i18n/index";
import type { AgentGUIViewLabels } from "./AgentGUINodeView.types";

type AgentGUIUsageStatusLabels = Pick<
  AgentGUIViewLabels,
  | "slashStatusUsageAuthRequired"
  | "slashStatusUsageSessionExpired"
  | "slashStatusUsageSubscriptionRequired"
  | "slashStatusUsageQuotaExhausted"
  | "slashStatusUsageConfigInvalid"
  | "slashStatusUsageError"
>;

export function agentGUIUsageStatusLabels(
  t: TranslateFn
): AgentGUIUsageStatusLabels {
  return {
    slashStatusUsageAuthRequired: t(
      "agentHost.agentGui.slashStatusUsageAuthRequired"
    ),
    slashStatusUsageSessionExpired: t(
      "agentHost.agentGui.slashStatusUsageSessionExpired"
    ),
    slashStatusUsageSubscriptionRequired: t(
      "agentHost.agentGui.slashStatusUsageSubscriptionRequired"
    ),
    slashStatusUsageQuotaExhausted: t(
      "agentHost.agentGui.slashStatusUsageQuotaExhausted"
    ),
    slashStatusUsageConfigInvalid: t(
      "agentHost.agentGui.slashStatusUsageConfigInvalid"
    ),
    slashStatusUsageError: t("agentHost.agentGui.slashStatusUsageError")
  };
}
