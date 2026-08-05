import { useState } from "react";
import {
  Button,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  StatusDot
} from "@tutti-os/ui-system/components";

import type { ConnectorMarketI18nRuntime } from "../../i18n/connectorMarketI18n.ts";
import type {
  ConnectorDetailFieldView,
  ConnectorAgentOptionView,
  ConnectorPermissionView
} from "../../services/view/connectorMarketViewTypes.ts";
import { ConnectorIcon } from "../catalog/ConnectorIcon.tsx";
import { ConnectorDialogSection } from "./ConnectorDialogSection.tsx";
import { ConnectorAgentGrantEditor } from "./ConnectorAgentGrantEditor.tsx";
import { connectorDetailLabel } from "./connectorDetailLabel.ts";
import { ConnectorPermissionList } from "./ConnectorPermissionList.tsx";

export interface ConnectorManagementDialogProps {
  canAuthorize: boolean;
  agents: ReadonlyArray<Readonly<ConnectorAgentOptionView>>;
  connectorKey: string;
  details: ReadonlyArray<Readonly<ConnectorDetailFieldView>>;
  displayName: string;
  i18n: ConnectorMarketI18nRuntime;
  onAuthorize: () => void;
  onClose: () => void;
  onUninstall: () => void;
  onAgentGrantsChange: (principalIds: string[]) => void;
  permissions: ReadonlyArray<Readonly<ConnectorPermissionView>>;
  selectedPrincipalIds: ReadonlyArray<string>;
}

export function ConnectorManagementDialog({
  agents,
  canAuthorize,
  connectorKey,
  details,
  displayName,
  i18n,
  onAuthorize,
  onClose,
  onUninstall,
  onAgentGrantsChange,
  permissions,
  selectedPrincipalIds
}: ConnectorManagementDialogProps) {
  const [selection, setSelection] = useState<string[]>([
    ...selectedPrincipalIds
  ]);
  return (
    <DialogContent className="max-h-[min(720px,calc(100vh-32px))] overflow-y-auto sm:max-w-[520px]">
      <DialogHeader>
        <div className="flex items-center gap-3 pr-8">
          <ConnectorIcon
            connectorKey={connectorKey}
            displayName={displayName}
            size="lg"
          />
          <div className="min-w-0">
            <DialogTitle>
              {i18n.t("dialogManagementTitle", { name: displayName })}
            </DialogTitle>
            <DialogDescription>
              {i18n.t("dialogManagementDescription")}
            </DialogDescription>
            <div className="mt-1 flex items-center gap-1.5 text-[11px] text-[var(--state-success)]">
              <StatusDot size="xs" tone="green" />
              {i18n.t("connectedStatus")}
            </div>
          </div>
        </div>
      </DialogHeader>

      <dl className="grid grid-cols-2 overflow-hidden rounded-lg border border-[var(--border-1)]">
        {details.map((detail, index) => {
          const isLastRow = index >= details.length - 2;
          return (
            <div
              key={detail.id}
              className={`flex min-w-0 items-center justify-between gap-3 border-r border-[var(--border-1)] px-3 py-2.5 even:border-r-0 ${
                isLastRow ? "" : "border-b"
              }`}
            >
              <dt className="text-[11px] text-[var(--text-tertiary)]">
                {connectorDetailLabel(detail.id, i18n)}
              </dt>
              <dd className="m-0 truncate text-[11px] text-[var(--text-primary)]">
                {detail.value}
              </dd>
            </div>
          );
        })}
      </dl>

      <ConnectorDialogSection title={i18n.t("permissionsTitle")}>
        <ConnectorPermissionList i18n={i18n} permissions={permissions} />
      </ConnectorDialogSection>

      <ConnectorDialogSection title={i18n.t("agentAccessTitle")}>
        <ConnectorAgentGrantEditor
          agents={agents}
          emptyLabel={i18n.t("agentAccessEmpty")}
          selectedPrincipalIds={selection}
          onChange={setSelection}
        />
        <div className="mt-2 flex justify-end">
          <Button
            size="sm"
            type="button"
            variant="secondary"
            onClick={() => onAgentGrantsChange(selection)}
          >
            {i18n.t("actionSaveAgentAccess")}
          </Button>
        </div>
      </ConnectorDialogSection>

      <DialogFooter className="sm:justify-between">
        <Button
          size="dialog"
          type="button"
          variant="destructive-secondary"
          onClick={onUninstall}
        >
          {i18n.t("actionUninstall")}
        </Button>
        <div className="flex items-center justify-end gap-2.5">
          <Button
            size="dialog"
            type="button"
            variant="secondary"
            onClick={onClose}
          >
            {i18n.t("close")}
          </Button>
          {canAuthorize ? (
            <Button size="dialog" type="button" onClick={onAuthorize}>
              {i18n.t("actionUpdateAuthorization")}
            </Button>
          ) : null}
        </div>
      </DialogFooter>
    </DialogContent>
  );
}
