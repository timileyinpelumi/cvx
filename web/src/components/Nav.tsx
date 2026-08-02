"use client";

import {
  Archive as ArchiveIcon,
  Flag,
  Scissors,
  SlidersHorizontal,
  User,
} from "lucide-react";

export type View = "tailor" | "history" | "gaps" | "profile" | "settings";

const ITEMS = [
  { view: "tailor", label: "Tailor", Icon: Scissors },
  { view: "history", label: "History", Icon: ArchiveIcon },
  { view: "gaps", label: "Gaps", Icon: Flag },
  { view: "profile", label: "Profile", Icon: User },
  { view: "settings", label: "Settings", Icon: SlidersHorizontal },
] as const;

type NavProps = {
  view: View;
  onChange: (view: View) => void;
  // "bar" is the mobile bottom tab bar, "rail" the desktop masthead icon
  // row. Both render, CSS shows one per breakpoint.
  variant: "bar" | "rail";
};

export function Nav({ view, onChange, variant }: NavProps) {
  if (variant === "rail") {
    return (
      <nav className="iconrail" aria-label="Views">
        {ITEMS.map(({ view: v, label, Icon }) => (
          <button
            key={v}
            type="button"
            className={v === view ? "iconrail-item is-current" : "iconrail-item"}
            aria-label={label}
            title={label}
            aria-current={v === view ? "true" : undefined}
            onClick={() => onChange(v)}
          >
            <Icon size={20} aria-hidden />
          </button>
        ))}
      </nav>
    );
  }

  return (
    <nav className="tabbar" aria-label="Views">
      {ITEMS.map(({ view: v, label, Icon }) => (
        <button
          key={v}
          type="button"
          className={v === view ? "tabbar-item is-current" : "tabbar-item"}
          aria-current={v === view ? "true" : undefined}
          onClick={() => onChange(v)}
        >
          <Icon size={20} aria-hidden />
          {label}
        </button>
      ))}
    </nav>
  );
}
