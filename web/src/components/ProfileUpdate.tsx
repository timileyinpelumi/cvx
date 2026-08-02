"use client";

import { useEffect, useState } from "react";

import type { ProfileSummary } from "./Uploader";

type ProfileUpdateProps = {
  onUpdated: (profile: ProfileSummary) => void;
};

export function ProfileUpdate({ onUpdated }: ProfileUpdateProps) {
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  const [errorDetail, setErrorDetail] = useState<string | null>(null);
  // A counter rather than a boolean: resubmitting within the 4s window bumps
  // this to a new value even though the visible state (shown) stays true, so
  // the effect below still re-fires and restarts the hide timer.
  const [successTick, setSuccessTick] = useState(0);
  const success = successTick > 0;

  useEffect(() => {
    if (successTick === 0) return;
    const timer = setTimeout(() => setSuccessTick(0), 4000);
    return () => clearTimeout(timer);
  }, [successTick]);

  async function submit() {
    if (note.trim() === "") return;
    setBusy(true);
    setFailed(false);
    setErrorDetail(null);
    try {
      const res = await fetch("/api/profile/extend", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ note }),
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
      const profile = (await res.json()) as ProfileSummary;
      onUpdated(profile);
      setNote("");
      setSuccessTick((t) => t + 1);
    } catch {
      setErrorDetail("server unreachable");
      setFailed(true);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="profile-update">
      <div className="profile-update-row">
        <input
          type="text"
          className="profile-update-field"
          placeholder="Tell cvx what you shipped, learned, or earned"
          aria-label="Add to profile"
          value={note}
          disabled={busy}
          onChange={(event) => setNote(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter") void submit();
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

      <div className="notice-slot" role="status">
        {success ? (
          <p className="profile-update-success">Added to your profile.</p>
        ) : null}
        {failed ? (
          <>
            <p className="notice notice--error">
              That didn&apos;t save. Try again in a moment.
            </p>
            {errorDetail ? <p className="error-detail">{errorDetail}</p> : null}
          </>
        ) : null}
      </div>
    </div>
  );
}
