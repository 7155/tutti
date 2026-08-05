import { Checkbox } from "@tutti-os/ui-system/components";

import type { ConnectorAgentOptionView } from "../../services/view/connectorMarketViewTypes.ts";

export interface ConnectorAgentGrantEditorProps {
  agents: ReadonlyArray<Readonly<ConnectorAgentOptionView>>;
  selectedPrincipalIds: ReadonlyArray<string>;
  onChange: (principalIds: string[]) => void;
  emptyLabel: string;
}

export function ConnectorAgentGrantEditor({
  agents,
  emptyLabel,
  selectedPrincipalIds,
  onChange
}: ConnectorAgentGrantEditorProps) {
  const selected = new Set(selectedPrincipalIds);
  return (
    <div className="overflow-hidden rounded-lg border border-[var(--border-1)]">
      {agents.length === 0 ? (
        <p className="m-0 px-3 py-3 text-[11px] text-[var(--text-secondary)]">
          {emptyLabel}
        </p>
      ) : (
        agents.map((agent) => (
          <label
            key={agent.principalId}
            className="flex cursor-pointer items-center gap-3 border-b border-[var(--border-1)] px-3 py-3 last:border-b-0"
          >
            <Checkbox
              checked={selected.has(agent.principalId)}
              onCheckedChange={(checked) => {
                const next = new Set(selected);
                if (checked) next.add(agent.principalId);
                else next.delete(agent.principalId);
                onChange([...next].sort());
              }}
            />
            <span className="min-w-0">
              <span className="block text-[12px] font-medium text-[var(--text-primary)]">
                {agent.name}
              </span>
              {agent.description ? (
                <span className="mt-0.5 block truncate text-[11px] text-[var(--text-secondary)]">
                  {agent.description}
                </span>
              ) : null}
            </span>
          </label>
        ))
      )}
    </div>
  );
}
