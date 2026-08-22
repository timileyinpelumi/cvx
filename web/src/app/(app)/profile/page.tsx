"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { Plus, X } from "lucide-react";
import { api } from "@/lib/api";
import { cx } from "@/lib/format";
import { useResource } from "@/lib/hooks";
import type { Certification, ProfileEdits } from "@/lib/types";
import { useSession } from "@/components/Session";
import { useToast } from "@/components/Toast";
import { Button, EmptyState, ErrorState, Eyebrow, PageHeader, RowSkeleton } from "@/components/ui";

/** The handful of facts cvx cannot read reliably off a resume, or that were
 *  never on one. Not a CV manager: your work history comes from the resume
 *  you upload and the notes you add, and every resume writes its own
 *  summary. */
export default function ProfilePage() {
  const fetcher = useCallback(() => api.profileEdits(), []);
  const { data, error, reload } = useResource<ProfileEdits>(fetcher);

  return (
    <>
      <PageHeader title="Profile" meta="Your details, and what fills a short page" />
      <div className="mx-auto max-w-[46rem] px-4 py-5 sm:px-7 sm:py-6">
        {error ? (
          error.toLowerCase().includes("no profile") ? (
            <EmptyState
              title="No profile yet"
              body="Upload a resume on your account page. cvx reads it into a profile, and this is where you fix what it could not."
              action={
                <Link href="/account">
                  <Button variant="primary">Go to account</Button>
                </Link>
              }
            />
          ) : (
            <ErrorState message={error} onRetry={reload} />
          )
        ) : data === undefined ? (
          <RowSkeleton rows={6} />
        ) : (
          <Editor initial={data} onSaved={reload} />
        )}
      </div>
    </>
  );
}

function Editor({ initial, onSaved }: { initial: ProfileEdits; onSaved: () => void }) {
  const toast = useToast();
  const { setProfile } = useSession();
  const [p, setP] = useState<ProfileEdits>(initial);
  const [saving, setSaving] = useState(false);

  const dirty = useMemo(() => JSON.stringify(p) !== JSON.stringify(initial), [p, initial]);

  useEffect(() => {
    if (!dirty) return;
    const warn = (e: BeforeUnloadEvent) => e.preventDefault();
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [dirty]);

  function edit(fn: (draft: ProfileEdits) => void) {
    setP((prev) => {
      const next = structuredClone(prev);
      fn(next);
      return next;
    });
  }

  async function save() {
    setSaving(true);
    try {
      setP(await api.saveProfileEdits(p));
      setProfile(await api.profile());
      toast("Saved. New resumes will use this.", "success");
      onSaved();
    } catch (err) {
      toast(err instanceof Error ? err.message : "Couldn't save that", "error");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="space-y-4">
      <div className="panel sticky top-0 z-10 flex items-center gap-2 px-4 py-3 sm:px-6">
        <Button variant="primary" size="sm" onClick={save} loading={saving} disabled={!dirty}>
          Save
        </Button>
        <Button size="sm" variant="ghost" onClick={() => setP(initial)} disabled={!dirty || saving}>
          Reset
        </Button>
        <p className="ml-auto text-[11.5px] text-fg-faint">{dirty ? "Unsaved changes" : "Saved"}</p>
      </div>

      <section className="panel divide-y divide-line">
        <Row label="Name">
          <input
            value={p.name}
            onChange={(e) => edit((d) => void (d.name = e.target.value))}
            className={field()}
          />
        </Row>
        <Row label="Email">
          <input
            value={p.email}
            onChange={(e) => edit((d) => void (d.email = e.target.value))}
            className={field()}
          />
        </Row>
        <Row label="Phone" note="Can be kept off the resume in settings.">
          <input
            value={p.phone}
            onChange={(e) => edit((d) => void (d.phone = e.target.value))}
            className={field()}
          />
        </Row>
        <Row label="Location" note="Can be kept off the resume in settings.">
          <input
            value={p.location}
            onChange={(e) => edit((d) => void (d.location = e.target.value))}
            className={field()}
          />
        </Row>
      </section>

      <section className="panel divide-y divide-line">
        <div className="px-4 py-3.5 sm:px-6">
          <Eyebrow>Links</Eyebrow>
          <p className="mt-1 text-[12px] leading-relaxed text-fg-muted">
            These three, and nothing else. A repo or a deployed project belongs to the work that
            cites it, not to the line under your name.
          </p>
        </div>
        <Row label="GitHub">
          <input
            value={p.github}
            onChange={(e) => edit((d) => void (d.github = e.target.value))}
            placeholder="github.com/you"
            className={field()}
          />
        </Row>
        <Row label="LinkedIn">
          <input
            value={p.linkedin}
            onChange={(e) => edit((d) => void (d.linkedin = e.target.value))}
            placeholder="linkedin.com/in/you"
            className={field()}
          />
        </Row>
        <Row label="Website">
          <input
            value={p.portfolio}
            onChange={(e) => edit((d) => void (d.portfolio = e.target.value))}
            placeholder="you.dev"
            className={field()}
          />
        </Row>
      </section>

      <section className="panel">
        <div className="flex items-start gap-3 border-b border-line px-4 py-3.5 sm:px-6">
          <div className="min-w-0 flex-1">
            <Eyebrow>Certifications</Eyebrow>
            <p className="mt-1 text-[12px] leading-relaxed text-fg-muted">
              Credentials with no bullets: a certificate, a licence, an award.
            </p>
          </div>
          <Button
            size="sm"
            variant="ghost"
            onClick={() =>
              edit((d) => {
                d.certifications = [...(d.certifications ?? []), { name: "", issuer: "", year: "" }];
              })
            }
          >
            <Plus size={13} />
            Add
          </Button>
        </div>
        <div className="divide-y divide-line">
          {(p.certifications ?? []).map((cert, i) => (
            <CertRow
              key={i}
              cert={cert}
              onChange={(next) => edit((d) => void ((d.certifications ??= [])[i] = next))}
              onRemove={() => edit((d) => void d.certifications?.splice(i, 1))}
            />
          ))}
          {(p.certifications ?? []).length === 0 && (
            <p className="px-4 py-4 text-[12.5px] text-fg-muted sm:px-6">None yet.</p>
          )}
        </div>
      </section>

      <ListSection
        title="Languages"
        note="Spoken languages. Programming languages are skills."
        placeholder="e.g. French (fluent)"
        values={p.languages ?? []}
        onChange={(next) => edit((d) => void (d.languages = next))}
      />

      <ListSection
        title="Interests"
        note="Last on the page, and only when there is room for them."
        placeholder="e.g. Open source maintainer"
        values={p.interests ?? []}
        onChange={(next) => edit((d) => void (d.interests = next))}
      />

      <p className="px-1 text-[11.5px] leading-relaxed text-fg-faint">
        Your work history comes from the resume you upload and the notes you add on your account
        page. Skills live on their own page.
      </p>
    </div>
  );
}

/* --------------------------------- pieces -------------------------------- */

function CertRow({
  cert,
  onChange,
  onRemove,
}: {
  cert: Certification;
  onChange: (next: Certification) => void;
  onRemove: () => void;
}) {
  return (
    <div className="flex flex-wrap items-center gap-2 px-4 py-3 sm:px-6">
      <input
        value={cert.name}
        onChange={(e) => onChange({ ...cert, name: e.target.value })}
        placeholder="Name"
        className={field("min-w-0 flex-1")}
      />
      <input
        value={cert.issuer}
        onChange={(e) => onChange({ ...cert, issuer: e.target.value })}
        placeholder="Issued by"
        className={field("w-full sm:w-44")}
      />
      <input
        value={cert.year}
        onChange={(e) => onChange({ ...cert, year: e.target.value })}
        placeholder="Year"
        className={field("w-20")}
      />
      <button
        type="button"
        onClick={onRemove}
        aria-label="Remove"
        className="flex h-8 w-8 shrink-0 items-center justify-center rounded text-fg-faint transition-colors duration-[130ms] hover:bg-missing-soft hover:text-missing"
      >
        <X size={13} />
      </button>
    </div>
  );
}

function ListSection({
  title,
  note,
  placeholder,
  values,
  onChange,
}: {
  title: string;
  note: string;
  placeholder: string;
  values: string[];
  onChange: (next: string[]) => void;
}) {
  const [draft, setDraft] = useState("");

  function add() {
    const value = draft.trim();
    if (!value) return;
    onChange([...values, value]);
    setDraft("");
  }

  return (
    <section className="panel px-4 py-4 sm:px-6">
      <Eyebrow>{title}</Eyebrow>
      <p className="mt-1 text-[12px] leading-relaxed text-fg-muted">{note}</p>

      {values.length > 0 && (
        <div className="mt-2.5 flex flex-wrap gap-1.5">
          {values.map((value, i) => (
            <span
              key={`${value}-${i}`}
              className="flex items-center gap-1.5 rounded-full border border-line px-2.5 py-1 text-[12px]"
            >
              {value}
              <button
                type="button"
                onClick={() => onChange(values.filter((_, j) => j !== i))}
                aria-label={`Remove ${value}`}
                className="text-fg-faint transition-colors duration-[130ms] hover:text-missing"
              >
                <X size={11} />
              </button>
            </span>
          ))}
        </div>
      )}

      <div className="mt-2.5 flex items-center gap-2">
        <input
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              add();
            }
          }}
          placeholder={placeholder}
          className={field("min-w-0 flex-1")}
        />
        <Button size="sm" variant="ghost" onClick={add} disabled={!draft.trim()}>
          Add
        </Button>
      </div>
    </section>
  );
}

function Row({
  label,
  note,
  children,
}: {
  label: string;
  note?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="px-4 py-3.5 sm:px-6">
      <div className="flex items-baseline gap-2">
        <Eyebrow className="flex-1">{label}</Eyebrow>
        {note && <span className="text-[11.5px] text-fg-faint">{note}</span>}
      </div>
      <div className="mt-1.5">{children}</div>
    </div>
  );
}

function field(extra = "") {
  return cx(
    "h-9 w-full rounded-[var(--radius-ctl)] border border-line bg-raised px-3",
    "text-[13.5px] outline-none transition-colors duration-[130ms] focus:border-ink",
    extra,
  );
}
