"use client";

import { useState } from "react";
import { Copy, Send } from "lucide-react";
import { api } from "@/lib/api";
import { daysAgo } from "@/lib/format";
import type { RecruiterEmail } from "@/lib/types";
import { useToast } from "./Toast";
import { Button, Eyebrow } from "./ui";

/** Offers the nudge for an application that was sent and has gone quiet.
 *  Renders nothing until it is actually reasonable to follow up, so the
 *  tracker is what decides when, not the user's patience. */
export function FollowUp({
  id,
  status,
  statusAt,
}: {
  id: string;
  status: string;
  statusAt: string;
}) {
  const toast = useToast();
  const [draft, setDraft] = useState<RecruiterEmail | null>(null);
  const [loading, setLoading] = useState(false);

  const days = daysAgo(statusAt);
  if (status !== "sent" || days === null || days < 7) return null;

  async function write() {
    setLoading(true);
    try {
      setDraft(await api.followUp(id));
    } catch (err) {
      toast(err instanceof Error ? err.message : "Couldn't write the follow up", "error");
    } finally {
      setLoading(false);
    }
  }

  async function copy() {
    if (!draft) return;
    const body = [draft.greeting, ...draft.paragraphs, draft.closing].join("\n\n");
    try {
      await navigator.clipboard.writeText(`${draft.subject}\n\n${body}`);
      toast("Copied. Paste it into your mail app.", "success");
    } catch {
      toast("Couldn't copy it. Select the text instead.", "error");
    }
  }

  return (
    <div className="mb-4 rounded-[var(--radius-panel)] border border-line bg-raised p-4 sm:p-5">
      <Eyebrow>Follow up</Eyebrow>
      <p className="mt-1.5 text-[13px] leading-relaxed text-fg-muted">
        Sent {days === 1 ? "a day" : `${days} days`} ago with no reply yet. A short nudge is fair
        game now.
      </p>

      {draft ? (
        <>
          <p className="mt-3 text-[12px] text-fg-faint">Subject</p>
          <p className="text-[13.5px] font-medium">{draft.subject}</p>
          <div className="mt-3 space-y-2.5 text-[13.5px] leading-relaxed">
            <p>{draft.greeting}</p>
            {draft.paragraphs.map((para, i) => (
              <p key={i}>{para}</p>
            ))}
            <p>{draft.closing}</p>
          </div>
          <div className="mt-3 flex flex-wrap items-center gap-2">
            <Button size="sm" variant="primary" onClick={copy}>
              <Copy size={13} />
              Copy
            </Button>
            <Button size="sm" variant="ghost" onClick={write} loading={loading}>
              Write another
            </Button>
          </div>
        </>
      ) : (
        <Button size="sm" className="mt-3" onClick={write} loading={loading}>
          <Send size={13} />
          Write the follow up
        </Button>
      )}
    </div>
  );
}
