"use client";

import { createContext, useCallback, useContext, useMemo, useRef, useState } from "react";
import { cx } from "@/lib/format";

type Tone = "info" | "success" | "error";

interface Toast {
  id: number;
  tone: Tone;
  message: string;
}

const ToastContext = createContext<{ push: (message: string, tone?: Tone) => void } | null>(null);

export function useToast() {
  const ctx = useContext(ToastContext);
  if (!ctx) throw new Error("useToast must be used inside ToastProvider");
  return ctx.push;
}

const TONE: Record<Tone, { accent: string; icon: string }> = {
  info: { accent: "border-l-line-strong", icon: "text-fg-faint" },
  success: { accent: "border-l-ink", icon: "text-ink" },
  error: { accent: "border-l-missing", icon: "text-missing" },
};

function ToneIcon({ tone, className }: { tone: Tone; className?: string }) {
  const stroke = { fill: "none", stroke: "currentColor", strokeWidth: 2.2 } as const;
  return (
    <svg width="15" height="15" viewBox="0 0 24 24" className={className} aria-hidden="true">
      {tone === "success" && (
        <>
          <circle cx="12" cy="12" r="9" {...stroke} />
          <path d="m8.5 12.5 2.5 2.5 4.5-5.5" {...stroke} strokeLinecap="round" strokeLinejoin="round" />
        </>
      )}
      {tone === "error" && (
        <>
          <circle cx="12" cy="12" r="9" {...stroke} />
          <path d="M12 7.5v5.5" {...stroke} strokeLinecap="round" />
          <circle cx="12" cy="16.5" r="0.5" fill="currentColor" stroke="currentColor" />
        </>
      )}
      {tone === "info" && (
        <>
          <circle cx="12" cy="12" r="9" {...stroke} />
          <path d="M12 11v5.5" {...stroke} strokeLinecap="round" />
          <circle cx="12" cy="7.5" r="0.5" fill="currentColor" stroke="currentColor" />
        </>
      )}
    </svg>
  );
}

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const nextId = useRef(1);

  const push = useCallback((message: string, tone: Tone = "info") => {
    const id = nextId.current++;
    setToasts((t) => [...t, { id, tone, message }]);
    setTimeout(() => setToasts((t) => t.filter((x) => x.id !== id)), 5000);
  }, []);

  const value = useMemo(() => ({ push }), [push]);

  return (
    <ToastContext.Provider value={value}>
      {children}
      {/* Sits above the mobile tab bar (h-14, hidden at md) plus the home indicator. */}
      <div
        aria-live="polite"
        className={cx(
          "pointer-events-none fixed inset-x-0 z-50 flex flex-col items-stretch gap-2 px-4",
          "bottom-[calc(3.5rem+env(safe-area-inset-bottom)+0.75rem)] md:bottom-6 md:items-center",
        )}
      >
        {toasts.map((t) => (
          <div
            key={t.id}
            className={cx(
              "toast-in pointer-events-auto flex w-full items-center gap-2.5 md:w-auto md:max-w-[30rem]",
              "rounded-[var(--radius-ctl)] border border-line border-l-2 bg-raised",
              "py-3 pl-3.5 pr-2 text-[13px] leading-snug shadow-lg md:py-2.5",
              TONE[t.tone].accent,
            )}
          >
            <ToneIcon tone={t.tone} className={cx("shrink-0", TONE[t.tone].icon)} />
            <span className="min-w-0 flex-1">{t.message}</span>
            <button
              type="button"
              onClick={() => setToasts((all) => all.filter((x) => x.id !== t.id))}
              className={cx(
                "flex shrink-0 items-center justify-center rounded",
                "-my-1 h-8 w-8 text-fg-faint transition-colors duration-[130ms]",
                "hover:bg-sunken hover:text-fg md:h-6 md:w-6",
              )}
              aria-label="Dismiss"
            >
              <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5">
                <path d="M18 6 6 18M6 6l12 12" strokeLinecap="round" />
              </svg>
            </button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}
