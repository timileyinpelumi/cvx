"use client";

import { useEffect, useRef, useState } from "react";
import {
  Archive as ArchiveIcon,
  Flag,
  LayoutGrid,
  Scissors,
  User,
} from "lucide-react";

export type View = "tailor" | "history" | "gaps" | "profile";

const ITEMS = [
  { view: "tailor", label: "Tailor", Icon: Scissors },
  { view: "history", label: "History", Icon: ArchiveIcon },
  { view: "gaps", label: "Gaps", Icon: Flag },
  { view: "profile", label: "Profile", Icon: User },
] as const;

type NavProps = {
  view: View;
  onChange: (view: View) => void;
};

export function Nav({ view, onChange }: NavProps) {
  const [open, setOpen] = useState(false);
  const buttonRef = useRef<HTMLButtonElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    function onPointerDown(event: PointerEvent) {
      const target = event.target as Node;
      if (menuRef.current?.contains(target)) return;
      if (buttonRef.current?.contains(target)) return;
      setOpen(false);
    }
    document.addEventListener("pointerdown", onPointerDown);
    return () => document.removeEventListener("pointerdown", onPointerDown);
  }, [open]);

  // Opening drops focus on the current view's item so arrow keys start from
  // where the user actually is.
  useEffect(() => {
    if (!open) return;
    menuRef.current
      ?.querySelector<HTMLButtonElement>(`[data-view="${view}"]`)
      ?.focus();
  }, [open, view]);

  function select(next: View) {
    onChange(next);
    setOpen(false);
    buttonRef.current?.focus();
  }

  function handleMenuKeyDown(event: React.KeyboardEvent<HTMLDivElement>) {
    const items = Array.from(
      menuRef.current?.querySelectorAll<HTMLButtonElement>("[data-view]") ?? [],
    );
    const current = items.indexOf(document.activeElement as HTMLButtonElement);
    if (event.key === "Escape") {
      setOpen(false);
      buttonRef.current?.focus();
    } else if (event.key === "ArrowDown") {
      event.preventDefault();
      items[(current + 1) % items.length]?.focus();
    } else if (event.key === "ArrowUp") {
      event.preventDefault();
      items[(current - 1 + items.length) % items.length]?.focus();
    } else if (event.key === "Home") {
      event.preventDefault();
      items[0]?.focus();
    } else if (event.key === "End") {
      event.preventDefault();
      items[items.length - 1]?.focus();
    }
  }

  return (
    <div className="nav-menu">
      <button
        ref={buttonRef}
        type="button"
        className="nav-trigger"
        aria-label="Views"
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen((o) => !o)}
      >
        <LayoutGrid size={20} aria-hidden />
      </button>
      {open ? (
        <div
          ref={menuRef}
          className="nav-pop"
          role="menu"
          aria-label="Views"
          onKeyDown={handleMenuKeyDown}
        >
          {ITEMS.map(({ view: v, label, Icon }) => (
            <button
              key={v}
              type="button"
              role="menuitem"
              data-view={v}
              className={v === view ? "nav-item is-current" : "nav-item"}
              aria-current={v === view ? "true" : undefined}
              onClick={() => select(v)}
            >
              <Icon size={16} aria-hidden />
              {label}
            </button>
          ))}
        </div>
      ) : null}
    </div>
  );
}
