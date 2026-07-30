import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { MinimumVersionUpgradeApp } from "./app/MinimumVersionUpgradeApp.tsx";
import { createMinimumVersionUpgradeWindowContainer } from "./app/windows/minimumUpgrade/createMinimumVersionUpgradeWindowContainer.ts";
import { I18nProvider } from "./i18n";
import "./style.css";

const root = document.querySelector<HTMLDivElement>("#app");
if (!root) {
  throw new Error("Minimum-version renderer root '#app' was not found.");
}
const container = createMinimumVersionUpgradeWindowContainer();
createRoot(root).render(
  <StrictMode>
    <I18nProvider>
      <MinimumVersionUpgradeApp port={container.port} />
    </I18nProvider>
  </StrictMode>
);
