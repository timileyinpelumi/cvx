"use client";

import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { Scissors } from "lucide-react";

import { ResultTag, type GenerateResult } from "./ResultTag";

const MAX_ROWS = 8;

type GeneratorProps = {
  result: GenerateResult | null;
  onResult: (result: GenerateResult) => void;
  onProfileChanged?: () => void;
};

export function Generator({ result, onResult, onProfileChanged }: GeneratorProps) {
  const fieldRef = useRef<HTMLTextAreaElement>(null);
  const [roleInput, setRoleInput] = useState("");
  const [coverLetter, setCoverLetter] = useState(false);
  const [recruiterEmail, setRecruiterEmail] = useState(false);
  // Captured at generate time so "Generate again" reruns exactly what
  // produced the result, even if the textarea has been edited since.
  const [submitted, setSubmitted] = useState<{
    role: string;
    cover: boolean;
    recruiter: boolean;
  } | null>(null);
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  const [errorDetail, setErrorDetail] = useState<string | null>(null);

  const autoGrow = useCallback(() => {
    const field = fieldRef.current;
    if (!field) return;
    const styles = getComputedStyle(field);
    const border =
      parseFloat(styles.borderTopWidth) + parseFloat(styles.borderBottomWidth);
    const max =
      parseFloat(styles.lineHeight) * MAX_ROWS +
      parseFloat(styles.paddingTop) +
      parseFloat(styles.paddingBottom) +
      border;
    field.style.height = "auto";
    field.style.height = `${Math.min(field.scrollHeight + border, max)}px`;
  }, []);

  useLayoutEffect(autoGrow, [autoGrow, roleInput]);

  useEffect(() => {
    window.addEventListener("resize", autoGrow);
    return () => window.removeEventListener("resize", autoGrow);
  }, [autoGrow]);

  async function generate(role: string, cover: boolean, recruiter: boolean) {
    setSubmitted({ role, cover, recruiter });
    setBusy(true);
    setFailed(false);
    setErrorDetail(null);
    try {
      const res = await fetch("/api/generate", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          roleInput: role,
          coverLetter: cover,
          recruiterEmail: recruiter,
        }),
      });
      if (!res.ok) {
        let detail = "";
        try {
          const errBody = (await res.json()) as { error?: string };
          detail = errBody.error ?? "";
        } catch {
          // non-JSON error body; no detail to show
        }
        setErrorDetail(detail || null);
        setFailed(true);
        return;
      }
      onResult((await res.json()) as GenerateResult);
    } catch {
      setErrorDetail("server unreachable");
      setFailed(true);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="tailor">
      <textarea
        ref={fieldRef}
        className="composer-field"
        aria-label="Paste a role title, a job posting link, or the full job description"
        placeholder="Paste a role title, a job posting link, or the full job description"
        value={roleInput}
        disabled={busy}
        onChange={(event) => setRoleInput(event.target.value)}
      />

      <label className="option-row">
        <input
          type="checkbox"
          className="option-box"
          checked={coverLetter}
          disabled={busy}
          onChange={(event) => setCoverLetter(event.target.checked)}
        />
        <span className="option-label">Also write a cover letter</span>
      </label>

      <label className="option-row">
        <input
          type="checkbox"
          className="option-box"
          checked={recruiterEmail}
          disabled={busy}
          onChange={(event) => setRecruiterEmail(event.target.checked)}
        />
        <span className="option-label">Recruiter-ready email</span>
      </label>

      <div className="composer-actions">
        <button
          type="button"
          className="btn btn--primary btn--block"
          disabled={busy || roleInput.trim() === ""}
          aria-busy={busy || undefined}
          onClick={() => void generate(roleInput, coverLetter, recruiterEmail)}
        >
          {busy ? null : <Scissors size={16} aria-hidden="true" />}
          {busy ? "Tailoring" : "Tailor resume"}
        </button>
      </div>

      <div className="notice-slot" role="status">
        {failed ? (
          <>
            <p className="notice notice--error">
              The tailoring failed. Try again in a moment.
            </p>
            {errorDetail ? <p className="error-detail">{errorDetail}</p> : null}
          </>
        ) : null}
      </div>

      {busy ? <ResultTag pending /> : null}
      {!busy && result ? (
        <ResultTag
          key={result.id}
          result={result}
          onProfileChanged={onProfileChanged}
          onRegenerate={
            submitted
              ? () =>
                  void generate(
                    submitted.role,
                    submitted.cover,
                    submitted.recruiter,
                  )
              : undefined
          }
        />
      ) : null}
    </div>
  );
}
