"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Plus, X } from "lucide-react";
import { api } from "@/lib/api";
import { useResource } from "@/lib/hooks";
import type { Certification, ProfileEdits } from "@/lib/types";
import { useSession } from "./Session";
import { useToast } from "./Toast";
import { Button, Eyebrow, Skeleton } from "./ui";

/** The house form idiom, copied rather than abstracted: this page and the
 *  settings below it are the only two forms in the app, and one shared
 *  primitive for two callers is a layer, not a saving. */
const INPUT =
  "mt-2 h-9 w-full rounded-[var(--radius-ctl)] border border-line bg-surface px-3 text-[13.5px] outline-none focus:border-ink";

/** The handful of facts cvx cannot read reliably off a resume, or that were
 *  never on one. Work history is not among them: it comes from the resume
 *  you upload and the notes you add above. */
export function ProfileDetails() {
  const fetcher = useCallback(() => api.profileEdits(), []);
  const { data, error, reload } = useResource<ProfileEdits>(fetcher);

  return (
    <section>
      <Eyebrow>Your details</Eyebrow>
      {error ? (
        <div className="panel mt-4 p-5">
          <p className="text-[13px] text-fg-muted">
            {error.toLowerCase().includes("no profile")
              ? "Upload a resume first and these fill in from it."
              : error}
          </p>
        </div>
      ) : data === undefined ? (
        <Skeleton className="mt-4 h-64 w-full" />
      ) : (
        <Editor initial={data} onSaved={reload} />
      )}
    </section>
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
    <div className="panel mt-4 divide-y divide-line">
      <div className="grid gap-4 p-5 sm:grid-cols-2">
        <div>
          <label htmlFor="name" className="eyebrow block">
            Name
          </label>
          <input
            id="name"
            value={p.name}
            onChange={(e) => edit((d) => void (d.name = e.target.value))}
            className={INPUT}
          />
        </div>
        <div>
          <label htmlFor="email" className="eyebrow block">
            Email
          </label>
          <input
            id="email"
            value={p.email}
            onChange={(e) => edit((d) => void (d.email = e.target.value))}
            className={INPUT}
          />
        </div>
        <div>
          <label htmlFor="phone" className="eyebrow block">
            Phone
          </label>
          <input
            id="phone"
            value={p.phone}
            onChange={(e) => edit((d) => void (d.phone = e.target.value))}
            className={INPUT}
          />
          <p className="mt-1.5 text-[11.5px] leading-snug text-fg-muted">
            Can be kept off the resume in settings.
          </p>
        </div>
        <div>
          <label htmlFor="location" className="eyebrow block">
            Location
          </label>
          <input
            id="location"
            value={p.location}
            onChange={(e) => edit((d) => void (d.location = e.target.value))}
            className={INPUT}
          />
          <p className="mt-1.5 text-[11.5px] leading-snug text-fg-muted">
            Can be kept off the resume in settings.
          </p>
        </div>
      </div>

      <div className="p-5">
        <p className="text-[13px] font-medium">Links</p>
        <p className="mt-0.5 text-[11.5px] leading-snug text-fg-muted">
          These three, and nothing else. A repo or a deployed project belongs to the work that cites
          it, not to the line under your name.
        </p>
        <div className="mt-3 grid gap-4 sm:grid-cols-3">
          <div>
            <label htmlFor="github" className="eyebrow block">
              GitHub
            </label>
            <input
              id="github"
              value={p.github}
              onChange={(e) => edit((d) => void (d.github = e.target.value))}
              placeholder="github.com/you"
              className={INPUT}
            />
          </div>
          <div>
            <label htmlFor="linkedin" className="eyebrow block">
              LinkedIn
            </label>
            <input
              id="linkedin"
              value={p.linkedin}
              onChange={(e) => edit((d) => void (d.linkedin = e.target.value))}
              placeholder="linkedin.com/in/you"
              className={INPUT}
            />
          </div>
          <div>
            <label htmlFor="portfolio" className="eyebrow block">
              Website
            </label>
            <input
              id="portfolio"
              value={p.portfolio}
              onChange={(e) => edit((d) => void (d.portfolio = e.target.value))}
              placeholder="you.dev"
              className={INPUT}
            />
          </div>
        </div>
      </div>

      <div className="p-5">
        <div className="flex items-start gap-3">
          <div className="min-w-0 flex-1">
            <p className="text-[13px] font-medium">Certifications</p>
            <p className="mt-0.5 text-[11.5px] leading-snug text-fg-muted">
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

        {(p.certifications ?? []).length === 0 ? (
          <p className="mt-2 text-[12.5px] text-fg-faint">None yet.</p>
        ) : (
          <div className="mt-3 space-y-2">
            {(p.certifications ?? []).map((cert, i) => (
              <CertRow
                key={i}
                cert={cert}
                onChange={(next) => edit((d) => void ((d.certifications ??= [])[i] = next))}
                onRemove={() => edit((d) => void d.certifications?.splice(i, 1))}
              />
            ))}
          </div>
        )}
      </div>

      <TagList
        title="Languages"
        note="Spoken languages. Programming languages are skills."
        placeholder="e.g. French (fluent)"
        values={p.languages ?? []}
        onChange={(next) => edit((d) => void (d.languages = next))}
      />

      <TagList
        title="Interests"
        note="Last on the page, and only when there is room for them."
        placeholder="e.g. Open source maintainer"
        values={p.interests ?? []}
        onChange={(next) => edit((d) => void (d.interests = next))}
      />

      <div className="flex items-center gap-2 p-5">
        <Button variant="primary" size="sm" onClick={save} loading={saving} disabled={!dirty}>
          Save details
        </Button>
        <Button size="sm" variant="ghost" onClick={() => setP(initial)} disabled={!dirty || saving}>
          Reset
        </Button>
        <p className="ml-auto text-[11.5px] text-fg-faint">{dirty ? "Unsaved changes" : "Saved"}</p>
      </div>
    </div>
  );
}

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
    <div className="flex flex-wrap items-center gap-2">
      <input
        value={cert.name}
        onChange={(e) => onChange({ ...cert, name: e.target.value })}
        placeholder="Name"
        className={`${INPUT} mt-0 min-w-0 flex-1`}
      />
      <input
        value={cert.issuer}
        onChange={(e) => onChange({ ...cert, issuer: e.target.value })}
        placeholder="Issued by"
        className={`${INPUT} mt-0 w-full sm:w-44`}
      />
      <input
        value={cert.year}
        onChange={(e) => onChange({ ...cert, year: e.target.value })}
        placeholder="Year"
        className={`${INPUT} mt-0 w-20`}
      />
      <button
        type="button"
        onClick={onRemove}
        aria-label={`Remove ${cert.name || "certification"}`}
        className="flex h-8 w-8 shrink-0 items-center justify-center rounded text-fg-faint transition-colors duration-[130ms] hover:bg-missing-soft hover:text-missing"
      >
        <X size={13} />
      </button>
    </div>
  );
}

function TagList({
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
    <div className="p-5">
      <p className="text-[13px] font-medium">{title}</p>
      <p className="mt-0.5 text-[11.5px] leading-snug text-fg-muted">{note}</p>

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
          className={`${INPUT} mt-0 min-w-0 flex-1`}
        />
        <Button size="sm" variant="ghost" onClick={add} disabled={!draft.trim()}>
          Add
        </Button>
      </div>
    </div>
  );
}
