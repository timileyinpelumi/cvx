"use client";

import { useCallback, useState } from "react";
import { api } from "@/lib/api";
import { cx, relativeAge } from "@/lib/format";
import { useResource } from "@/lib/hooks";
import type { AdminCount, AdminEvent, AdminOverview, AdminUser } from "@/lib/types";
import { Button, EmptyState, ErrorState, Eyebrow, PageHeader, RowSkeleton } from "@/components/ui";

const WINDOWS = [1, 7, 14, 30] as const;
type Tab = "overview" | "events" | "users";

/** Everything worth watching, in one place: what it is doing, what it is
 *  costing, what is failing, and who is using it. Every panel is one indexed
 *  query and every number is a link to the rows behind it. */
export default function AdminPage() {
  const [tab, setTab] = useState<Tab>("overview");
  const [days, setDays] = useState<number>(7);

  return (
    <>
      <PageHeader
        title="Admin"
        meta="Logs, usage and cost"
        actions={
          <div className="flex items-center gap-1.5">
            {WINDOWS.map((w) => (
              <button
                key={w}
                type="button"
                onClick={() => setDays(w)}
                aria-pressed={days === w}
                className={cx(
                  "num h-7 rounded-full border px-2.5 text-[12px] font-medium transition-colors duration-[130ms]",
                  days === w
                    ? "border-ink bg-ink-soft text-ink"
                    : "border-line text-fg-muted hover:border-line-strong hover:text-fg",
                )}
              >
                {w}d
              </button>
            ))}
          </div>
        }
      />

      <div className="mx-auto max-w-[62rem] px-4 py-5 sm:px-7 sm:py-6">
        <div className="mb-4 flex gap-1.5">
          {(["overview", "events", "users"] as Tab[]).map((t) => (
            <button
              key={t}
              type="button"
              onClick={() => setTab(t)}
              aria-pressed={tab === t}
              className={cx(
                "h-8 rounded-full border px-3 text-[12.5px] font-medium capitalize transition-colors duration-[130ms]",
                tab === t
                  ? "border-ink bg-ink-soft text-ink"
                  : "border-line text-fg-muted hover:border-line-strong hover:text-fg",
              )}
            >
              {t}
            </button>
          ))}
        </div>

        {tab === "overview" && <Overview days={days} />}
        {tab === "events" && <Events days={days} />}
        {tab === "users" && <Users />}
      </div>
    </>
  );
}

/* -------------------------------- overview ------------------------------- */

function Overview({ days }: { days: number }) {
  const fetcher = useCallback(() => api.adminOverview(days), [days]);
  const { data, error, reload } = useResource<AdminOverview>(fetcher);

  if (error) return <AdminError message={error} onRetry={reload} />;
  if (data === undefined) return <RowSkeleton rows={8} />;

  const spend = data.usage.reduce((n, u) => n + (u.cost ?? 0), 0);
  const tokens = data.usage.reduce((n, u) => n + (u.tokens ?? 0), 0);
  const llmCalls = data.usage.reduce((n, u) => n + u.total, 0);
  const failures = data.failures.reduce((n, f) => n + f.total, 0);

  return (
    <div className="space-y-4">
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <Stat label="Spend" value={money(spend)} note={`${compact(tokens)} tokens, ${llmCalls} calls`} />
        <Stat
          label="Resumes"
          value={String(data.funnel.generations)}
          note={`${data.totals.generations} all time`}
        />
        <Stat
          label="People"
          value={String(data.totals.activeUsers)}
          note={`${data.funnel.signups} new, ${data.totals.users - data.totals.activeUsers} closed`}
        />
        <Stat
          label="Failures"
          value={String(failures)}
          note={failures === 0 ? "nothing broke" : "see below"}
          tone={failures > 0 ? "bad" : "good"}
        />
      </div>

      <Panel title="Health">
        <dl className="grid gap-x-6 gap-y-2 text-[12.5px] sm:grid-cols-2">
          <Field label="Uptime" value={duration(data.health.uptimeSeconds)} />
          <Field label="Model" value={data.health.llm || "unknown"} />
          <Field label="Database" value={bytes(data.totals.dbBytes)} />
          <Field label="Memory" value={bytes(data.health.heapBytes)} />
          <Field label="Goroutines" value={String(data.health.goroutines)} />
          <Field
            label="Config"
            value={[
              data.health.production ? "production" : "development",
              data.health.oauthEnabled ? "oauth on" : "oauth off",
              data.health.mailEnabled ? "mail on" : "mail off",
            ].join(" · ")}
          />
        </dl>
      </Panel>

      <Panel title={`Resumes a day, last ${data.windowDays} days`}>
        <Sparkline points={data.daily} />
      </Panel>

      <Panel title="Where people stop">
        <Funnel funnel={data.funnel} />
      </Panel>

      {data.failures.length > 0 && (
        <Panel title="What is failing">
          <ul className="divide-y divide-line">
            {data.failures.map((f) => (
              <li key={f.label} className="flex items-start gap-3 py-2 first:pt-0 last:pb-0">
                <span className="num shrink-0 rounded-full bg-missing-soft px-2 py-0.5 text-[11.5px] font-medium text-missing">
                  {f.total}
                </span>
                <span className="min-w-0 break-words text-[12.5px] leading-relaxed">{f.label}</span>
              </li>
            ))}
          </ul>
        </Panel>
      )}

      {data.usage.length > 0 && (
        <Panel title="Cost by model">
          <Table
            head={["Model", "Calls", "Avg", "Tokens", "Cost"]}
            rows={data.usage.map((u) => [
              u.label || "unknown",
              String(u.total),
              `${u.ms ?? 0}ms`,
              compact(u.tokens ?? 0),
              money(u.cost ?? 0),
            ])}
          />
        </Panel>
      )}

      <Panel title="Activity">
        <Table
          head={["Kind", "Count", "Failed", "Avg"]}
          rows={data.activity.map((a) => [a.label, String(a.total), String(a.failed), `${a.ms ?? 0}ms`])}
        />
      </Panel>
    </div>
  );
}

function Funnel({ funnel }: { funnel: AdminOverview["funnel"] }) {
  const steps = [
    { label: "Signed up", value: funnel.signups },
    { label: "Uploaded a resume", value: funnel.uploads },
    { label: "Composed", value: funnel.generations },
    { label: "Downloaded", value: funnel.downloads },
    { label: "Marked sent", value: funnel.sent },
  ];
  const top = Math.max(...steps.map((s) => s.value), 1);

  return (
    <ul className="space-y-2">
      {steps.map((step, i) => {
        const previous = i === 0 ? step.value : steps[i - 1].value;
        const dropped = previous - step.value;
        return (
          <li key={step.label}>
            <div className="flex items-baseline gap-2 text-[12.5px]">
              <span className="min-w-0 flex-1 truncate">{step.label}</span>
              <span className="num font-medium">{step.value}</span>
              {i > 0 && dropped > 0 && (
                <span className="num text-[11.5px] text-fg-faint">{dropped} fewer</span>
              )}
            </div>
            <div className="mt-1 h-1.5 w-full overflow-hidden rounded-full bg-sunken">
              <div
                className="h-full rounded-full bg-ink"
                style={{ width: `${Math.round((step.value / top) * 100)}%` }}
              />
            </div>
          </li>
        );
      })}
    </ul>
  );
}

/** An inline SVG bar chart. A charting library would be several hundred
 *  kilobytes for fourteen rectangles. */
function Sparkline({ points }: { points: AdminCount[] }) {
  if (points.length === 0) return <Empty>Nothing yet.</Empty>;

  const top = Math.max(...points.map((p) => p.total), 1);
  const width = 100;
  const gap = 1.5;
  const barWidth = (width - gap * (points.length - 1)) / points.length;

  return (
    <div>
      <svg viewBox={`0 0 ${width} 28`} preserveAspectRatio="none" className="h-16 w-full" role="img"
        aria-label={`Daily totals, peak ${top}`}>
        {points.map((p, i) => {
          const h = Math.max((p.total / top) * 26, p.total > 0 ? 1.5 : 0.4);
          return (
            <rect
              key={p.label}
              x={i * (barWidth + gap)}
              y={28 - h}
              width={barWidth}
              height={h}
              rx={0.6}
              className={p.failed > 0 ? "fill-missing" : p.total > 0 ? "fill-ink" : "fill-line"}
            >
              <title>{`${p.label}: ${p.total}${p.failed ? `, ${p.failed} failed` : ""}`}</title>
            </rect>
          );
        })}
      </svg>
      <div className="num mt-1 flex justify-between text-[11px] text-fg-faint">
        <span>{points[0]?.label}</span>
        <span>peak {top}</span>
        <span>{points[points.length - 1]?.label}</span>
      </div>
    </div>
  );
}

/* --------------------------------- events -------------------------------- */

function Events({ days }: { days: number }) {
  const [kind, setKind] = useState("");
  const [failedOnly, setFailedOnly] = useState(false);

  const fetcher = useCallback(
    () => api.adminEvents({ kind: kind || undefined, failed: failedOnly, days, limit: 200 }),
    [kind, failedOnly, days],
  );
  const { data, error, reload } = useResource<AdminEvent[]>(fetcher);

  if (error) return <AdminError message={error} onRetry={reload} />;

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <input
          value={kind}
          onChange={(e) => setKind(e.target.value)}
          placeholder="Filter by kind, e.g. generate"
          className="h-8 min-w-0 flex-1 rounded-[var(--radius-ctl)] border border-line bg-raised px-3 text-[12.5px] outline-none focus:border-ink"
        />
        <button
          type="button"
          onClick={() => setFailedOnly((f) => !f)}
          aria-pressed={failedOnly}
          className={cx(
            "h-8 rounded-full border px-3 text-[12.5px] font-medium transition-colors duration-[130ms]",
            failedOnly
              ? "border-missing bg-missing-soft text-missing"
              : "border-line text-fg-muted hover:border-line-strong hover:text-fg",
          )}
        >
          Failures only
        </button>
        <Button size="sm" variant="ghost" onClick={reload}>
          Refresh
        </Button>
      </div>

      {data === undefined ? (
        <RowSkeleton rows={10} />
      ) : data.length === 0 ? (
        <Empty>Nothing matches that.</Empty>
      ) : (
        <div className="panel divide-y divide-line">
          {data.map((e) => (
            <div key={e.id} className="flex flex-wrap items-baseline gap-x-3 gap-y-1 px-4 py-2.5">
              <span
                className={cx(
                  "num shrink-0 rounded-full px-2 py-0.5 text-[11px] font-medium",
                  e.ok ? "bg-sunken text-fg-muted" : "bg-missing-soft text-missing",
                )}
              >
                {e.kind}
              </span>
              {e.target && (
                <span className="min-w-0 flex-1 truncate text-[12.5px]" title={e.target}>
                  {e.target}
                </span>
              )}
              {!e.target && <span className="flex-1" />}
              {e.ms ? <span className="num text-[11.5px] text-fg-faint">{e.ms}ms</span> : null}
              <span className="num shrink-0 text-[11.5px] text-fg-faint">{relativeAge(e.at)}</span>
              {e.detail && (
                <p className="w-full break-words text-[12px] leading-relaxed text-missing">{e.detail}</p>
              )}
              {e.meta && Object.keys(e.meta).length > 0 && (
                <p className="num w-full break-words text-[11.5px] leading-relaxed text-fg-faint">
                  {Object.entries(e.meta)
                    .map(([k, v]) => `${k}=${String(v)}`)
                    .join("  ")}
                </p>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

/* --------------------------------- users --------------------------------- */

function Users() {
  const fetcher = useCallback(() => api.adminUsers(), []);
  const { data, error, reload } = useResource<AdminUser[]>(fetcher);

  if (error) return <AdminError message={error} onRetry={reload} />;
  if (data === undefined) return <RowSkeleton rows={8} />;
  if (data.length === 0) return <Empty>Nobody has signed up yet.</Empty>;

  return (
    <div className="panel overflow-x-auto">
      <Table
        head={["Email", "Provider", "Resumes", "Tokens", "Cost", "Last seen"]}
        rows={data.map((u) => [
          u.deleted ? `${u.email} (closed)` : u.email,
          u.provider,
          String(u.generations),
          compact(u.tokens),
          money(u.cost),
          u.lastSeen ? relativeAge(u.lastSeen) : "never",
        ])}
      />
    </div>
  );
}

/* --------------------------------- pieces -------------------------------- */

function Stat({
  label,
  value,
  note,
  tone,
}: {
  label: string;
  value: string;
  note: string;
  tone?: "good" | "bad";
}) {
  return (
    <div className="panel px-4 py-3.5">
      <Eyebrow>{label}</Eyebrow>
      <p
        className={cx(
          "num mt-1 text-[22px] font-semibold leading-tight tracking-[-0.02em]",
          tone === "bad" && "text-missing",
        )}
      >
        {value}
      </p>
      <p className="mt-0.5 text-[11.5px] text-fg-faint">{note}</p>
    </div>
  );
}

function Panel({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="panel px-4 py-4 sm:px-5">
      <Eyebrow>{title}</Eyebrow>
      <div className="mt-2.5">{children}</div>
    </section>
  );
}

function Field({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-baseline justify-between gap-3 border-b border-line py-1.5 last:border-b-0">
      <dt className="shrink-0 text-fg-muted">{label}</dt>
      <dd className="num min-w-0 truncate text-right" title={value}>
        {value}
      </dd>
    </div>
  );
}

function Table({ head, rows }: { head: string[]; rows: string[][] }) {
  if (rows.length === 0) return <Empty>Nothing yet.</Empty>;
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-left text-[12.5px]">
        <thead>
          <tr className="text-[11px] uppercase tracking-[0.08em] text-fg-faint">
            {head.map((h) => (
              <th key={h} className="whitespace-nowrap py-1.5 pr-4 font-medium last:pr-0">
                {h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody className="divide-y divide-line">
          {rows.map((row, i) => (
            <tr key={i}>
              {row.map((cell, j) => (
                <td
                  key={j}
                  className={cx(
                    "py-2 pr-4 align-top last:pr-0",
                    j === 0 ? "max-w-[18rem] break-words" : "num whitespace-nowrap",
                  )}
                >
                  {cell}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function AdminError({ message, onRetry }: { message: string; onRetry: () => void }) {
  const forbidden = message.toLowerCase().includes("admin");
  if (forbidden) {
    return (
      <EmptyState
        title="Not your panel"
        body="This account is not on the admin allowlist. Set CVX_ADMIN_EMAILS on the server to open it."
      />
    );
  }
  return <ErrorState message={message} onRetry={onRetry} />;
}

function Empty({ children }: { children: React.ReactNode }) {
  return <p className="py-3 text-[12.5px] text-fg-muted">{children}</p>;
}

/* --------------------------------- format -------------------------------- */

function money(n: number): string {
  if (n === 0) return "$0";
  if (n < 0.01) return "<$0.01";
  return `$${n.toFixed(2)}`;
}

function compact(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
  return String(n);
}

function bytes(n: number): string {
  if (n >= 1 << 30) return `${(n / (1 << 30)).toFixed(1)}GB`;
  if (n >= 1 << 20) return `${(n / (1 << 20)).toFixed(1)}MB`;
  if (n >= 1 << 10) return `${(n / (1 << 10)).toFixed(0)}KB`;
  return `${n}B`;
}

function duration(seconds: number): string {
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}
