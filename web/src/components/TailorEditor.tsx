"use client";

import { useEffect, useState } from "react";
import { Wand2, X } from "lucide-react";
import { api } from "@/lib/api";
import { cx } from "@/lib/format";
import type { Provenance, Tailored } from "@/lib/types";
import { useToast } from "./Toast";
import { Button, Eyebrow } from "./ui";

/** Edit the words on a generated resume: headline, summary, and bullet text,
 *  with bullets and whole items removable. The server re-validates against
 *  the profile guardrail and re-renders the PDF on save. */
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
  const [saving, setSaving] = useState(false);
  const [provenance, setProvenance] = useState<Provenance>({});
  const [rewriting, setRewriting] = useState<string | null>(null);

  // Where each line came from. Best effort: the editor works without it, so
  // a failed fetch just means no "from your profile" lines.
  useEffect(() => {
    let live = true;
    api
      .provenance(id)
      .then((p) => live && setProvenance(p))
      .catch(() => {});
    return () => {
      live = false;
    };
  }, [id]);

  function setBullet(si: number, ii: number, bi: number, text: string) {
    setT((prev) => {
      const next = structuredClone(prev);
      next.sections[si].items[ii].bullets[bi].text = text;
      return next;
    });
  }

  function removeBullet(si: number, ii: number, bi: number) {
    setT((prev) => {
      const next = structuredClone(prev);
      next.sections[si].items[ii].bullets.splice(bi, 1);
      return next;
    });
  }

  function removeItem(si: number, ii: number) {
    setT((prev) => {
      const next = structuredClone(prev);
      next.sections[si].items.splice(ii, 1);
      next.sections = next.sections.filter((s) => s.items.length > 0);
      return next;
    });
  }

  async function rewrite(si: number, ii: number, bi: number) {
    const bullet = t.sections[si].items[ii].bullets[bi];
    setRewriting(bullet.sourceBulletId);
    try {
      const text = await api.rewriteBullet(id, bullet.sourceBulletId, bullet.text);
      setBullet(si, ii, bi, text);
    } catch (err) {
      toast(err instanceof Error ? err.message : "Couldn't rewrite that line", "error");
    } finally {
      setRewriting(null);
    }
  }

  async function save() {
    setSaving(true);
    try {
      await api.saveTailored(id, t);
      toast("Saved. The PDF is freshly rendered.", "success");
      onSaved();
    } catch (err) {
      toast(err instanceof Error ? err.message : "Couldn't save those edits", "error");
      setSaving(false);
    }
  }

  const fieldClass = cx(
    "w-full rounded-[var(--radius-ctl)] border border-line bg-raised px-3 py-2",
    "text-[13.5px] leading-relaxed outline-none transition-colors duration-[130ms] focus:border-ink",
  );

  return (
    <div className="panel">
      <div className="border-b border-line px-4 py-4 sm:px-6">
        <Eyebrow>Headline</Eyebrow>
        <input
          value={t.headline}
          onChange={(e) => setT({ ...t, headline: e.target.value })}
          maxLength={110}
          className={cx(fieldClass, "mt-2 h-9")}
        />

        <Eyebrow className="mt-4">Summary</Eyebrow>
        <textarea
          value={t.summary}
          onChange={(e) => setT({ ...t, summary: e.target.value })}
          className={cx(fieldClass, "mt-2 h-24")}
        />
      </div>

      {t.sections.map((section, si) => (
        <div key={section.title} className="border-b border-line px-4 py-4 sm:px-6">
          <Eyebrow>{section.title}</Eyebrow>
          {section.items.map((item, ii) => (
            <div key={item.sourceId} className={cx("mt-3", ii > 0 && "border-t border-line pt-4")}>
              <div className="flex items-baseline gap-3">
                <p className="min-w-0 flex-1 text-[13.5px] font-medium">
                  {item.title}
                  {item.organization && (
                    <span className="text-fg-muted"> at {item.organization}</span>
                  )}
                </p>
                <button
                  type="button"
                  onClick={() => removeItem(si, ii)}
                  className="num shrink-0 text-[11.5px] text-fg-faint transition-colors duration-[130ms] hover:text-missing"
                >
                  Remove
                </button>
              </div>

              <div className="mt-2 space-y-2">
                {item.bullets.map((b, bi) => {
                  const source = provenance[b.sourceBulletId];
                  return (
                    <div key={b.sourceBulletId}>
                      <div className="flex items-start gap-2">
                        <textarea
                          value={b.text}
                          onChange={(e) => setBullet(si, ii, bi, e.target.value)}
                          rows={2}
                          className={fieldClass}
                        />
                        <div className="mt-1.5 flex shrink-0 flex-col gap-1">
                          <button
                            type="button"
                            onClick={() => rewrite(si, ii, bi)}
                            disabled={rewriting !== null}
                            aria-label="Rewrite this line"
                            title="Rewrite just this line"
                            className={cx(
                              "flex h-6 w-6 items-center justify-center rounded text-fg-faint transition-colors duration-[130ms]",
                              rewriting === b.sourceBulletId
                                ? "animate-pulse text-ink"
                                : "hover:bg-ink-soft hover:text-ink",
                              rewriting !== null && "cursor-not-allowed",
                            )}
                          >
                            <Wand2 size={12} />
                          </button>
                          <button
                            type="button"
                            onClick={() => removeBullet(si, ii, bi)}
                            aria-label="Remove bullet"
                            className="flex h-6 w-6 items-center justify-center rounded text-fg-faint transition-colors duration-[130ms] hover:bg-missing-soft hover:text-missing"
                          >
                            <X size={12} />
                          </button>
                        </div>
                      </div>
                      {source && source.original.trim() !== b.text.trim() && (
                        <p className="mt-1 pr-8 text-[11.5px] leading-relaxed text-fg-faint">
                          From your profile: {source.original}
                        </p>
                      )}
                    </div>
                  );
                })}
              </div>
            </div>
          ))}
        </div>
      ))}

      <div className="flex items-center gap-2 px-4 py-4 sm:px-6">
        <Button variant="primary" size="sm" onClick={save} loading={saving}>
          Save and re-render
        </Button>
        <Button variant="ghost" size="sm" onClick={onCancel} disabled={saving}>
          Cancel
        </Button>
        <p className="ml-auto hidden text-[11.5px] text-fg-faint sm:block">
          Your words, your history. cvx still checks every line traces back to your profile.
        </p>
      </div>
    </div>
  );
}
