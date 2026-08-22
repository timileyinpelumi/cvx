import * as React from "react";
import { cx } from "@/lib/format";

/* ---------------------------------- Button --------------------------------- */

type Variant = "primary" | "secondary" | "ghost" | "danger";
type Size = "sm" | "md";

const VARIANT: Record<Variant, string> = {
  primary:
    "bg-ink text-ink-contrast border border-ink hover:bg-ink-hover hover:border-ink-hover disabled:hover:bg-ink",
  secondary:
    "bg-raised text-fg border border-line hover:border-line-strong hover:bg-sunken disabled:hover:bg-raised",
  ghost:
    "bg-transparent text-fg-muted border border-transparent hover:text-fg hover:bg-sunken",
  danger:
    "bg-transparent text-missing border border-line hover:border-missing hover:bg-missing-soft",
};

const SIZE: Record<Size, string> = {
  sm: "h-8 px-2.5 text-[13px] gap-1.5",
  md: "h-9 px-3.5 text-[13.5px] gap-2",
};

export interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant;
  size?: Size;
  loading?: boolean;
}

export function Button({
  variant = "secondary",
  size = "md",
  loading = false,
  disabled,
  className,
  children,
  ...rest
}: ButtonProps) {
  return (
    <button
      type="button"
      disabled={disabled || loading}
      className={cx(
        "inline-flex items-center justify-center rounded-[var(--radius-ctl)] font-medium",
        "transition-colors duration-[130ms] whitespace-nowrap",
        "disabled:opacity-45 disabled:cursor-not-allowed",
        // While loading, the spinner takes the leading icon's slot.
        loading && "[&_svg:not(.animate-spin)]:hidden",
        VARIANT[variant],
        SIZE[size],
        className,
      )}
      {...rest}
    >
      {loading && <Spinner />}
      {children}
    </button>
  );
}

export function Spinner({ className }: { className?: string }) {
  return (
    <svg
      className={cx("animate-spin shrink-0", className)}
      width="13"
      height="13"
      viewBox="0 0 24 24"
      fill="none"
      aria-hidden="true"
    >
      <circle cx="12" cy="12" r="9" stroke="currentColor" strokeWidth="3" opacity="0.25" />
      <path d="M21 12a9 9 0 0 0-9-9" stroke="currentColor" strokeWidth="3" strokeLinecap="round" />
    </svg>
  );
}

/* ---------------------------------- Layout --------------------------------- */

export function PageHeader({
  title,
  meta,
  actions,
  below,
}: {
  title: string;
  meta?: React.ReactNode;
  actions?: React.ReactNode;
  /** A second row inside the same sticky block, for tabs and the like.
   *  Sticking it separately would mean offsetting it by the header's
   *  height, which changes with the title and the viewport; keeping it in
   *  here means there is no number to get wrong. */
  below?: React.ReactNode;
}) {
  return (
    <header className="sticky top-0 z-20 border-b border-line bg-surface px-5 py-3.5 sm:px-7">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
        <div className="min-w-0 flex-1">
          <h1 className="font-display text-[19px] font-bold tracking-[-0.02em] leading-tight">
            {title}
          </h1>
          {meta && <div className="mt-0.5 text-[12.5px] text-fg-muted">{meta}</div>}
        </div>
        {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
      </div>
      {below && <div className="mt-3">{below}</div>}
    </header>
  );
}

export function Eyebrow({ children, className }: { children: React.ReactNode; className?: string }) {
  return <div className={cx("eyebrow", className)}>{children}</div>;
}

/* ----------------------------------- States -------------------------------- */

export function EmptyState({
  title,
  body,
  action,
}: {
  title: string;
  body: string;
  action?: React.ReactNode;
}) {
  return (
    <div className="flex flex-col items-center justify-center px-6 py-20 text-center">
      <TypeCaseGlyph />
      <h2 className="mt-5 font-display text-[16px] font-bold tracking-[-0.015em]">{title}</h2>
      <p className="mt-1.5 max-w-[38ch] text-[13.5px] leading-relaxed text-fg-muted">{body}</p>
      {action && <div className="mt-5">{action}</div>}
    </div>
  );
}

/** A 3x3 fragment of the type case — the same grid the composer fills, at rest. */
function TypeCaseGlyph() {
  return (
    <svg width="46" height="46" viewBox="0 0 46 46" fill="none" aria-hidden="true">
      {[0, 1, 2].map((r) =>
        [0, 1, 2].map((c) => (
          <rect
            key={`${r}-${c}`}
            x={1 + c * 15}
            y={1 + r * 15}
            width="13"
            height="13"
            rx="2"
            stroke="var(--line-strong)"
            fill={r === 1 && c === 1 ? "var(--ink-soft)" : "none"}
          />
        )),
      )}
    </svg>
  );
}

export function ErrorState({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <div className="flex flex-col items-center justify-center px-6 py-20 text-center">
      <div
        className="flex h-10 w-10 items-center justify-center rounded-full bg-missing-soft text-missing"
        aria-hidden="true"
      >
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
          <path d="M12 8v5M12 17h.01" strokeLinecap="round" />
          <circle cx="12" cy="12" r="9" />
        </svg>
      </div>
      <h2 className="mt-4 font-display text-[15px] font-bold">Something went wrong</h2>
      <p className="mt-1.5 max-w-[42ch] text-[13.5px] leading-relaxed text-fg-muted">{message}</p>
      {onRetry && (
        <Button className="mt-5" onClick={onRetry}>
          Try again
        </Button>
      )}
    </div>
  );
}

export function Skeleton({ className }: { className?: string }) {
  return <div className={cx("animate-pulse rounded bg-sunken", className)} />;
}

export function RowSkeleton({ rows = 5 }: { rows?: number }) {
  return (
    <div className="divide-y divide-line">
      {Array.from({ length: rows }, (_, i) => (
        <div key={i} className="flex items-center gap-4 px-5 py-3.5 sm:px-7">
          <Skeleton className="h-4 flex-1" />
          <Skeleton className="h-3 w-16" />
        </div>
      ))}
    </div>
  );
}

/* ---------------------------------- Badges --------------------------------- */

export function GapBadge({ missing, weak }: { missing: number; weak: number }) {
  if (missing + weak === 0) {
    return <span className="num text-[11.5px] text-fg-faint">no gaps</span>;
  }
  return (
    <span className="num flex items-center gap-1.5 text-[11.5px]">
      {missing > 0 && (
        <span className="rounded bg-missing-soft px-1.5 py-0.5 text-missing" title={`${missing} missing`}>
          {missing} missing
        </span>
      )}
      {weak > 0 && (
        <span className="rounded bg-weak-soft px-1.5 py-0.5 text-weak" title={`${weak} weak`}>
          {weak} weak
        </span>
      )}
    </span>
  );
}
