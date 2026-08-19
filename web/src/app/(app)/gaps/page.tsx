"use client";

import { useCallback } from "react";
import Link from "next/link";
import { api } from "@/lib/api";
import { useResource } from "@/lib/hooks";
import { cx } from "@/lib/format";
import type { GapTrend, GapsSummary } from "@/lib/types";
import { Button, EmptyState, ErrorState, PageHeader, RowSkeleton } from "@/components/ui";

export default function GapsPage() {
  const fetcher = useCallback(() => api.gaps(), []);
  const { data, error, reload } = useResource<GapsSummary>(fetcher);

  const trends = data?.trends ?? [];

  return (
    <>
      <PageHeader
        title="Gaps"
        meta={
          data
            ? trends.length === 1
              ? "1 thing that keeps coming up"
              : `${trends.length} things that keep coming up`
            : undefined
        }
      />

      {error ? (
        <ErrorState message={error} onRetry={reload} />
      ) : data === undefined ? (
        <RowSkeleton rows={6} />
      ) : trends.length === 0 ? (
        <EmptyState
          title="Nothing's come up yet"
          body="When two or more jobs ask for something you don't have yet, it shows up here. Compose a few resumes and you'll start to see a pattern."
          action={
            <Link href="/compose">
              <Button variant="primary">Compose a resume</Button>
            </Link>
          }
        />
      ) : (
        <>
          <div className="border-b border-line px-5 py-4 sm:px-7">
            <p className="max-w-[64ch] text-[13.5px] leading-relaxed text-fg-muted">
              Things jobs keep asking for that aren&rsquo;t in your profile yet. cvx
              won&rsquo;t put them on your resume, so this is a good list of what to
              pick up next.
            </p>
          </div>

          <ul className="divide-y divide-line">
            {trends.map((trend) => (
              <TrendRow key={trend.requirement} trend={trend} />
            ))}
          </ul>
        </>
      )}
    </>
  );
}

function TrendRow({ trend }: { trend: GapTrend }) {
  return (
    <li className="px-5 py-4 transition-colors duration-[130ms] hover:bg-sunken sm:px-7">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5">
        <p className="min-w-0 flex-1 basis-52 text-[13.5px] font-medium leading-snug">
          {trend.requirement}
        </p>
        <div className="flex shrink-0 items-center gap-1.5">
          {trend.missing > 0 && <SeverityPill tone="missing" count={trend.missing} />}
          {trend.weak > 0 && <SeverityPill tone="weak" count={trend.weak} />}
          <span className="num ml-1 text-[11.5px] text-fg-faint">
            in {trend.count === 1 ? "1 resume" : `${trend.count} resumes`}
          </span>
        </div>
      </div>

      {trend.lastEvidence && (
        <p className="mt-1.5 max-w-[68ch] text-[12.5px] leading-relaxed text-fg-faint">
          {trend.lastEvidence}
        </p>
      )}
    </li>
  );
}

function SeverityPill({ tone, count }: { tone: "missing" | "weak"; count: number }) {
  return (
    <span
      className={cx(
        "num rounded-full px-2 py-0.5 text-[11px] font-medium leading-[16px]",
        tone === "missing" ? "bg-missing-soft text-missing" : "bg-weak-soft text-weak",
      )}
    >
      {count} {tone}
    </span>
  );
}
