"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { plural } from "@/lib/format";
import { usePrefersReducedMotion } from "@/lib/hooks";

/* The type case.
 *
 * Generation is one slow POST with no progress channel, so this deliberately
 * does NOT show a percentage — a number we can't measure would be a lie. It
 * shows what the machine is doing, and fills a case sized to the user's real
 * profile: one cell per item and per skill. Nothing outside the case can end
 * up in the resume, which is exactly the guardrail the server enforces.
 *
 * The fill approaches the total asymptotically and never reaches it on its
 * own; only the response completes it. */

const MAX_CELLS = 112;

const STAGES = [
  { at: 0, label: "Reading the job ad" },
  { at: 2500, label: "Picking what fits" },
  { at: 7000, label: "Writing it up" },
  { at: 14000, label: "Making the PDF" },
] as const;

/** Deterministic shuffle — cells light in a scattered order, like sorts being
 *  lifted, but identically on every render so hydration stays stable. */
function scatter(n: number): number[] {
  const order = Array.from({ length: n }, (_, i) => i);
  let seed = 0x9e3779b9;
  for (let i = n - 1; i > 0; i--) {
    seed = (seed * 1664525 + 1013904223) >>> 0;
    const j = seed % (i + 1);
    [order[i], order[j]] = [order[j], order[i]];
  }
  return order;
}

export function TypeCase({
  itemCount,
  skillCount,
  done = false,
}: {
  itemCount: number;
  skillCount: number;
  done?: boolean;
}) {
  const total = Math.max(12, Math.min(itemCount + skillCount, MAX_CELLS));
  const order = useMemo(() => scatter(total), [total]);

  const [elapsed, setElapsed] = useState(0);
  const started = useRef<number | null>(null);
  const reduced = usePrefersReducedMotion();

  useEffect(() => {
    if (done) return;
    started.current = performance.now();
    let frame = 0;
    const tick = () => {
      setElapsed(performance.now() - (started.current ?? 0));
      frame = requestAnimationFrame(tick);
    };
    frame = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(frame);
  }, [done]);

  // Asymptotic: fast at first, then decelerating, capped just under full.
  const progress = done ? 1 : Math.min(0.93, 1 - Math.exp(-elapsed / 9000));
  const lit = Math.round(total * progress);

  const stage = done
    ? "All done"
    : [...STAGES].reverse().find((s) => elapsed >= s.at)?.label ?? STAGES[0].label;

  return (
    <div className="flex flex-col items-center">
      <div
        className="grid gap-[3px]"
        style={{ gridTemplateColumns: `repeat(${columnsFor(total)}, 1fr)`, maxWidth: 300 }}
        role="img"
        aria-label={`Working from ${plural(itemCount, "entry", "entries")} and ${plural(skillCount, "skill")}`}
      >
        {order.map((slot, i) => {
          const isLit = reduced ? true : slot < lit;
          return (
            <span
              key={i}
              className="h-[9px] w-[9px] rounded-[2px] transition-colors duration-300"
              style={{
                background: isLit ? "var(--ink)" : "var(--ink-soft)",
                opacity: isLit ? 1 : 0.65,
              }}
            />
          );
        })}
      </div>

      <p aria-live="polite" className="mt-5 font-display text-[14px] font-semibold tracking-[-0.01em]">
        {stage}
      </p>
      <p className="num mt-1 text-[11.5px] text-fg-faint">
        Working from {itemCount} entries and {skillCount} skills
      </p>
    </div>
  );
}

function columnsFor(total: number): number {
  return Math.max(6, Math.min(14, Math.ceil(Math.sqrt(total * 1.6))));
}

