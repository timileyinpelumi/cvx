"use client";

import { useEffect, useRef, useState } from "react";
import { Link2, RotateCcw } from "lucide-react";
import { api } from "@/lib/api";
import { cx } from "@/lib/format";
import type { GenerateResult } from "@/lib/types";
import { useSession } from "@/components/Session";
import { useToast } from "@/components/Toast";
import { Proof, type ProofData } from "@/components/Proof";
import { TypeCase } from "@/components/TypeCase";
import { Button, ErrorState, Eyebrow, PageHeader } from "@/components/ui";

type Status = "idle" | "composing" | "done" | "failed";

const looksLikeURL = (s: string) => /^https?:\/\/\S+$/i.test(s.trim());

export default function ComposePage() {
  const { profile } = useSession();
  const toast = useToast();

  const [role, setRole] = useState("");
  const [coverLetter, setCoverLetter] = useState(false);
  const [recruiterEmail, setRecruiterEmail] = useState(false);
  const [status, setStatus] = useState<Status>("idle");
  const [proof, setProof] = useState<ProofData | null>(null);
  const [error, setError] = useState<string | null>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => textareaRef.current?.focus(), []);

  // The recruiter email toggle starts from the saved preference.
  useEffect(() => {
    api
      .settings()
      .then((s) => setRecruiterEmail(s.recruiterAuto))
      .catch(() => {});
  }, []);

  const composing = status === "composing";
  const canCompose = role.trim().length > 0 && !composing;

  async function compose() {
    if (!canCompose) return;
    setStatus("composing");
    setError(null);
    setProof(null);

    try {
      const result: GenerateResult = await api.generate({
        roleInput: role.trim(),
        coverLetter,
        recruiterEmail,
      });

      // The generate response carries no target role, but the list does — and
      // refetching also keeps the resumes list correct without a second visit.
      let targetRole: string | undefined;
      let roleSummary: string | undefined;
      let createdAt: string | undefined;
      try {
        const list = await api.generations();
        const match = list.find((g) => g.id === result.id);
        targetRole = match?.targetRole;
        roleSummary = match?.roleSummary;
        createdAt = match?.createdAt;
      } catch {
        /* the proof is still usable without the role title */
      }

      setProof({
        id: result.id,
        targetRole,
        roleSummary,
        createdAt,
        filename: result.filename,
        gaps: result.gaps,
        whatChanged: result.whatChanged,
        hasCoverLetter: result.coverLetter,
        coverFilename: result.coverFilename,
        emailed: result.emailed,
        fit: result.fit,
        coverage: result.coverage,
        proseWarnings: result.proseWarnings,
      });
      setStatus("done");

      if (coverLetter && !result.coverLetter) {
        toast("Your resume is ready, but the cover letter didn't work out.", "error");
      } else {
        toast("Your resume is ready.", "success");
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "That didn't work. Give it another go.");
      setStatus("failed");
    }
  }

  function reset() {
    setRole("");
    setProof(null);
    setError(null);
    setStatus("idle");
    textareaRef.current?.focus();
  }

  const hasPane = status !== "idle";

  return (
    <>
      <PageHeader
        title="Compose"
        meta={
          profile
            ? `Using your ${profile.itemCount} entries and ${profile.skillCount} skills`
            : undefined
        }
        actions={
          hasPane ? (
            <Button size="sm" onClick={reset}>
              <RotateCcw size={13} />
              Start over
            </Button>
          ) : null
        }
      />

      <div
        className={cx(
          "gap-6 px-5 py-6 sm:px-7",
          hasPane ? "lg:grid lg:grid-cols-[minmax(0,26rem)_minmax(0,1fr)] lg:items-start" : "",
        )}
      >
        {/* Composer */}
        <section className={cx(!hasPane && "mx-auto w-full max-w-[46rem]")}>
          <label htmlFor="role" className="eyebrow block">
            The job
          </label>

          <textarea
            id="role"
            ref={textareaRef}
            value={role}
            onChange={(e) => setRole(e.target.value)}
            onKeyDown={(e) => {
              if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
                e.preventDefault();
                compose();
              }
            }}
            placeholder="Paste the job ad here, or a link to it."
            spellCheck={false}
            disabled={composing}
            className={cx(
              "mt-2 w-full rounded-[var(--radius-panel)] border border-line bg-raised p-4",
              "text-[13.5px] leading-relaxed outline-none transition-colors duration-[130ms]",
              "focus:border-ink disabled:cursor-not-allowed disabled:opacity-60",
              hasPane ? "h-32 sm:h-[13rem]" : "h-40 sm:h-[19rem]",
            )}
          />

          {looksLikeURL(role) && (
            <p className="mt-2 flex items-center gap-1.5 text-[12.5px] text-fg-muted">
              <Link2 size={13} className="shrink-0 text-ink" />
              cvx will open this link and read the job ad.
            </p>
          )}

          <div className="mt-4 grid grid-cols-2 gap-2">
            <Toggle
              checked={coverLetter}
              onChange={setCoverLetter}
              label="Cover letter"
              hint="A one page letter to go with it"
              disabled={composing}
            />
            <Toggle
              checked={recruiterEmail}
              onChange={setRecruiterEmail}
              label="Recruiter email"
              hint="An email you can forward, files attached"
              disabled={composing}
            />
          </div>

          <div className="mt-5 flex items-center gap-3">
            <Button
              variant="primary"
              onClick={compose}
              disabled={!canCompose}
              loading={composing}
            >
              {composing ? "Working on it" : "Compose my resume"}
            </Button>
            <kbd className="num hidden rounded border border-line px-1.5 py-0.5 text-[10.5px] text-fg-faint sm:block">
              ⌘↵
            </kbd>
          </div>
        </section>

        {/* Result pane */}
        {hasPane && (
          <section className="mt-8 lg:mt-0">
            {status === "composing" && (
              <div className="panel flex min-h-[24rem] items-center justify-center px-6 py-14">
                <TypeCase
                  itemCount={profile?.itemCount ?? 0}
                  skillCount={profile?.skillCount ?? 0}
                />
              </div>
            )}

            {status === "failed" && error && (
              <div className="panel">
                <ErrorState message={error} onRetry={compose} />
              </div>
            )}

            {status === "done" && proof && <Proof data={proof} />}
          </section>
        )}
      </div>

      {!hasPane && (
        <div className="mx-auto max-w-[46rem] px-5 pb-10 sm:px-7">
          <Eyebrow>How it works</Eyebrow>
          <p className="mt-2 max-w-[60ch] text-[13.5px] leading-relaxed text-fg-muted">
            cvx reads the job ad, picks out the parts of your profile that fit, and
            lays it all out as a one page PDF. If the job asks for something you
            haven&rsquo;t done, it tells you instead of making it up.
          </p>
        </div>
      )}
    </>
  );
}

function Toggle({
  checked,
  onChange,
  label,
  hint,
  disabled,
}: {
  checked: boolean;
  onChange: (v: boolean) => void;
  label: string;
  hint: string;
  disabled?: boolean;
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className={cx(
        "rounded-[var(--radius-ctl)] border px-3 py-2.5 text-left transition-colors duration-[130ms]",
        checked ? "border-ink bg-ink-soft" : "border-line bg-raised",
        disabled
          ? "cursor-not-allowed opacity-45"
          : !checked && "hover:border-line-strong",
      )}
    >
      <span className="flex items-center gap-2">
        <span
          aria-hidden="true"
          className={cx(
            "flex h-3.5 w-3.5 shrink-0 items-center justify-center rounded-[3px] border",
            checked ? "border-ink bg-ink text-ink-contrast" : "border-line-strong",
          )}
        >
          {checked && (
            <svg width="9" height="9" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="4">
              <path d="m5 12 5 5L20 7" strokeLinecap="round" strokeLinejoin="round" />
            </svg>
          )}
        </span>
        <span className="text-[13px] font-medium">{label}</span>
      </span>
      <span className="mt-1 block text-[11.5px] leading-snug text-fg-muted">{hint}</span>
    </button>
  );
}
