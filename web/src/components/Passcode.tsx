"use client";

import { useState } from "react";

type PasscodeProps = {
  onUnlocked: () => void;
};

export function Passcode({ onUnlocked }: PasscodeProps) {
  const [value, setValue] = useState("");
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);

  async function submit() {
    if (value === "" || busy) return;
    setBusy(true);
    setFailed(false);
    try {
      const res = await fetch("/api/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ passcode: value }),
      });
      if (res.status === 204) {
        onUnlocked();
        return;
      }
      setFailed(true);
    } catch {
      setFailed(true);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="passcode">
      <h2 className="passcode-heading">Enter your passcode</h2>
      <div className="passcode-row">
        <input
          type="password"
          className="passcode-field"
          value={value}
          disabled={busy}
          autoFocus
          onChange={(event) => {
            setValue(event.target.value);
            setFailed(false);
          }}
          onKeyDown={(event) => {
            if (event.key === "Enter") void submit();
          }}
        />
        <button
          type="button"
          className="btn btn--primary btn--block"
          disabled={busy || value === ""}
          aria-busy={busy || undefined}
          onClick={() => void submit()}
        >
          Unlock
        </button>
      </div>

      <div className="notice-slot" role="status">
        {failed ? (
          <p className="notice notice--error">
            That passcode is wrong. Try again.
          </p>
        ) : null}
      </div>
    </div>
  );
}
