"use client";

import { useState } from "react";

type GapFillProps = {
  context: string;
  onFilled: () => void;
};

export function GapFill({ context, onFilled }: GapFillProps) {
  const [open, setOpen] = useState(false);
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  const [errorDetail, setErrorDetail] = useState<string | null>(null);
  const [done, setDone] = useState(false);

  async function submit() {
    if (note.trim() === "") return;
    setBusy(true);
    setFailed(false);
    setErrorDetail(null);
    try {
      const res = await fetch("/api/profile/extend", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ note, context }),
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
      setDone(true);
      onFilled();
    } catch {
      setErrorDetail("server unreachable");
      setFailed(true);
    } finally {
      setBusy(false);
    }
  }

  if (done) {
    return <span className="gap-fill-done">added to profile</span>;
  }

  return (
    <div className="gap-fill">
      {open ? (
        <div className="gap-fill-form">
          <input
            autoFocus
            type="text"
            className="gap-fill-field"
            placeholder="What is your real experience here"
            aria-label="What is your real experience here"
            value={note}
            disabled={busy}
            onChange={(event) => setNote(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter") void submit();
              if (event.key === "Escape") setOpen(false);
            }}
          />
          <button
            type="button"
            className="btn btn--primary"
            disabled={busy || note.trim() === ""}
            aria-busy={busy || undefined}
            onClick={() => void submit()}
          >
            Add
          </button>
        </div>
      ) : (
        <button
          type="button"
          className="gap-fill-toggle"
          onClick={() => setOpen(true)}
        >
          Fill in
        </button>
      )}
      {failed ? (
        <div className="gap-fill-error">
          <p className="notice notice--error">
            That didn&apos;t save. Try again in a moment.
          </p>
          {errorDetail ? <p className="error-detail">{errorDetail}</p> : null}
        </div>
      ) : null}
    </div>
  );
}
