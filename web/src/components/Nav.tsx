"use client";

import { useRef } from "react";

export type View = "tailor" | "history" | "gaps" | "profile";

const TABS: { view: View; label: string }[] = [
  { view: "tailor", label: "Tailor" },
  { view: "history", label: "History" },
  { view: "gaps", label: "Gaps" },
  { view: "profile", label: "Profile" },
];

export const PANEL_ID = "view-panel";

export function tabId(view: View) {
  return `tab-${view}`;
}

type NavProps = {
  view: View;
  onChange: (view: View) => void;
};

export function Nav({ view, onChange }: NavProps) {
  const rowRef = useRef<HTMLDivElement>(null);

  // Switching views is instant, so the tabs follow the APG's automatic
  // activation pattern: arrow keys move focus and select in one step.
  function select(next: View) {
    onChange(next);
    rowRef.current
      ?.querySelector<HTMLButtonElement>(`[data-view="${next}"]`)
      ?.focus();
  }

  function handleKeyDown(event: React.KeyboardEvent<HTMLDivElement>) {
    const current = TABS.findIndex((tab) => tab.view === view);
    let next = -1;
    if (event.key === "ArrowRight") next = (current + 1) % TABS.length;
    else if (event.key === "ArrowLeft")
      next = (current - 1 + TABS.length) % TABS.length;
    else if (event.key === "Home") next = 0;
    else if (event.key === "End") next = TABS.length - 1;
    if (next === -1) return;
    event.preventDefault();
    select(TABS[next].view);
  }

  return (
    <div
      ref={rowRef}
      className="nav"
      role="tablist"
      aria-label="Views"
      onKeyDown={handleKeyDown}
    >
      {TABS.map((tab) => {
        const active = tab.view === view;
        return (
          <button
            key={tab.view}
            type="button"
            role="tab"
            id={tabId(tab.view)}
            data-view={tab.view}
            className={active ? "nav-tab is-active" : "nav-tab"}
            aria-selected={active}
            aria-controls={PANEL_ID}
            tabIndex={active ? 0 : -1}
            onClick={() => onChange(tab.view)}
          >
            {tab.label}
          </button>
        );
      })}
    </div>
  );
}
