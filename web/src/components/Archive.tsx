"use client";

import { useState } from "react";
import { Download, FileText } from "lucide-react";

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
};

const dateFormat = new Intl.DateTimeFormat("en-GB", {
  day: "numeric",
  month: "short",
  year: "numeric",
});

function formatDate(iso: string) {
  const parsed = new Date(iso);
  return Number.isNaN(parsed.getTime()) ? iso : dateFormat.format(parsed);
}

export function Archive({ rows }: ArchiveProps) {
  const [query, setQuery] = useState("");

  if (rows.length === 0) return null;

  const q = query.trim().toLowerCase();
  const filtered = q
    ? rows.filter((row) => row.targetRole.toLowerCase().includes(q))
    : rows;

  return (
    <section className="archive">
      {rows.length > 1 ? (
        <input
          type="search"
          className="archive-search"
          placeholder="Filter by role"
          aria-label="Filter by role"
          value={query}
          onChange={(event) => setQuery(event.target.value)}
        />
      ) : null}

      {filtered.length === 0 ? (
        <p className="view-empty">No generations match that role.</p>
      ) : (
        <ul className="archive-list">
          {filtered.map((row) => {
            const missing = row.gaps.filter(
              (gap) => gap.severity === "missing",
            ).length;
            const weak = row.gaps.length - missing;
            return (
              <li key={row.id} className="archive-row">
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
                    href={`/api/generations/${encodeURIComponent(row.id)}/pdf`}
                    aria-label={`Download ${row.targetRole}`}
                    download
                  >
                    <Download size={16} aria-hidden />
                    <span className="archive-action-label">Resume</span>
                  </a>
                  {row.hasCoverLetter ? (
                    <a
                      className="archive-action"
                      href={`/api/generations/${encodeURIComponent(row.id)}/cover`}
                      aria-label={`Download cover letter for ${row.targetRole}`}
                      download
                    >
                      <FileText size={16} aria-hidden />
                      <span className="archive-action-label">Cover letter</span>
                    </a>
                  ) : null}
                </div>
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}
