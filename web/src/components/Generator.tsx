"use client";

import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { Scissors } from "lucide-react";

import { ResultTag, type GenerateResult } from "./ResultTag";

const MAX_ROWS = 8;

type GeneratorProps = {
  result: GenerateResult | null;
  onResult: (result: GenerateResult) => void;
};

export function Generator({ result, onResult }: GeneratorProps) {
  const fieldRef = useRef<HTMLTextAreaElement>(null);
  const [roleInput, setRoleInput] = useState("");
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);

  const autoGrow = useCallback(() => {
    const field = fieldRef.current;
    if (!field) return;
    const styles = getComputedStyle(field);
    const border =
      parseFloat(styles.borderTopWidth) + parseFloat(styles.borderBottomWidth);
    const max =
      parseFloat(styles.lineHeight) * MAX_ROWS +
      parseFloat(styles.paddingTop) +
      parseFloat(styles.paddingBottom) +
      border;
    field.style.height = "auto";
    field.style.height = `${Math.min(field.scrollHeight + border, max)}px`;
  }, []);

  useLayoutEffect(autoGrow, [autoGrow, roleInput]);

  useEffect(() => {
    window.addEventListener("resize", autoGrow);
    return () => window.removeEventListener("resize", autoGrow);
  }, [autoGrow]);

  async function generate() {
    setBusy(true);
    setFailed(false);
    try {
      const res = await fetch("/api/generate", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ roleInput }),
      });
      if (!res.ok) throw new Error(String(res.status));
      onResult((await res.json()) as GenerateResult);
    } catch {
      setFailed(true);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="tailor">
      <textarea
        ref={fieldRef}
        className="composer-field"
        aria-label="Paste a role title, or the full job description for a closer fit"
        placeholder="Paste a role title, or the full job description for a closer fit"
        value={roleInput}
        disabled={busy}
        onChange={(event) => setRoleInput(event.target.value)}
      />

      <div className="composer-actions">
        <button
          type="button"
          className="btn btn--primary btn--block"
          disabled={busy || roleInput.trim() === ""}
          aria-busy={busy || undefined}
          onClick={() => void generate()}
        >
          {busy ? null : <Scissors size={16} aria-hidden="true" />}
          {busy ? "Tailoring" : "Tailor resume"}
        </button>
      </div>

      <div className="notice-slot" role="status">
        {failed ? (
          <p className="notice notice--error">
            The tailoring failed. Try again in a moment.
          </p>
        ) : null}
      </div>

      {busy ? <ResultTag pending /> : null}
      {!busy && result ? <ResultTag result={result} /> : null}
    </div>
  );
}
