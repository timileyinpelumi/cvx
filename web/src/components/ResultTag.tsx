"use client";

import { Download } from "lucide-react";

import { Stitch } from "./Stitch";

export type Gap = {
  requirement: string;
  evidence: string;
  severity: string;
};

export type GenerateResult = {
  id: string;
  filename: string;
  gaps: Gap[];
  whatChanged: string[];
  emailed: boolean;
};

type ResultTagProps = {
  result?: GenerateResult;
  pending?: boolean;
};

export function ResultTag({ result, pending = false }: ResultTagProps) {
  return (
    <article
      className={pending ? "result-tag result-tag--pending" : "result-tag"}
      aria-busy={pending || undefined}
    >
      <Stitch width="100%" className="result-tag-stitch" animate={!pending} />
      <div className="result-tag-body">
        {result ? (
          <>
            <div className="result-tag-head">
              <span className="result-file" title={result.filename}>
                {result.filename}
              </span>
              <a
                className="btn btn--primary btn--block"
                href={`/api/generations/${encodeURIComponent(result.id)}/pdf`}
                download
              >
                <Download size={16} aria-hidden="true" />
                Download PDF
              </a>
            </div>

            {result.emailed ? (
              <p className="result-note">Sent to your inbox too.</p>
            ) : null}

            {result.whatChanged.length > 0 ? (
              <section className="result-block">
                <h3 className="block-heading">What changed</h3>
                <ul className="change-list">
                  {result.whatChanged.map((change, i) => (
                    <li key={i} className="change-row">
                      {change}
                    </li>
                  ))}
                </ul>
              </section>
            ) : null}

            {result.gaps.length > 0 ? (
              <section className="result-block">
                <h3 className="block-heading">Gaps for this role</h3>
                <ul className="gap-list">
                  {result.gaps.map((gap, i) => (
                    <li key={i} className="gap-row">
                      <span
                        className={
                          gap.severity === "missing"
                            ? "gap-severity gap-severity--missing"
                            : "gap-severity gap-severity--weak"
                        }
                      >
                        {gap.severity === "missing" ? "missing" : "weak"}
                      </span>
                      <span className="gap-text">
                        <span className="gap-requirement">
                          {gap.requirement}
                        </span>{" "}
                        <span className="gap-evidence">{gap.evidence}</span>
                      </span>
                    </li>
                  ))}
                </ul>
              </section>
            ) : null}
          </>
        ) : null}
      </div>
    </article>
  );
}
