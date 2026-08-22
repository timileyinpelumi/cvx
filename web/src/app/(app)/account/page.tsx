"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { ExternalLink, LogOut, Plus, Trash2, Upload } from "lucide-react";
import { api, previewURL } from "@/lib/api";
import { cx } from "@/lib/format";
import { ACCENTS, THEMES, type ResumeDensity, type ResumeStyle, type Settings } from "@/lib/types";
import { useSession } from "@/components/Session";
import { useToast } from "@/components/Toast";
import { ProfileDetails } from "@/components/ProfileDetails";
import { Button, Eyebrow, PageHeader, Skeleton } from "@/components/ui";

export default function AccountPage() {
  return (
    <>
      <PageHeader title="Account" meta="Everything cvx knows about you" />
      <div className="mx-auto max-w-[46rem] space-y-8 px-5 py-6 sm:px-7">
        <RecordSection />
        <ProfileDetails />
        <PreferencesSection />
        <SessionSection />
      </div>
    </>
  );
}

/* ----------------------------------- Record -------------------------------- */

function RecordSection() {
  const { profile, setProfile } = useSession();
  const toast = useToast();
  const inputRef = useRef<HTMLInputElement>(null);
  const [uploading, setUploading] = useState(false);
  const [note, setNote] = useState("");
  const [context, setContext] = useState("");
  const [adding, setAdding] = useState(false);
  const [showAdd, setShowAdd] = useState(false);
  const [historyCount, setHistoryCount] = useState(0);
  const [restoring, setRestoring] = useState(false);

  const refreshHistory = useCallback(() => {
    api
      .profileHistory()
      .then((h) => setHistoryCount(h.count))
      .catch(() => setHistoryCount(0));
  }, []);
  useEffect(refreshHistory, [refreshHistory]);

  async function restore() {
    setRestoring(true);
    try {
      const summary = await api.restoreProfile();
      setProfile(summary);
      refreshHistory();
      toast("Your previous profile is back.", "success");
    } catch (err) {
      toast(err instanceof Error ? err.message : "Couldn't restore that", "error");
    } finally {
      setRestoring(false);
    }
  }

  async function replace(file: File) {
    setUploading(true);
    try {
      const summary = await api.uploadProfile(file);
      setProfile(summary);
      refreshHistory();
      toast(`Updated. ${summary.itemCount} entries and ${summary.skillCount} skills.`, "success");
    } catch (err) {
      toast(err instanceof Error ? err.message : "That upload didn't work", "error");
    } finally {
      setUploading(false);
    }
  }

  async function add() {
    if (!note.trim()) return;
    setAdding(true);
    try {
      const summary = await api.extendProfile(note.trim(), context.trim());
      setProfile(summary);
      refreshHistory();
      setNote("");
      setContext("");
      setShowAdd(false);
      toast(`Added. Your profile now has ${summary.itemCount} entries.`, "success");
    } catch (err) {
      toast(err instanceof Error ? err.message : "Couldn't add that", "error");
    } finally {
      setAdding(false);
    }
  }

  return (
    <section>
      <Eyebrow>Your profile</Eyebrow>
      <p className="mt-2 max-w-[62ch] text-[13.5px] leading-relaxed text-fg-muted">
        Everything cvx knows about your work. Every resume it makes comes from here.
      </p>

      <div className="panel mt-4 p-5">
        <p className="font-display text-[16px] font-bold tracking-[-0.015em]">{profile?.name}</p>
        <p className="num mt-1 text-[12.5px] text-fg-muted">
          {profile?.itemCount} entries and {profile?.skillCount} skills
        </p>

        <div className="mt-4 flex flex-wrap gap-2">
          <Button size="sm" loading={uploading} onClick={() => inputRef.current?.click()}>
            <Upload size={13} />
            Upload a new resume
          </Button>
          <Button size="sm" variant="ghost" onClick={() => setShowAdd((s) => !s)}>
            <Plus size={13} />
            Add something
          </Button>
          <input
            ref={inputRef}
            type="file"
            accept="application/pdf,.pdf"
            className="sr-only"
            onChange={(e) => {
              const file = e.target.files?.[0];
              if (file) replace(file);
              e.target.value = "";
            }}
          />
        </div>

        {historyCount > 0 && (
          <p className="mt-3 text-[11.5px] text-fg-faint">
            Changed something by mistake?{" "}
            <button
              type="button"
              onClick={restore}
              disabled={restoring}
              className="underline underline-offset-2 transition-colors duration-[130ms] hover:text-fg disabled:opacity-50"
            >
              {restoring ? "Restoring" : "Restore the previous version"}
            </button>
          </p>
        )}

        {showAdd && (
          <div className="mt-5 border-t border-line pt-4">
            <label htmlFor="note" className="eyebrow block">
              What did you do?
            </label>
            <textarea
              id="note"
              value={note}
              onChange={(e) => setNote(e.target.value)}
              placeholder="Rebuilt the billing system and cut invoice errors by about 40%."
              className="mt-2 h-24 w-full rounded-[var(--radius-ctl)] border border-line bg-surface p-3 text-[13.5px] leading-relaxed outline-none focus:border-ink"
            />

            <label htmlFor="context" className="eyebrow mt-3 block">
              Which job or project? <span className="normal-case tracking-normal">(optional)</span>
            </label>
            <input
              id="context"
              value={context}
              onChange={(e) => setContext(e.target.value)}
              placeholder="e.g. Acme, or the billing project"
              className="mt-2 h-9 w-full rounded-[var(--radius-ctl)] border border-line bg-surface px-3 text-[13.5px] outline-none focus:border-ink"
            />

            <div className="mt-3 flex gap-2">
              <Button size="sm" variant="primary" loading={adding} disabled={!note.trim()} onClick={add}>
                Add to profile
              </Button>
              <Button size="sm" variant="ghost" onClick={() => setShowAdd(false)} disabled={adding}>
                Cancel
              </Button>
            </div>
          </div>
        )}
      </div>
    </section>
  );
}

/* --------------------------------- Preferences ----------------------------- */

function PreferencesSection() {
  const toast = useToast();
  const [settings, setSettings] = useState<Settings | null>(null);
  const [saving, setSaving] = useState(false);
  const style = settings?.resumeStyle ?? null;

  useEffect(() => {
    api
      .settings()
      .then(setSettings)
      .catch((err) => toast(err instanceof Error ? err.message : "Couldn't load preferences", "error"));
  }, [toast]);

  const saveAll = useCallback(
    async (next: Settings) => {
      const previous = settings;
      setSettings(next); // optimistic — the controls stay responsive
      setSaving(true);
      try {
        setSettings(await api.saveSettings(next));
      } catch (err) {
        setSettings(previous);
        toast(err instanceof Error ? err.message : "Couldn't save", "error");
      } finally {
        setSaving(false);
      }
    },
    [settings, toast],
  );

  const save = useCallback(
    (nextStyle: ResumeStyle) => {
      if (settings) saveAll({ ...settings, resumeStyle: nextStyle });
    },
    [settings, saveAll],
  );

  if (!settings || !style) {
    return (
      <section>
        <Eyebrow>How your resume looks</Eyebrow>
        <Skeleton className="mt-4 h-52 w-full" />
      </section>
    );
  }

  return (
    <section>
      <div className="flex items-baseline gap-3">
        <Eyebrow className="flex-1">How your resume looks</Eyebrow>
        {saving && <span className="num text-[11px] text-fg-faint">saving</span>}
      </div>

      <div className="panel mt-4 divide-y divide-line">
        <div className="p-5">
          <p className="text-[13px] font-medium">Layout</p>
          <div className="mt-3 grid gap-2 sm:grid-cols-3">
            {THEMES.map((t) => (
              <button
                key={t.value}
                type="button"
                onClick={() => save({ ...style, theme: t.value })}
                aria-pressed={style.theme === t.value}
                className={cx(
                  "rounded-[var(--radius-ctl)] border px-3 py-2.5 text-left transition-colors duration-[130ms]",
                  style.theme === t.value
                    ? "border-ink bg-ink-soft"
                    : "border-line hover:border-line-strong",
                )}
              >
                <span className="block text-[13px] font-medium">{t.name}</span>
                <span className="mt-0.5 block text-[11.5px] leading-snug text-fg-muted">{t.note}</span>
              </button>
            ))}
          </div>
        </div>

        <div className="p-5">
          <p className="text-[13px] font-medium">Colour</p>
          <p className="mt-0.5 text-[12px] text-fg-muted">The one colour used on your resume.</p>
          <div className="mt-3 flex flex-wrap gap-2">
            {ACCENTS.map((a) => (
              <button
                key={a.value}
                type="button"
                onClick={() => save({ ...style, accent: a.value })}
                aria-label={a.name}
                aria-pressed={style.accent === a.value}
                title={a.name}
                className={cx(
                  "h-8 w-8 rounded-full border-2 transition-transform duration-[130ms]",
                  style.accent === a.value
                    ? "border-fg scale-110"
                    : "border-transparent hover:scale-105",
                )}
                style={{ background: a.value }}
              />
            ))}
          </div>
        </div>

        <div className="p-5">
          <p className="text-[13px] font-medium">Spacing</p>
          <div className="mt-3 flex gap-2">
            {(["normal", "tight"] as ResumeDensity[]).map((d) => (
              <button
                key={d}
                type="button"
                onClick={() => save({ ...style, density: d })}
                aria-pressed={style.density === d}
                className={cx(
                  "h-8 rounded-[var(--radius-ctl)] border px-3 text-[13px] font-medium capitalize",
                  "transition-colors duration-[130ms]",
                  style.density === d ? "border-ink bg-ink-soft text-ink" : "border-line hover:border-line-strong",
                )}
              >
                {d}
              </button>
            ))}
          </div>
        </div>

        <label className="flex cursor-pointer items-start gap-3 p-5">
          <input
            type="checkbox"
            checked={style.skillsFirst}
            onChange={(e) => save({ ...style, skillsFirst: e.target.checked })}
            className="mt-0.5 h-3.5 w-3.5 shrink-0 accent-[var(--ink)]"
          />
          <span>
            <span className="block text-[13px] font-medium">Put skills before experience</span>
            <span className="mt-0.5 block text-[12px] text-fg-muted">
              Good when the job cares most about which tools you know.
            </span>
          </span>
        </label>

        <label className="flex cursor-pointer items-start gap-3 p-5">
          <input
            type="checkbox"
            checked={style.hidePhone}
            onChange={(e) => save({ ...style, hidePhone: e.target.checked })}
            className="mt-0.5 h-3.5 w-3.5 shrink-0 accent-[var(--ink)]"
          />
          <span>
            <span className="block text-[13px] font-medium">Keep my phone number off the resume</span>
            <span className="mt-0.5 block text-[12px] text-fg-muted">
              Worth it when the resume goes on a job board. Your email still shows.
            </span>
          </span>
        </label>

        <label className="flex cursor-pointer items-start gap-3 p-5">
          <input
            type="checkbox"
            checked={style.hideLocation}
            onChange={(e) => save({ ...style, hideLocation: e.target.checked })}
            className="mt-0.5 h-3.5 w-3.5 shrink-0 accent-[var(--ink)]"
          />
          <span>
            <span className="block text-[13px] font-medium">Keep my location off the resume</span>
            <span className="mt-0.5 block text-[12px] text-fg-muted">
              Some people would rather not say where they live until later.
            </span>
          </span>
        </label>

        <label className="flex cursor-pointer items-start gap-3 p-5">
          <input
            type="checkbox"
            checked={settings.emailCopy}
            onChange={(e) => saveAll({ ...settings, emailCopy: e.target.checked })}
            className="mt-0.5 h-3.5 w-3.5 shrink-0 accent-[var(--ink)]"
          />
          <span>
            <span className="block text-[13px] font-medium">Email me a copy</span>
            <span className="mt-0.5 block text-[12px] text-fg-muted">
              Every resume you make also lands in your inbox, files attached.
            </span>
          </span>
        </label>

        <label className="flex cursor-pointer items-start gap-3 p-5">
          <input
            type="checkbox"
            checked={settings.recruiterAuto}
            onChange={(e) => saveAll({ ...settings, recruiterAuto: e.target.checked })}
            className="mt-0.5 h-3.5 w-3.5 shrink-0 accent-[var(--ink)]"
          />
          <span>
            <span className="block text-[13px] font-medium">Recruiter email on every resume</span>
            <span className="mt-0.5 block text-[12px] text-fg-muted">
              Compose starts with the recruiter email switched on, so the forwardable
              version arrives without asking each time.
            </span>
          </span>
        </label>

        <div className="p-5">
          <a href={previewURL()} target="_blank" rel="noopener noreferrer">
            <Button size="sm">
              <ExternalLink size={13} />
              See a preview
            </Button>
          </a>
          <p className="mt-2 text-[12px] text-fg-muted">
            Opens a sample PDF using your own profile, so you can see how it looks.
          </p>
        </div>
      </div>

      <div className="mt-8 flex items-baseline gap-3">
        <Eyebrow className="flex-1">How it writes</Eyebrow>
      </div>
      <p className="mt-2 max-w-[62ch] text-[13.5px] leading-relaxed text-fg-muted">
        These shape the words on every resume it makes from now on. Everything
        still has to come from your profile.
      </p>

      <div className="panel mt-4 divide-y divide-line">
        <WritingKnob
          label="Voice"
          options={[
            { value: "plain", label: "Plain", note: "Says it straight" },
            { value: "confident", label: "Confident", note: "Leans on stronger verbs" },
          ]}
          current={settings.generation.tone}
          onPick={(v) => saveAll({ ...settings, generation: { ...settings.generation, tone: v as Settings["generation"]["tone"] } })}
        />
        <WritingKnob
          label="Summary"
          options={[
            { value: "standard", label: "Standard", note: "Up to 60 words" },
            { value: "short", label: "Short", note: "35 words at most" },
            { value: "none", label: "None", note: "Skip the summary" },
          ]}
          current={settings.generation.summary}
          onPick={(v) => saveAll({ ...settings, generation: { ...settings.generation, summary: v as Settings["generation"]["summary"] } })}
        />
        <WritingKnob
          label="Bullets"
          options={[
            { value: "full", label: "Full", note: "3 to 4 per role" },
            { value: "lean", label: "Lean", note: "2 to 3, strongest only" },
          ]}
          current={settings.generation.bullets}
          onPick={(v) => saveAll({ ...settings, generation: { ...settings.generation, bullets: v as Settings["generation"]["bullets"] } })}
        />
      </div>
    </section>
  );
}

function WritingKnob({
  label,
  options,
  current,
  onPick,
}: {
  label: string;
  options: { value: string; label: string; note: string }[];
  current: string;
  onPick: (value: string) => void;
}) {
  return (
    <div className="p-5">
      <p className="text-[13px] font-medium">{label}</p>
      <div className="mt-3 grid gap-2 sm:grid-cols-3">
        {options.map((o) => (
          <button
            key={o.value}
            type="button"
            onClick={() => onPick(o.value)}
            aria-pressed={current === o.value}
            className={cx(
              "rounded-[var(--radius-ctl)] border px-3 py-2.5 text-left transition-colors duration-[130ms]",
              current === o.value ? "border-ink bg-ink-soft" : "border-line hover:border-line-strong",
            )}
          >
            <span className="block text-[13px] font-medium">{o.label}</span>
            <span className="mt-0.5 block text-[11.5px] leading-snug text-fg-muted">{o.note}</span>
          </button>
        ))}
      </div>
    </div>
  );
}

/* ---------------------------------- Session -------------------------------- */

function SessionSection() {
  const { me } = useSession();
  const router = useRouter();
  const toast = useToast();
  const [busy, setBusy] = useState(false);
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const [deleting, setDeleting] = useState(false);

  async function signOut() {
    setBusy(true);
    await api.logout().catch(() => {});
    router.replace("/signin");
  }

  async function deleteAccount() {
    setDeleting(true);
    try {
      await api.deleteAccount();
      router.replace("/signin");
    } catch (err) {
      toast(err instanceof Error ? err.message : "Couldn't delete your account", "error");
      setDeleting(false);
      setConfirmingDelete(false);
    }
  }

  return (
    <section>
      <Eyebrow>Signed in</Eyebrow>
      <div className="panel mt-4 divide-y divide-line">
        <div className="flex flex-wrap items-center gap-3 p-5">
          <div className="min-w-0 flex-1">
            <p className="truncate text-[13px] font-medium">{me?.email}</p>
            <p className="mt-0.5 text-[11.5px] capitalize text-fg-faint">via {me?.provider}</p>
          </div>
          <Button size="sm" variant="ghost" loading={busy} onClick={signOut}>
            <LogOut size={13} />
            Sign out
          </Button>
        </div>

        <div className="flex flex-col gap-3 p-5 sm:flex-row sm:items-center">
          <div className="min-w-0 flex-1">
            <p className="text-[13px] font-medium">Delete account</p>
            <p className="mt-0.5 text-[11.5px] leading-snug text-fg-muted">
              Closes your account and signs you out. Signing in again with this email starts you
              over with a blank profile.
            </p>
          </div>
          <Button
            size="sm"
            variant="danger"
            className="w-full sm:w-auto"
            onClick={() => setConfirmingDelete(true)}
          >
            <Trash2 size={13} />
            Delete account
          </Button>
        </div>
      </div>

      {confirmingDelete && me && (
        <DeleteAccountDialog
          email={me.email}
          deleting={deleting}
          onConfirm={deleteAccount}
          onClose={() => setConfirmingDelete(false)}
        />
      )}
    </section>
  );
}

function DeleteAccountDialog({
  email,
  deleting,
  onConfirm,
  onClose,
}: {
  email: string;
  deleting: boolean;
  onConfirm: () => void;
  onClose: () => void;
}) {
  const [typed, setTyped] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);
  const matches = typed.trim().toLowerCase() === email.toLowerCase();

  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  return (
    <div
      className="fixed inset-0 z-50 flex items-end justify-center overflow-y-auto bg-black/45 px-4 py-6 sm:items-center"
      onMouseDown={() => !deleting && onClose()}
      role="presentation"
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-label="Delete account"
        className="w-full max-w-md rounded-[var(--radius-panel)] border border-line-strong bg-raised p-5 shadow-2xl"
        onMouseDown={(e) => e.stopPropagation()}
        onKeyDown={(e) => {
          if (e.key === "Escape" && !deleting) onClose();
        }}
      >
        <p className="text-[14px] font-medium">Delete this account?</p>
        <p className="mt-1.5 text-[12.5px] leading-relaxed text-fg-muted">
          You will be signed out and this account will stop working. Signing in again with the same
          email gives you a new, empty account.
        </p>

        <label htmlFor="confirm-email" className="mt-4 block text-[12px] text-fg-muted">
          Type <span className="font-medium text-fg">{email}</span> to confirm
        </label>
        <input
          id="confirm-email"
          ref={inputRef}
          value={typed}
          onChange={(e) => setTyped(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && matches && !deleting) onConfirm();
          }}
          autoComplete="off"
          autoCapitalize="none"
          spellCheck={false}
          disabled={deleting}
          placeholder={email}
          className="mt-2 h-9 w-full rounded-[var(--radius-ctl)] border border-line bg-surface px-3 text-[13.5px] outline-none focus:border-ink"
        />

        <div className="mt-5 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
          <Button size="sm" variant="ghost" onClick={onClose} disabled={deleting}>
            Cancel
          </Button>
          <Button
            size="sm"
            variant="danger"
            onClick={onConfirm}
            loading={deleting}
            disabled={!matches}
          >
            Delete account
          </Button>
        </div>
      </div>
    </div>
  );
}
