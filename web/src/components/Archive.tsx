"use client";

export type GenerationMeta = {
  id: string;
  targetRole: string;
  filename: string;
  createdAt: string;
  gaps: unknown[];
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
  if (rows.length === 0) return null;

  return (
    <section className="archive">
      <ul className="archive-list">
        {rows.map((row) => (
          <li key={row.id} className="archive-row">
            <span className="archive-meta">
              <span className="archive-role">{row.targetRole}</span>
              <time className="archive-date" dateTime={row.createdAt}>
                {formatDate(row.createdAt)}
              </time>
            </span>
            <span className="archive-links">
              <a
                className="archive-link"
                href={`/api/generations/${encodeURIComponent(row.id)}/pdf`}
                // Every row's link reads "Download"; the role disambiguates them
                // for anyone tabbing or listing links.
                aria-label={`Download ${row.targetRole}`}
                download
              >
                Download
              </a>
              {row.hasCoverLetter ? (
                <a
                  className="archive-link"
                  href={`/api/generations/${encodeURIComponent(row.id)}/cover`}
                  aria-label={`Download cover letter for ${row.targetRole}`}
                  download
                >
                  Download cover letter
                </a>
              ) : null}
            </span>
          </li>
        ))}
      </ul>
    </section>
  );
}
