"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ChevronDown, ChevronUp, Plus, RotateCcw, Undo2, Wand2, X } from "lucide-react";
import { api } from "@/lib/api";
import { cx } from "@/lib/format";
import type {
  AvailableContent,
  AvailableItem,
  PreviewResult,
  Provenance,
  Tailored,
} from "@/lib/types";
import { useToast } from "./Toast";
import { Button, Eyebrow, Spinner } from "./ui";

/** The content standard the pipeline enforces, shown here so the rules are
 *  visible while you type instead of applied silently on save. */
const LIMITS = {
  headlineChars: 110,
  summaryWords: [55, 75] as const,
  bulletWords: 34,
  items: [3, 8] as const,
  skills: [6, 16] as const,
};

/** Edit everything on a generated resume, with the real page beside it. The
 *  server re-validates against the profile guardrail, re-normalizes and
 *  re-renders on save, so nothing here can put a line on the page that does
 *  not trace back to the profile. */
export function TailorEditor({
  id,
  initial,
  onSaved,
  onCancel,
}: {
  id: string;
  initial: Tailored;
  onSaved: () => void;
  onCancel: () => void;
}) {
  const toast = useToast();
  const [t, setT] = useState<Tailored>(initial);
  const [history, setHistory] = useState<Tailored[]>([]);
  const [saving, setSaving] = useState(false);
  const [provenance, setProvenance] = useState<Provenance>({});
  const [available, setAvailable] = useState<AvailableContent | null>(null);
  const [adding, setAdding] = useState(false);
  const [rewriting, setRewriting] = useState<string | null>(null);
  const [preview, setPreview] = useState<PreviewResult | null>(null);
  const [previewing, setPreviewing] = useState(false);

  const dirty = useMemo(() => JSON.stringify(t) !== JSON.stringify(initial), [t, initial]);

  /** Every change goes through here, so undo is complete by construction:
   *  there is no way to mutate the resume that forgets to record the step. */
  const edit = useCallback((fn: (draft: Tailored) => void) => {
    setT((prev) => {
      const next = structuredClone(prev);
      fn(next);
      setHistory((h) => [...h.slice(-49), prev]);
      return next;
    });
  }, []);

  function undo() {
    setHistory((h) => {
      if (h.length === 0) return h;
      setT(h[h.length - 1]);
      return h.slice(0, -1);
    });
  }

  function reset() {
    setT(initial);
    setHistory([]);
  }

  // Where each line came from, and what the profile holds that this resume
  // does not use. Both are best effort: the editor works without either.
  useEffect(() => {
    let live = true;
    api
      .provenance(id)
      .then((p) => live && setProvenance(p))
      .catch(() => {});
    api
      .available(id)
      .then((a) => live && setAvailable(a))
      .catch(() => {});
    return () => {
      live = false;
    };
  }, [id]);

  // Leaving with unsaved edits used to lose them silently.
  useEffect(() => {
    if (!dirty) return;
    const warn = (e: BeforeUnloadEvent) => e.preventDefault();
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [dirty]);

  const save = useCallback(async () => {
    setSaving(true);
    try {
      await api.saveTailored(id, t);
      toast("Saved. The PDF is freshly rendered.", "success");
      onSaved();
    } catch (err) {
      toast(err instanceof Error ? err.message : "Couldn't save those edits", "error");
      setSaving(false);
    }
  }, [id, t, toast, onSaved]);

  const renderPreview = useCallback(async () => {
    setPreviewing(true);
    try {
      setPreview(await api.previewTailored(id, t));
    } catch (err) {
      toast(err instanceof Error ? err.message : "Couldn't render that", "error");
    } finally {
      setPreviewing(false);
    }
  }, [id, t, toast]);

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key === "s") {
        e.preventDefault();
        void save();
      } else if ((e.metaKey || e.ctrlKey) && e.key === "z") {
        e.preventDefault();
        undo();
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [save]);

  async function rewrite(si: number, ii: number, bi: number, instruction: string) {
    const bullet = t.sections[si].items[ii].bullets[bi];
    setRewriting(bullet.sourceBulletId);
    try {
      const text = await api.rewriteBullet(id, bullet.sourceBulletId, bullet.text, instruction);
      edit((d) => {
        d.sections[si].items[ii].bullets[bi].text = text;
      });
    } catch (err) {
      toast(err instanceof Error ? err.message : "Couldn't rewrite that line", "error");
    } finally {
      setRewriting(null);
    }
  }

  function addBullet(item: AvailableItem, bulletId: string, text: string) {
    edit((d) => {
      for (const section of d.sections) {
        const target = section.items.find((it) => it.sourceId === item.sourceId);
        if (target) {
          target.bullets.push({ sourceBulletId: bulletId, text });
          return;
        }
      }
      // The item is not on the resume yet: bring it in with this bullet.
      const kind = sectionKindFor(item.kind);
      let section = d.sections.find((s) => s.kind === kind);
      if (!section) {
        section = { kind, title: sectionTitleFor(kind), items: [] };
        d.sections.push(section);
      }
      section.items.push({
        sourceId: item.sourceId,
        title: item.title,
        organization: item.organization,
        dates: item.dates,
        bullets: [{ sourceBulletId: bulletId, text }],
      });
    });
  }

  const usedBulletIds = useMemo(() => {
    const used = new Set<string>();
    for (const s of t.sections) {
      for (const it of s.items) {
        for (const b of it.bullets) used.add(b.sourceBulletId);
      }
    }
    return used;
  }, [t]);

  const itemCount = t.sections.reduce((n, s) => n + s.items.length, 0);
  const summaryWords = t.summary.trim() ? t.summary.trim().split(/\s+/).length : 0;

  return (
    <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,26rem)] lg:items-start">
      <div className="panel">
        <div className="flex flex-wrap items-center gap-2 border-b border-line px-4 py-3 sm:px-6">
          <Button variant="primary" size="sm" onClick={save} loading={saving}>
            Save and re-render
          </Button>
          <Button size="sm" onClick={renderPreview} loading={previewing}>
            Preview changes
          </Button>
          <Button size="sm" variant="ghost" onClick={undo} disabled={history.length === 0}>
            <Undo2 size={13} />
            Undo
          </Button>
          <Button size="sm" variant="ghost" onClick={reset} disabled={!dirty}>
            <RotateCcw size={13} />
            Reset
          </Button>
          <Button size="sm" variant="ghost" onClick={onCancel} disabled={saving} className="ml-auto">
            Cancel
          </Button>
        </div>

        <Field label="Job title" note="The resume is named after this, and so is the file.">
          <input
            value={t.targetRole}
            onChange={(e) => edit((d) => void (d.targetRole = e.target.value))}
            className={fieldClass("h-9")}
          />
        </Field>

        <Field label="Headline" note={`${t.headline.length} of ${LIMITS.headlineChars} characters`}>
          <input
            value={t.headline}
            onChange={(e) => edit((d) => void (d.headline = e.target.value))}
            maxLength={LIMITS.headlineChars}
            className={fieldClass("h-9")}
          />
        </Field>

        <Field
          label="Summary"
          note={`${summaryWords} words`}
          warn={summaryWords < LIMITS.summaryWords[0] || summaryWords > LIMITS.summaryWords[1]}
          warnNote={`the standard is ${LIMITS.summaryWords[0]} to ${LIMITS.summaryWords[1]}`}
        >
          <AutoTextarea
            value={t.summary}
            onChange={(v) => edit((d) => void (d.summary = v))}
            minRows={4}
          />
        </Field>

        {t.sections.map((section, si) => (
          <div key={`${section.kind}-${si}`} className="border-b border-line px-4 py-4 sm:px-6">
            <div className="flex items-center gap-2">
              <input
                value={section.title}
                onChange={(e) => edit((d) => void (d.sections[si].title = e.target.value))}
                aria-label="Section heading"
                className="eyebrow min-w-0 flex-1 bg-transparent outline-none"
              />
              <Move
                onUp={si > 0 ? () => edit((d) => swap(d.sections, si, si - 1)) : undefined}
                onDown={
                  si < t.sections.length - 1
                    ? () => edit((d) => swap(d.sections, si, si + 1))
                    : undefined
                }
              />
            </div>

            {section.items.map((item, ii) => (
              <div
                key={`${item.sourceId}-${ii}`}
                className={cx("mt-3", ii > 0 && "border-t border-line pt-4")}
              >
                <div className="flex items-baseline gap-2">
                  <p className="min-w-0 flex-1 text-[13.5px] font-medium">
                    {item.title}
                    {item.organization && (
                      <span className="text-fg-muted"> at {item.organization}</span>
                    )}
                    {item.dates && (
                      <span className="num ml-2 text-[11.5px] text-fg-faint">{item.dates}</span>
                    )}
                  </p>
                  <Move
                    onUp={
                      ii > 0 ? () => edit((d) => swap(d.sections[si].items, ii, ii - 1)) : undefined
                    }
                    onDown={
                      ii < section.items.length - 1
                        ? () => edit((d) => swap(d.sections[si].items, ii, ii + 1))
                        : undefined
                    }
                  />
                  <button
                    type="button"
                    onClick={() => edit((d) => removeItem(d, si, ii))}
                    className="num shrink-0 text-[11.5px] text-fg-faint transition-colors duration-[130ms] hover:text-missing"
                  >
                    Remove
                  </button>
                </div>

                <div className="mt-2 space-y-2">
                  {item.bullets.map((b, bi) => (
                    <BulletRow
                      key={`${b.sourceBulletId}-${bi}`}
                      text={b.text}
                      original={provenance[b.sourceBulletId]?.original}
                      busy={rewriting === b.sourceBulletId}
                      disabled={rewriting !== null}
                      canMoveUp={bi > 0}
                      canMoveDown={bi < item.bullets.length - 1}
                      onChange={(v) =>
                        edit((d) => void (d.sections[si].items[ii].bullets[bi].text = v))
                      }
                      onMoveUp={() => edit((d) => swap(d.sections[si].items[ii].bullets, bi, bi - 1))}
                      onMoveDown={() =>
                        edit((d) => swap(d.sections[si].items[ii].bullets, bi, bi + 1))
                      }
                      onRewrite={(instruction) => rewrite(si, ii, bi, instruction)}
                      onRemove={() =>
                        edit((d) => void d.sections[si].items[ii].bullets.splice(bi, 1))
                      }
                    />
                  ))}
                </div>
              </div>
            ))}
          </div>
        ))}

        <ChipField
          label="Skills"
          note={`${t.selectedSkills.length} on the resume`}
          warn={
            t.selectedSkills.length < LIMITS.skills[0] || t.selectedSkills.length > LIMITS.skills[1]
          }
          warnNote={`the standard is ${LIMITS.skills[0]} to ${LIMITS.skills[1]}`}
          selected={t.selectedSkills}
          options={available?.skills ?? []}
          onChange={(next) => edit((d) => void (d.selectedSkills = next))}
        />

        <ChipField
          label="Certifications"
          selected={t.certifications ?? []}
          options={available?.certifications ?? []}
          empty="Nothing in your profile yet. Add one from your account page."
          onChange={(next) => edit((d) => void (d.certifications = next))}
        />

        <ChipField
          label="Languages"
          selected={t.languages ?? []}
          options={available?.languages ?? []}
          empty="Nothing in your profile yet."
          onChange={(next) => edit((d) => void (d.languages = next))}
        />

        <ChipField
          label="Interests"
          note="Last on the page, first to go if it overflows."
          selected={t.interests ?? []}
          options={available?.interests ?? []}
          empty="Nothing in your profile yet."
          onChange={(next) => edit((d) => void (d.interests = next))}
        />

        <div className="border-b border-line px-4 py-4 sm:px-6">
          <div className="flex items-center gap-2">
            <Eyebrow className="flex-1">Not on this resume</Eyebrow>
            <span className="num text-[11.5px] text-fg-faint">
              {itemCount} of {LIMITS.items[1]} items used
            </span>
            <button
              type="button"
              onClick={() => setAdding((a) => !a)}
              className="text-[12px] font-medium text-ink transition-opacity duration-[130ms] hover:opacity-70"
            >
              {adding ? "Hide" : "Show"}
            </button>
          </div>

          {adding && (
            <div className="mt-3 space-y-3">
              {!available ? (
                <Spinner />
              ) : (
                <AddBack
                  items={available.items}
                  usedBulletIds={usedBulletIds}
                  onAdd={addBullet}
                />
              )}
            </div>
          )}
        </div>

        <p className="px-4 py-4 text-[11.5px] leading-relaxed text-fg-faint sm:px-6">
          Your words, your history. cvx still checks every line traces back to your profile, and
          trims the page from the bottom if it runs long.
        </p>
      </div>

      <PreviewPane preview={preview} loading={previewing} dirty={dirty} onRender={renderPreview} />
    </div>
  );
}

/* --------------------------------- pieces -------------------------------- */

function AddBack({
  items,
  usedBulletIds,
  onAdd,
}: {
  items: AvailableItem[];
  usedBulletIds: Set<string>;
  onAdd: (item: AvailableItem, bulletId: string, text: string) => void;
}) {
  const offers = items
    .map((item) => ({
      item,
      spare: item.bullets.filter((b) => !usedBulletIds.has(b.sourceBulletId)),
    }))
    .filter((o) => o.spare.length > 0);

  if (offers.length === 0) {
    return (
      <p className="text-[12.5px] text-fg-muted">
        Everything in your profile is already on this resume.
      </p>
    );
  }

  return (
    <>
      {offers.map(({ item, spare }) => (
        <div key={item.sourceId} className="border-t border-line pt-3 first:border-t-0 first:pt-0">
          <p className="text-[12.5px] font-medium">
            {item.title}
            {item.organization && <span className="text-fg-muted"> at {item.organization}</span>}
            {!item.onResume && (
              <span className="num ml-2 text-[11px] text-fg-faint">not on the page</span>
            )}
          </p>
          <ul className="mt-1.5 space-y-1.5">
            {spare.map((b) => (
              <li key={b.sourceBulletId} className="flex items-start gap-2">
                <button
                  type="button"
                  onClick={() => onAdd(item, b.sourceBulletId, b.text)}
                  aria-label="Add this line"
                  className="mt-[3px] flex h-5 w-5 shrink-0 items-center justify-center rounded text-fg-faint transition-colors duration-[130ms] hover:bg-ink-soft hover:text-ink"
                >
                  <Plus size={12} />
                </button>
                <span className="text-[12.5px] leading-relaxed text-fg-muted">{b.text}</span>
              </li>
            ))}
          </ul>
        </div>
      ))}
    </>
  );
}

function PreviewPane({
  preview,
  loading,
  dirty,
  onRender,
}: {
  preview: PreviewResult | null;
  loading: boolean;
  dirty: boolean;
  onRender: () => void;
}) {
  // The PDF arrives as base64 so one request answers both "how does it look"
  // and "does it fit"; a blob URL is what an iframe can actually display.
  const url = useMemo(() => {
    if (!preview) return null;
    const bytes = Uint8Array.from(atob(preview.pdf), (c) => c.charCodeAt(0));
    return URL.createObjectURL(new Blob([bytes], { type: "application/pdf" }));
  }, [preview]);

  useEffect(() => {
    if (!url) return;
    return () => URL.revokeObjectURL(url);
  }, [url]);

  return (
    <div className="panel lg:sticky lg:top-4">
      <div className="flex items-center gap-2 border-b border-line px-4 py-3">
        <Eyebrow className="flex-1">The page</Eyebrow>
        {preview && (
          <span className="num text-[11.5px] text-fg-faint">
            {Math.round(preview.fill * 100)}% full
          </span>
        )}
      </div>

      {url ? (
        <>
          <iframe
            src={url}
            title="Preview of your edits"
            className="h-[60vh] w-full border-b border-line bg-sunken lg:h-[70vh]"
          />
          <div className="space-y-2 px-4 py-3">
            {preview?.trimmed?.length ? (
              <div className="text-[12px] leading-relaxed text-fg-muted">
                <p className="font-medium text-fg">Trimmed to fit the page</p>
                <p className="mt-0.5">{preview.trimmed.join(", ")}</p>
              </div>
            ) : null}
            {preview?.pageAdvice?.length ? (
              <div className="text-[12px] leading-relaxed text-fg-muted">
                <p className="font-medium text-fg">The page ends early</p>
                <p className="mt-0.5">Try adding {preview.pageAdvice[0]}.</p>
              </div>
            ) : null}
            {dirty && (
              <button
                type="button"
                onClick={onRender}
                disabled={loading}
                className="text-[12px] font-medium text-ink transition-opacity duration-[130ms] hover:opacity-70"
              >
                {loading ? "Rendering" : "Refresh the preview"}
              </button>
            )}
          </div>
        </>
      ) : (
        <div className="px-4 py-6 text-center">
          <p className="text-[12.5px] leading-relaxed text-fg-muted">
            See the real page, and whether it fits, without saving.
          </p>
          <Button size="sm" className="mt-3" onClick={onRender} loading={loading}>
            Render the preview
          </Button>
        </div>
      )}
    </div>
  );
}

function BulletRow({
  text,
  original,
  busy,
  disabled,
  canMoveUp,
  canMoveDown,
  onChange,
  onMoveUp,
  onMoveDown,
  onRewrite,
  onRemove,
}: {
  text: string;
  original?: string;
  busy: boolean;
  disabled: boolean;
  canMoveUp: boolean;
  canMoveDown: boolean;
  onChange: (v: string) => void;
  onMoveUp: () => void;
  onMoveDown: () => void;
  onRewrite: (instruction: string) => void;
  onRemove: () => void;
}) {
  const [asking, setAsking] = useState(false);
  const [instruction, setInstruction] = useState("");

  const words = text.trim() ? text.trim().split(/\s+/).length : 0;
  const long = words > LIMITS.bulletWords;

  function ask() {
    onRewrite(instruction);
    setAsking(false);
    setInstruction("");
  }

  return (
    <div>
      <div className="flex items-start gap-2">
        <AutoTextarea value={text} onChange={onChange} minRows={2} />
        <div className="mt-1.5 flex shrink-0 flex-col gap-1">
          <button
            type="button"
            onClick={() => setAsking((a) => !a)}
            disabled={disabled}
            aria-label="Rewrite this line"
            title="Rewrite just this line"
            className={cx(
              "flex h-6 w-6 items-center justify-center rounded text-fg-faint transition-colors duration-[130ms]",
              busy ? "animate-pulse text-ink" : "hover:bg-ink-soft hover:text-ink",
              disabled && "cursor-not-allowed",
            )}
          >
            <Wand2 size={12} />
          </button>
          <Move
            onUp={canMoveUp ? onMoveUp : undefined}
            onDown={canMoveDown ? onMoveDown : undefined}
          />
          <button
            type="button"
            onClick={onRemove}
            aria-label="Remove bullet"
            className="flex h-6 w-6 items-center justify-center rounded text-fg-faint transition-colors duration-[130ms] hover:bg-missing-soft hover:text-missing"
          >
            <X size={12} />
          </button>
        </div>
      </div>

      {asking && (
        <div className="mt-1.5 flex items-center gap-2 pr-8">
          <input
            value={instruction}
            onChange={(e) => setInstruction(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && ask()}
            autoFocus
            placeholder="Optional: what should change? e.g. lead with the number"
            className={fieldClass("h-8 text-[12.5px]")}
          />
          <Button size="sm" variant="ghost" loading={busy} onClick={ask}>
            Rewrite
          </Button>
        </div>
      )}

      <div className="mt-1 flex flex-wrap gap-x-3 pr-8 text-[11.5px] leading-relaxed text-fg-faint">
        {long && (
          <span className="text-weak">
            {words} words, aim for {LIMITS.bulletWords} or fewer
          </span>
        )}
        {original && original.trim() !== text.trim() && <span>From your profile: {original}</span>}
      </div>
    </div>
  );
}

function ChipField({
  label,
  note,
  warn,
  warnNote,
  empty,
  selected,
  options,
  onChange,
}: {
  label: string;
  note?: string;
  warn?: boolean;
  warnNote?: string;
  empty?: string;
  selected: string[];
  options: string[];
  onChange: (next: string[]) => void;
}) {
  if (options.length === 0 && selected.length === 0) {
    if (!empty) return null;
    return (
      <div className="border-b border-line px-4 py-4 sm:px-6">
        <Eyebrow>{label}</Eyebrow>
        <p className="mt-1.5 text-[12.5px] text-fg-muted">{empty}</p>
      </div>
    );
  }

  const all = [...selected, ...options.filter((o) => !selected.includes(o))];

  return (
    <Field label={label} note={note} warn={warn} warnNote={warnNote}>
      <div className="flex flex-wrap gap-1.5">
        {all.map((value) => {
          const on = selected.includes(value);
          return (
            <button
              key={value}
              type="button"
              aria-pressed={on}
              onClick={() =>
                onChange(on ? selected.filter((s) => s !== value) : [...selected, value])
              }
              className={cx(
                "rounded-full border px-2.5 py-1 text-[12px] transition-colors duration-[130ms]",
                on
                  ? "border-ink bg-ink-soft text-ink"
                  : "border-line text-fg-muted hover:border-line-strong hover:text-fg",
              )}
            >
              {value}
            </button>
          );
        })}
      </div>
    </Field>
  );
}

function Field({
  label,
  note,
  warn,
  warnNote,
  children,
}: {
  label: string;
  note?: string;
  warn?: boolean;
  warnNote?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="border-b border-line px-4 py-4 sm:px-6">
      <div className="flex items-baseline gap-2">
        <Eyebrow className="flex-1">{label}</Eyebrow>
        {note && (
          <span className={cx("num text-[11.5px]", warn ? "text-weak" : "text-fg-faint")}>
            {note}
            {warn && warnNote ? `, ${warnNote}` : ""}
          </span>
        )}
      </div>
      <div className="mt-2">{children}</div>
    </div>
  );
}

function Move({ onUp, onDown }: { onUp?: () => void; onDown?: () => void }) {
  return (
    <div className="flex shrink-0 items-center">
      <button
        type="button"
        onClick={onUp}
        disabled={!onUp}
        aria-label="Move up"
        className="flex h-6 w-5 items-center justify-center rounded text-fg-faint transition-colors duration-[130ms] hover:text-fg disabled:opacity-30"
      >
        <ChevronUp size={13} />
      </button>
      <button
        type="button"
        onClick={onDown}
        disabled={!onDown}
        aria-label="Move down"
        className="flex h-6 w-5 items-center justify-center rounded text-fg-faint transition-colors duration-[130ms] hover:text-fg disabled:opacity-30"
      >
        <ChevronDown size={13} />
      </button>
    </div>
  );
}

/** A textarea that grows with its content: a fixed two rows meant every
 *  bullet longer than a line was edited through a keyhole. */
function AutoTextarea({
  value,
  onChange,
  minRows,
}: {
  value: string;
  onChange: (v: string) => void;
  minRows: number;
}) {
  const ref = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${el.scrollHeight}px`;
  }, [value]);

  return (
    <textarea
      ref={ref}
      value={value}
      rows={minRows}
      onChange={(e) => onChange(e.target.value)}
      className={cx(fieldClass(), "resize-none overflow-hidden")}
    />
  );
}

/* --------------------------------- helpers ------------------------------- */

function fieldClass(extra = "") {
  return cx(
    "w-full rounded-[var(--radius-ctl)] border border-line bg-raised px-3 py-2",
    "text-[13.5px] leading-relaxed outline-none transition-colors duration-[130ms] focus:border-ink",
    extra,
  );
}

function swap<T>(list: T[], a: number, b: number) {
  [list[a], list[b]] = [list[b], list[a]];
}

function removeItem(d: Tailored, si: number, ii: number) {
  d.sections[si].items.splice(ii, 1);
  d.sections = d.sections.filter((s) => s.items.length > 0);
}

/** Profile item kinds are free text; the resume's section kinds are not. */
function sectionKindFor(kind: string): string {
  switch (kind.toLowerCase()) {
    case "experience":
    case "work":
      return "experience";
    case "project":
    case "projects":
      return "projects";
    case "education":
      return "education";
    case "volunteering":
    case "volunteer":
      return "volunteering";
    case "certification":
    case "certifications":
      return "certifications";
    default:
      return "other";
  }
}

function sectionTitleFor(kind: string): string {
  switch (kind) {
    case "experience":
      return "Experience";
    case "projects":
      return "Projects";
    case "education":
      return "Education";
    case "volunteering":
      return "Volunteering";
    case "certifications":
      return "Certifications";
    default:
      return "More";
  }
}
