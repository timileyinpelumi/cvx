"use client";

export type GenerationMeta = {
  id: string;
  targetRole: string;
  filename: string;
  createdAt: string;
  gaps: unknown[];
  whatChanged: string[];
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
      <h2 className="section-heading">Earlier resumes</h2>
      <ul className="archive-list">
        {rows.map((row) => (
          <li key={row.id} className="archive-row">
            <span className="archive-role">{row.targetRole}</span>
            <time className="archive-date" dateTime={row.createdAt}>
              {formatDate(row.createdAt)}
            </time>
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
          </li>
        ))}
      </ul>
    </section>
  );
}
