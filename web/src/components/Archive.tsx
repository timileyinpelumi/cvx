"use client";

import { useState } from "react";
import { ChevronLeft, ChevronRight, Download, FileText } from "lucide-react";

export type GenerationMeta = {
  id: string;
  targetRole: string;
  filename: string;
  createdAt: string;
  gaps: { severity: string }[];
  whatChanged: string[];
  hasCoverLetter: boolean;
};

type ArchiveProps = {
  rows: GenerationMeta[];
  onChanged: () => void;
};

const PAGE_SIZE = 10;

const dateFormat = new Intl.DateTimeFormat("en-GB", {
  day: "numeric",
  month: "short",
  year: "numeric",
});

function formatDate(iso: string) {
  const parsed = new Date(iso);
  return Number.isNaN(parsed.getTime()) ? iso : dateFormat.format(parsed);
}

function pdfURL(id: string) {
  return `/api/generations/${encodeURIComponent(id)}/pdf`;
}

function coverURL(id: string) {
  return `/api/generations/${encodeURIComponent(id)}/cover`;
}

export function Archive({ rows, onChanged }: ArchiveProps) {
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(1);
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());
  const [confirming, setConfirming] = useState(false);
  const [busy, setBusy] = useState(false);

  if (rows.length === 0) return null;

  const q = query.trim().toLowerCase();
  const filtered = q
    ? rows.filter((row) => row.targetRole.toLowerCase().includes(q))
    : rows;

  const pageCount = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
  const current = Math.min(page, pageCount);
  const start = (current - 1) * PAGE_SIZE;
  const visible = filtered.slice(start, start + PAGE_SIZE);

  function toggle(id: string) {
    setConfirming(false);
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  function clearSelection() {
    setSelected(new Set());
    setConfirming(false);
  }

  // Browsers drop rapid-fire programmatic downloads, so the files go out on
  // a short stagger.
  function downloadSelected() {
    const urls = rows
      .filter((row) => selected.has(row.id))
      .flatMap((row) =>
        row.hasCoverLetter
          ? [pdfURL(row.id), coverURL(row.id)]
          : [pdfURL(row.id)],
      );
    urls.forEach((href, i) => {
      window.setTimeout(() => {
        const link = document.createElement("a");
        link.href = href;
        link.download = "";
        link.click();
      }, i * 300);
    });
  }

  async function deleteSelected() {
    setBusy(true);
    try {
      await Promise.all(
        [...selected].map((id) =>
          fetch(`/api/generations/${encodeURIComponent(id)}`, {
            method: "DELETE",
          }),
        ),
      );
    } catch {
      // Whatever failed still exists server-side; the reload below shows
      // the archive as it actually is.
    } finally {
      setBusy(false);
      clearSelection();
      onChanged();
    }
  }

  return (
    <section className="archive">
      {rows.length > 1 ? (
        <input
          type="search"
          className="archive-search"
          placeholder="Filter by role"
          aria-label="Filter by role"
          value={query}
          onChange={(event) => {
            setQuery(event.target.value);
            setPage(1);
          }}
        />
      ) : null}

      {selected.size > 0 ? (
        <div className="archive-bar" role="toolbar" aria-label="Selection">
          <span className="archive-bar-count">
            {selected.size} selected
          </span>
          <button
            type="button"
            className="archive-bar-action"
            disabled={busy}
            onClick={downloadSelected}
          >
            Download
          </button>
          <button
            type="button"
            className="archive-bar-action archive-bar-action--danger"
            disabled={busy}
            onClick={() => {
              if (confirming) void deleteSelected();
              else setConfirming(true);
            }}
          >
            {confirming ? "Confirm delete" : "Delete"}
          </button>
          <button
            type="button"
            className="archive-bar-action archive-bar-action--quiet"
            disabled={busy}
            onClick={clearSelection}
          >
            Clear
          </button>
        </div>
      ) : null}

      {filtered.length === 0 ? (
        <p className="view-empty">No generations match that role.</p>
      ) : (
        <ul className="archive-list">
          {visible.map((row) => {
            const missing = row.gaps.filter(
              (gap) => gap.severity === "missing",
            ).length;
            const weak = row.gaps.length - missing;
            return (
              <li key={row.id} className="archive-row">
                <input
                  type="checkbox"
                  className="option-box archive-check"
                  checked={selected.has(row.id)}
                  aria-label={`Select ${row.targetRole}`}
                  onChange={() => toggle(row.id)}
                />
                <div className="archive-main">
                  <div className="archive-info">
                    <span className="archive-role">{row.targetRole}</span>
                    <span className="archive-facts">
                      <time className="archive-date" dateTime={row.createdAt}>
                        {formatDate(row.createdAt)}
                      </time>
                      {row.hasCoverLetter ? (
                        <span className="archive-cover">cover letter</span>
                      ) : null}
                      {row.gaps.length > 0 ? (
                        <span
                          className="archive-gaps"
                          aria-label={`${missing} missing, ${weak} weak`}
                        >
                          {row.gaps.map((gap, i) => (
                            <i
                              key={i}
                              className={
                                gap.severity === "missing"
                                  ? "gap-dot is-missing"
                                  : "gap-dot is-weak"
                              }
                            />
                          ))}
                          {row.gaps.length}{" "}
                          {row.gaps.length === 1 ? "gap" : "gaps"}
                        </span>
                      ) : null}
                    </span>
                  </div>
                  <div className="archive-actions">
                    <a
                      className="archive-action"
                      href={pdfURL(row.id)}
                      aria-label={`Download ${row.targetRole}`}
                      download
                    >
                      <Download size={16} aria-hidden />
                      <span className="archive-action-label">Resume</span>
                    </a>
                    {row.hasCoverLetter ? (
                      <a
                        className="archive-action"
                        href={coverURL(row.id)}
                        aria-label={`Download cover letter for ${row.targetRole}`}
                        download
                      >
                        <FileText size={16} aria-hidden />
                        <span className="archive-action-label">
                          Cover letter
                        </span>
                      </a>
                    ) : null}
                  </div>
                </div>
              </li>
            );
          })}
        </ul>
      )}

      {pageCount > 1 ? (
        <div className="archive-pager">
          <button
            type="button"
            className="archive-pager-btn"
            aria-label="Previous page"
            disabled={current === 1}
            onClick={() => setPage(current - 1)}
          >
            <ChevronLeft size={16} aria-hidden />
          </button>
          <span>
            {start + 1}-{Math.min(start + PAGE_SIZE, filtered.length)} of{" "}
            {filtered.length}
          </span>
          <button
            type="button"
            className="archive-pager-btn"
            aria-label="Next page"
            disabled={current === pageCount}
            onClick={() => setPage(current + 1)}
          >
            <ChevronRight size={16} aria-hidden />
          </button>
        </div>
      ) : null}
    </section>
  );
}
