"use client";

import { useState } from "react";
import { Download, RotateCcw } from "lucide-react";

import { GapFill } from "./GapFill";
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
  coverLetter: boolean;
  recruiterEmail: boolean;
};

type ResultTagProps = {
  result?: GenerateResult;
  pending?: boolean;
  onProfileChanged?: () => void;
  onRegenerate?: () => void;
};

export function ResultTag({
  result,
  pending = false,
  onProfileChanged,
  onRegenerate,
}: ResultTagProps) {
  const [filled, setFilled] = useState<Set<number>>(new Set());

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
              <div className="result-tag-actions">
                <a
                  className="btn btn--primary btn--block"
                  href={`/api/generations/${encodeURIComponent(result.id)}/pdf`}
                  download
                >
                  <Download size={16} aria-hidden="true" />
                  Download PDF
                </a>
                {result.coverLetter ? (
                  <a
                    className="btn btn--secondary btn--block"
                    href={`/api/generations/${encodeURIComponent(
                      result.id,
                    )}/cover`}
                    download
                  >
                    <Download size={16} aria-hidden="true" />
                    Download cover letter
                  </a>
                ) : null}
              </div>
            </div>

            {result.emailed ? (
              <p className="result-note">
                {result.recruiterEmail
                  ? "A recruiter-ready email is in your inbox."
                  : "Sent to your inbox too."}
              </p>
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
                    <li
                      key={i}
                      className={
                        filled.has(i) ? "gap-row gap-row--filled" : "gap-row"
                      }
                    >
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
                        <GapFill
                          context={`${gap.requirement} — ${gap.evidence}`}
                          onFilled={() => {
                            setFilled((prev) => new Set(prev).add(i));
                            onProfileChanged?.();
                          }}
                        />
                      </span>
                    </li>
                  ))}
                </ul>
              </section>
            ) : null}

            {filled.size > 0 && onRegenerate ? (
              <div className="result-regenerate">
                <button
                  type="button"
                  className="btn btn--secondary btn--block"
                  onClick={onRegenerate}
                >
                  <RotateCcw size={16} aria-hidden="true" />
                  Generate again
                </button>
              </div>
            ) : null}
          </>
        ) : null}
      </div>
    </article>
  );
}
