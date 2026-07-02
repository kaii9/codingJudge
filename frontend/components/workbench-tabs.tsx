"use client";

import type { KeyboardEvent } from "react";

export type WorkbenchTab = "problem" | "code" | "result";

interface WorkbenchTabsProps {
  active: WorkbenchTab;
  onChange: (tab: WorkbenchTab) => void;
}

const tabs: ReadonlyArray<{ id: WorkbenchTab; label: string }> = [
  { id: "problem", label: "Problem" },
  { id: "code", label: "Code" },
  { id: "result", label: "Result" },
];

function keyboardTarget(current: WorkbenchTab, key: string): WorkbenchTab | null {
  const currentIndex = tabs.findIndex(tab => tab.id === current);

  if (key === "Home") return tabs[0].id;
  if (key === "End") return tabs[tabs.length - 1].id;
  if (key === "ArrowRight") return tabs[(currentIndex + 1) % tabs.length].id;
  if (key === "ArrowLeft") {
    return tabs[(currentIndex - 1 + tabs.length) % tabs.length].id;
  }
  return null;
}

export function WorkbenchTabs({ active, onChange }: WorkbenchTabsProps) {
  const handleKeyDown = (
    event: KeyboardEvent<HTMLButtonElement>,
    current: WorkbenchTab,
  ) => {
    const next = keyboardTarget(current, event.key);
    if (!next) return;

    event.preventDefault();
    onChange(next);
    event.currentTarget.parentElement
      ?.querySelector<HTMLButtonElement>(`#workbench-${next}-tab`)
      ?.focus();
  };

  return (
    <div className="workbench__tabs" role="tablist" aria-label="Workbench views">
      {tabs.map(tab => (
        <button
          key={tab.id}
          id={`workbench-${tab.id}-tab`}
          type="button"
          role="tab"
          aria-controls={`workbench-${tab.id}-panel`}
          aria-selected={active === tab.id}
          tabIndex={active === tab.id ? 0 : -1}
          onClick={() => onChange(tab.id)}
          onKeyDown={event => handleKeyDown(event, tab.id)}
        >
          {tab.label}
        </button>
      ))}
    </div>
  );
}
