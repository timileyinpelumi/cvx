"use client";

import { Moon, Sun } from "lucide-react";
import { applyTheme } from "@/lib/theme";
import { useTheme } from "@/lib/hooks";
import { cx } from "@/lib/format";

export function ThemeToggle({ className }: { className?: string }) {
  const theme = useTheme();

  return (
    <button
      type="button"
      onClick={() => applyTheme(theme === "dark" ? "light" : "dark")}
      aria-label={theme === "dark" ? "Switch to light" : "Switch to dark"}
      className={cx(
        "flex h-8 w-8 items-center justify-center rounded-[var(--radius-ctl)]",
        "text-fg-muted transition-colors duration-[130ms] hover:bg-sunken hover:text-fg",
        className,
      )}
    >
      {/* null until hydration, so the icon never flips under the user. */}
      {theme === "dark" ? <Moon size={15} /> : theme === "light" ? <Sun size={15} /> : null}
    </button>
  );
}
