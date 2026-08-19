"use client";

import { useCallback, useMemo, useState } from "react";
import Link from "next/link";
import { ChevronRight, Download, Pin, Search, Trash2, X } from "lucide-react";
import { api, pdfURL } from "@/lib/api";
import { useResource } from "@/lib/hooks";
import { countBySeverity, cx, plural, relativeAge } from "@/lib/format";
import { STATUS_TONES, STATUSES, type GenerationMeta } from "@/lib/types";
import { useToast } from "@/components/Toast";
import { Button, EmptyState, ErrorState, GapBadge, PageHeader, RowSkeleton } from "@/components/ui";

export default function ResumesPage() {
  const toast = useToast();
  const [query, setQuery] = useState("");
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [busy, setBusy] = useState(false);

  const fetcher = useCallback(() => api.generations(), []);
  const { data: rows, error, reload } = useResource<GenerationMeta[]>(fetcher);

  const filtered = useMemo(() => {
    if (!rows) return [];
    const q = query.trim().toLowerCase();
    if (!q) return rows;
    return rows.filter(
      (r) =>
        r.targetRole.toLowerCase().includes(q) || r.filename.toLowerCase().includes(q),
    );
  }, [rows, query]);

  async function togglePin(row: GenerationMeta) {
    try {
      await api.setPinned(row.id, !row.pinned);
      reload();
    } catch (err) {
      toast(err instanceof Error ? err.message : "Couldn't pin that one", "error");
    }
  }

  function toggle(id: string) {
    setSelected((s) => {
      const next = new Set(s);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  async function deleteSelected() {
    setBusy(true);
    const ids = [...selected];
    const failed: string[] = [];
    for (const id of ids) {
      try {
        await api.deleteGeneration(id);
      } catch {
        failed.push(id);
      }
    }
    setSelected(new Set());
    reload();
    setBusy(false);
    const removed = ids.length - failed.length;
    if (failed.length === 0) toast(`Deleted ${plural(removed, "resume")}.`, "success");
    else toast(`Deleted ${removed}. ${plural(failed.length, "resume")} couldn't be deleted.`, "error");
  }

  function downloadSelected() {
    for (const id of selected) {
      const a = document.createElement("a");
      a.href = pdfURL(id);
      a.download = "";
      document.body.appendChild(a);
      a.click();
      a.remove();
    }
    toast(`Downloading ${plural(selected.size, "resume")}.`);
  }

  return (
    <>
      <PageHeader
        title="Resumes"
        meta={rows ? `${plural(rows.length, "resume")} so far` : undefined}
        actions={
          rows && rows.length > 0 ? (
            <div className="relative">
              <Search
                size={14}
                className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-fg-faint"
              />
              <input
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="Search by job title"
                aria-label="Search resumes by job title"
                className={cx(
                  "h-8 w-40 rounded-[var(--radius-ctl)] border border-line bg-raised pl-8 pr-2.5",
                  "text-[13px] outline-none transition-colors duration-[130ms] focus:border-ink sm:w-56",
                )}
              />
            </div>
          ) : null
        }
      />

      {selected.size > 0 && (
        <div className="flex flex-wrap items-center gap-2 border-b border-line bg-ink-soft px-5 py-2.5 sm:px-7">
          <span className="num text-[12.5px] font-medium text-ink">
            {selected.size} selected
          </span>
          <div className="flex-1" />
          <Button size="sm" onClick={downloadSelected} disabled={busy}>
            <Download size={13} />
            Download
          </Button>
          <Button size="sm" variant="danger" onClick={deleteSelected} loading={busy}>
            <Trash2 size={13} />
            Delete
          </Button>
          <Button size="sm" variant="ghost" onClick={() => setSelected(new Set())} disabled={busy}>
            <X size={13} />
            Clear
          </Button>
        </div>
      )}

      {error ? (
        <ErrorState message={error} onRetry={reload} />
      ) : rows === undefined ? (
        <RowSkeleton />
      ) : rows.length === 0 ? (
        <EmptyState
          title="No resumes yet"
          body="Once you compose one it shows up here, along with what changed and anything the job asked for that you're missing."
          action={
            <Link href="/compose">
              <Button variant="primary">Compose your first one</Button>
            </Link>
          }
        />
      ) : filtered.length === 0 ? (
        <EmptyState
          title="Nothing found"
          body={`No resumes match “${query}”.`}
          action={<Button onClick={() => setQuery("")}>Clear search</Button>}
        />
      ) : (
        <ul className="divide-y divide-line">
          {filtered.map((row) => (
            <Row
              key={row.id}
              row={row}
              selected={selected.has(row.id)}
              onToggle={() => toggle(row.id)}
              onTogglePin={() => togglePin(row)}
            />
          ))}
        </ul>
      )}
    </>
  );
}

function Row({
  row,
  selected,
  onToggle,
  onTogglePin,
}: {
  row: GenerationMeta;
  selected: boolean;
  onToggle: () => void;
  onTogglePin: () => void;
}) {
  const { missing, weak } = countBySeverity(row.gaps);

  return (
    <li
      className={cx(
        "group flex items-center gap-3 px-5 transition-colors duration-[130ms] sm:px-7",
        selected ? "bg-ink-soft" : "hover:bg-sunken",
      )}
    >
      <input
        type="checkbox"
        checked={selected}
        onChange={onToggle}
        aria-label={`Select ${row.targetRole}`}
        className="h-3.5 w-3.5 shrink-0 accent-[var(--ink)]"
      />

      <Link href={`/resumes/${row.id}`} className="flex min-w-0 flex-1 items-center gap-3 py-3.5">
        <div className="min-w-0 flex-1">
          <p className="truncate text-[13.5px] font-medium">{row.targetRole}</p>
          <p className="num truncate text-[11.5px] text-fg-faint">{row.filename}</p>
        </div>

        {row.status && (
          <span
            className={cx(
              "num shrink-0 rounded-full px-2 py-0.5 text-[10.5px] font-medium",
              STATUS_TONES[row.status].badge,
            )}
          >
            {STATUSES.find((s) => s.value === row.status)?.label}
          </span>
        )}

        <div className="hidden shrink-0 sm:block">
          <GapBadge missing={missing} weak={weak} />
        </div>

        {row.hasCoverLetter && (
          <span className="num hidden shrink-0 rounded border border-line px-1.5 py-0.5 text-[10.5px] text-fg-muted lg:block">
            + cover
          </span>
        )}

        <span className="num w-12 shrink-0 text-right text-[11.5px] text-fg-faint">
          {relativeAge(row.createdAt)}
        </span>

        <ChevronRight size={14} className="shrink-0 text-fg-faint" />
      </Link>

      <button
        type="button"
        onClick={onTogglePin}
        aria-label={row.pinned ? `Unpin ${row.targetRole}` : `Pin ${row.targetRole}`}
        aria-pressed={row.pinned}
        title={row.pinned ? "Unpin" : "Pin to top"}
        className={cx(
          "flex h-7 w-7 shrink-0 items-center justify-center rounded transition-colors duration-[130ms]",
          row.pinned
            ? "text-ink"
            : "text-fg-faint hover:bg-sunken hover:text-fg focus-visible:opacity-100 sm:opacity-0 sm:group-hover:opacity-100",
        )}
      >
        <Pin size={13} fill={row.pinned ? "currentColor" : "none"} />
      </button>
    </li>
  );
}
