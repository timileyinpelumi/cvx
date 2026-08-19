"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { CornerDownLeft, Moon, Search, Sun, LogOut } from "lucide-react";
import { NAV } from "@/lib/nav";
import { api } from "@/lib/api";
import { applyTheme, readTheme } from "@/lib/theme";
import { cx } from "@/lib/format";

interface Command {
  id: string;
  label: string;
  hint: string;
  icon: React.ComponentType<{ size?: number | string }>;
  run: () => void;
}

/** Mounted only while open (see Shell), so every open starts with fresh query
 *  and selection — no reset effects needed. */
export function CommandPalette({ onClose }: { onClose: () => void }) {
  const router = useRouter();
  const [query, setQuery] = useState("");
  const [active, setActive] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);

  const commands = useMemo<Command[]>(() => {
    const nav = NAV.map((n) => ({
      id: `go:${n.href}`,
      label: n.label,
      hint: n.hint,
      icon: n.icon as Command["icon"],
      run: () => router.push(n.href),
    }));
    return [
      ...nav,
      {
        id: "theme",
        label: "Switch theme",
        hint: "Light or dark",
        icon: readTheme() === "dark" ? Sun : Moon,
        run: () => applyTheme(readTheme() === "dark" ? "light" : "dark"),
      },
      {
        id: "signout",
        label: "Sign out",
        hint: "See you next time",
        icon: LogOut,
        run: async () => {
          await api.logout().catch(() => {});
          router.replace("/signin");
        },
      },
    ];
  }, [router]);

  const results = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return commands;
    return commands.filter(
      (c) => c.label.toLowerCase().includes(q) || c.hint.toLowerCase().includes(q),
    );
  }, [commands, query]);

  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  function choose(cmd: Command | undefined) {
    if (!cmd) return;
    onClose();
    cmd.run();
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center bg-black/45 px-4 pt-[12vh]"
      onMouseDown={onClose}
      role="presentation"
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-label="Command menu"
        className="w-full max-w-lg overflow-hidden rounded-[var(--radius-panel)] border border-line-strong bg-raised shadow-2xl"
        onMouseDown={(e) => e.stopPropagation()}
        onKeyDown={(e) => {
          if (e.key === "Escape") onClose();
          else if (e.key === "ArrowDown") {
            e.preventDefault();
            setActive((i) => (i + 1) % Math.max(results.length, 1));
          } else if (e.key === "ArrowUp") {
            e.preventDefault();
            setActive((i) => (i - 1 + results.length) % Math.max(results.length, 1));
          } else if (e.key === "Enter") {
            e.preventDefault();
            choose(results[active]);
          }
        }}
      >
        <div className="flex items-center gap-2.5 border-b border-line px-3.5">
          <Search size={15} className="shrink-0 text-fg-faint" />
          <input
            ref={inputRef}
            value={query}
            onChange={(e) => {
              setQuery(e.target.value);
              setActive(0);
            }}
            placeholder="Search"
            aria-label="Search commands"
            className="h-11 flex-1 bg-transparent text-[14px] outline-none"
          />
        </div>

        {results.length === 0 ? (
          <p className="px-4 py-6 text-center text-[13px] text-fg-muted">
            Nothing matches &ldquo;{query}&rdquo;.
          </p>
        ) : (
          <ul className="max-h-[46vh] overflow-y-auto py-1.5 scroll-thin">
            {results.map((cmd, i) => {
              const Icon = cmd.icon;
              return (
                <li key={cmd.id}>
                  <button
                    type="button"
                    onMouseEnter={() => setActive(i)}
                    onClick={() => choose(cmd)}
                    className={cx(
                      "flex w-full items-center gap-3 px-3.5 py-2 text-left",
                      i === active ? "bg-ink-soft" : "bg-transparent",
                    )}
                  >
                    <Icon size={15} />
                    <span className="flex-1 text-[13.5px] font-medium">{cmd.label}</span>
                    <span className="hidden text-[12px] text-fg-faint sm:block">{cmd.hint}</span>
                    {i === active && <CornerDownLeft size={13} className="text-fg-faint" />}
                  </button>
                </li>
              );
            })}
          </ul>
        )}
      </div>
    </div>
  );
}
