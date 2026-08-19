"use client";

import { useState } from "react";
import { Check, ChevronDown, Download, Eye, Mail } from "lucide-react";
import { coverURL, pdfURL } from "@/lib/api";
import { countBySeverity, cx, fullDate, plural } from "@/lib/format";
import type { Gap } from "@/lib/types";
import { Button, Eyebrow } from "./ui";

export interface ProofData {
  id: string;
  targetRole?: string;
  filename: string;
  gaps: Gap[];
  whatChanged: string[];
  hasCoverLetter: boolean;
  coverFilename?: string;
  emailed?: boolean;
  createdAt?: string;
}

export function Proof({ data }: { data: ProofData }) {
  const { missing, weak, total } = countBySeverity(data.gaps);
  const [previewing, setPreviewing] = useState(false);

  return (
    <article className="paper-scope overflow-hidden rounded-[var(--radius-panel)] border border-line bg-surface text-fg">
      <header className="border-b border-line px-4 py-4 sm:px-6">
        <Eyebrow>Your resume</Eyebrow>
        <h2 className="mt-1.5 font-display text-[19px] font-bold leading-tight tracking-[-0.02em]">
          {data.targetRole || "Your resume"}
        </h2>
        <p className="num mt-1 truncate text-[12px] text-fg-muted" title={data.filename}>
          {data.filename}
        </p>

        <div className="mt-4 flex flex-wrap items-center gap-2">
          <a href={pdfURL(data.id)} download>
            <Button variant="primary" size="sm">
              <Download size={13} />
              Resume
            </Button>
          </a>

          {data.hasCoverLetter && (
            <a href={coverURL(data.id)} download>
              <Button size="sm">
                <Download size={13} />
                Cover letter
              </Button>
            </a>
          )}

          <Button size="sm" variant="ghost" onClick={() => setPreviewing((p) => !p)} aria-expanded={previewing}>
            <Eye size={13} />
            {previewing ? "Hide preview" : "Preview"}
          </Button>

          {data.emailed && (
            <span className="flex items-center gap-1.5 text-[12px] text-fg-muted">
              <Mail size={13} />
              Also sent to your inbox
            </span>
          )}
        </div>

        {data.createdAt && (
          <p className="num mt-3 text-[11.5px] text-fg-faint">{fullDate(data.createdAt)}</p>
        )}
      </header>

      {previewing && (
        <div className="border-b border-line bg-sunken p-3 sm:p-4">
          <iframe
            src={`${pdfURL(data.id)}?inline=1`}
            title={`Preview of ${data.filename}`}
            className="h-[70vh] w-full rounded-[var(--radius-ctl)] border border-line bg-raised"
          />
        </div>
      )}

      <Section
        title="What changed"
        count={data.whatChanged.length}
        defaultOpen
        empty="Nothing noted for this one."
      >
        <ul className="space-y-2">
          {data.whatChanged.map((line, i) => (
            <li key={i} className="flex gap-2.5 text-[13.5px] leading-relaxed">
              <Check size={14} className="mt-[3px] shrink-0 text-ink" />
              <span>{line}</span>
            </li>
          ))}
        </ul>
      </Section>

      <Section
        title="Gaps"
        count={total}
        badge={
          total > 0 ? (
            <span className="num text-[11.5px] text-fg-muted">
              {missing > 0 && <span className="text-missing">{missing} missing</span>}
              {missing > 0 && weak > 0 && " · "}
              {weak > 0 && <span className="text-weak">{weak} weak</span>}
            </span>
          ) : null
        }
        defaultOpen={total > 0 && total <= 4}
        empty="You've got everything this job asked for."
      >
        <ul className="space-y-3.5">
          {data.gaps.map((gap, i) => {
            const isMissing = gap.severity === "missing";
            return (
              <li key={i} className="flex gap-3">
                <span
                  aria-hidden="true"
                  className={cx(
                    "mt-[6px] h-1.5 w-1.5 shrink-0 rounded-full",
                    isMissing ? "bg-missing" : "bg-weak",
                  )}
                />
                <div className="min-w-0">
                  <p className="text-[13.5px] font-medium leading-snug">{gap.requirement}</p>
                  <p className="mt-0.5 text-[12.5px] leading-relaxed text-fg-muted">
                    {gap.evidence ||
                      (isMissing
                        ? "Nothing in your profile matches this."
                        : "You've got some of this, but not much.")}
                  </p>
                </div>
              </li>
            );
          })}
        </ul>
      </Section>
    </article>
  );
}

function Section({
  title,
  count,
  badge,
  defaultOpen = false,
  empty,
  children,
}: {
  title: string;
  count: number;
  badge?: React.ReactNode;
  defaultOpen?: boolean;
  empty: string;
  children: React.ReactNode;
}) {
  const [open, setOpen] = useState(defaultOpen);

  if (count === 0) {
    return (
      <div className="border-t border-line px-4 py-4 sm:px-6">
        <Eyebrow>{title}</Eyebrow>
        <p className="mt-1.5 text-[13px] text-fg-muted">{empty}</p>
      </div>
    );
  }

  return (
    <div className="border-t border-line">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
        className="flex w-full items-center gap-2.5 px-4 py-3.5 text-left sm:px-6"
      >
        <ChevronDown
          size={14}
          className={cx("shrink-0 text-fg-faint transition-transform duration-[130ms]", !open && "-rotate-90")}
        />
        <Eyebrow className="flex-1">{title}</Eyebrow>
        {badge ?? <span className="num text-[11.5px] text-fg-faint">{count}</span>}
      </button>
      {open && <div className="px-4 pb-5 sm:px-6">{children}</div>}
    </div>
  );
}

export function proofSummary(gaps: Gap[]): string {
  const { missing, weak } = countBySeverity(gaps);
  if (missing + weak === 0) return "no gaps";
  const parts: string[] = [];
  if (missing) parts.push(plural(missing, "missing gap"));
  if (weak) parts.push(`${weak} weak`);
  return parts.join(" · ");
}
