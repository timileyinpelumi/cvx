"use client";

export type GapTrend = {
  requirement: string;
  count: number;
  missing: number;
  weak: number;
  lastEvidence: string;
};

type GapTrackerProps = {
  trends: GapTrend[];
  total: number;
};

export function GapTracker({ trends, total }: GapTrackerProps) {
  if (trends.length === 0) return null;

  return (
    <section className="gap-tracker">
      <h2 className="section-heading">Recurring gaps</h2>
      <ul className="gap-tracker-list">
        {trends.map((trend) => (
          <li key={trend.requirement} className="gap-tracker-row">
            <div className="gap-tracker-top">
              <span className="gap-tracker-requirement">
                {trend.requirement}
              </span>
              <span
                className={
                  trend.missing > trend.weak
                    ? "gap-tracker-count gap-tracker-count--missing"
                    : "gap-tracker-count gap-tracker-count--weak"
                }
              >
                {trend.count} of {total} roles
              </span>
            </div>
            <p className="gap-tracker-evidence">{trend.lastEvidence}</p>
          </li>
        ))}
      </ul>
    </section>
  );
}
