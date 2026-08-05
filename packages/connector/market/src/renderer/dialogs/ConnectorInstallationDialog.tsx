import { useState } from "react";
import {
  Button,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from "@tutti-os/ui-system/components";

import type { ConnectorMarketI18nRuntime } from "../../i18n/connectorMarketI18n.ts";
import type { ConnectorAgentOptionView } from "../../services/view/connectorMarketViewTypes.ts";
import { ConnectorIcon } from "../catalog/ConnectorIcon.tsx";
import { ConnectorAgentGrantEditor } from "./ConnectorAgentGrantEditor.tsx";
import { ConnectorDialogSection } from "./ConnectorDialogSection.tsx";

export function ConnectorInstallationDialog({
  agents,
  connectorKey,
  displayName,
  i18n,
  onClose,
  onInstall
}: {
  agents: ReadonlyArray<Readonly<ConnectorAgentOptionView>>;
  connectorKey: string;
  displayName: string;
  i18n: ConnectorMarketI18nRuntime;
  onClose: () => void;
  onInstall: (principalIds: string[]) => void;
}) {
  const [selectedPrincipalIds, setSelectedPrincipalIds] = useState<string[]>(
    []
  );
  return (
    <DialogContent className="max-h-[min(720px,calc(100vh-32px))] overflow-y-auto sm:max-w-[520px]">
      <DialogHeader>
        <div className="flex items-center gap-3 pr-8">
          <ConnectorIcon
            connectorKey={connectorKey}
            displayName={displayName}
            size="lg"
          />
          <div>
            <DialogTitle>
              {i18n.t("dialogInstallationTitle", { name: displayName })}
            </DialogTitle>
            <DialogDescription>
              {i18n.t("dialogInstallationDescription")}
            </DialogDescription>
          </div>
        </div>
      </DialogHeader>
      <ConnectorDialogSection title={i18n.t("agentAccessTitle")}>
        <ConnectorAgentGrantEditor
          agents={agents}
          emptyLabel={i18n.t("agentAccessEmpty")}
          selectedPrincipalIds={selectedPrincipalIds}
          onChange={setSelectedPrincipalIds}
        />
      </ConnectorDialogSection>
      <DialogFooter>
        <Button
          size="dialog"
          type="button"
          variant="secondary"
          onClick={onClose}
        >
          {i18n.t("cancel")}
        </Button>
        <Button
          size="dialog"
          type="button"
          onClick={() => onInstall(selectedPrincipalIds)}
        >
          {i18n.t("actionInstall")}
        </Button>
      </DialogFooter>
    </DialogContent>
  );
}
