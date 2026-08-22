"use client";

import { useCallback, useState } from "react";
import { Check } from "lucide-react";
import { api } from "@/lib/api";
import { cx } from "@/lib/format";
import { useResource } from "@/lib/hooks";
import type { IntakeQuestion } from "@/lib/types";
import { useSession } from "./Session";
import { useToast } from "./Toast";
import { Button, Eyebrow } from "./ui";

/** The holes in a profile, asked about one at a time.
 *
 *  This is not a first-run feature. Most CVs are missing dates and numbers
 *  too, and those are the two things the guardrail forbids cvx from
 *  inventing, so the only way onto the page is to ask. The questions come
 *  from the profile's own shape, which is why there are never more than a
 *  handful and why they stop when there is nothing left to ask. */
export function IntakeQuestions({ compact = false }: { compact?: boolean }) {
  const fetcher = useCallback(() => api.profileQuestions(), []);
  const { data, error, reload } = useResource<IntakeQuestion[]>(fetcher);
  const [answered, setAnswered] = useState<string[]>([]);

  const open = (data ?? []).filter((q) => !answered.includes(q.id));

  if (error || data === undefined || open.length === 0) return null;

  return (
    <section>
      {!compact && <Eyebrow>Worth filling in</Eyebrow>}
      <div className={cx("panel divide-y divide-line", !compact && "mt-4")}>
        {!compact && (
          <p className="px-5 py-3.5 text-[12.5px] leading-relaxed text-fg-muted">
            cvx can only reorder and reword what it already has, so these are things it cannot work
            out for itself. Answer what you like and skip the rest.
          </p>
        )}
        {open.map((q) => (
          <QuestionRow
            key={q.id}
            question={q}
            onAnswered={() => {
              setAnswered((a) => [...a, q.id]);
              reload();
            }}
            onSkipped={() => setAnswered((a) => [...a, q.id])}
          />
        ))}
      </div>
    </section>
  );
}

function QuestionRow({
  question,
  onAnswered,
  onSkipped,
}: {
  question: IntakeQuestion;
  onAnswered: () => void;
  onSkipped: () => void;
}) {
  const toast = useToast();
  const { setProfile } = useSession();
  const [answer, setAnswer] = useState("");
  const [saving, setSaving] = useState(false);
  const [done, setDone] = useState(false);

  async function save() {
    setSaving(true);
    try {
      // The answer goes through the same path a typed note does, so it is
      // read into items and bullets rather than stored as loose text.
      await api.extendProfile(answer.trim(), question.ask);
      setProfile(await api.profile());
      setDone(true);
      toast("Added.", "success");
      onAnswered();
    } catch (err) {
      toast(err instanceof Error ? err.message : "Couldn't add that", "error");
    } finally {
      setSaving(false);
    }
  }

  if (done) {
    return (
      <p className="flex items-center gap-2 px-5 py-3 text-[12.5px] text-fg-muted">
        <Check size={13} className="text-good" />
        {question.ask}
      </p>
    );
  }

  return (
    <div className="px-5 py-4">
      <p className="text-[13px] font-medium">{question.ask}</p>
      <p className="mt-0.5 text-[11.5px] leading-snug text-fg-muted">{question.why}</p>
      <textarea
        value={answer}
        onChange={(e) => setAnswer(e.target.value)}
        rows={2}
        placeholder={question.placeholder}
        className={cx(
          "mt-2.5 w-full rounded-[var(--radius-ctl)] border border-line bg-surface px-3 py-2",
          "text-[13.5px] leading-relaxed outline-none transition-colors duration-[130ms] focus:border-ink",
        )}
      />
      <div className="mt-2 flex items-center gap-2">
        <Button size="sm" variant="primary" loading={saving} disabled={!answer.trim()} onClick={save}>
          Add
        </Button>
        <Button size="sm" variant="ghost" onClick={onSkipped} disabled={saving}>
          Skip
        </Button>
      </div>
    </div>
  );
}
