"use client";

import { useRef, useState } from "react";
import { Upload } from "lucide-react";
import { api } from "@/lib/api";
import { cx } from "@/lib/format";
import { Button } from "./ui";
import { Wordmark } from "./Wordmark";
import { useSession } from "./Session";
import { useToast } from "./Toast";

/** The one thing that must exist before anything else works: the record every
 *  tailored resume is drawn from. */
export function Onboarding() {
  const { setProfile } = useSession();
  const toast = useToast();
  const [busy, setBusy] = useState(false);
  const [dragging, setDragging] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  async function upload(file: File) {
    if (file.type !== "application/pdf" && !file.name.toLowerCase().endsWith(".pdf")) {
      toast("That file needs to be a PDF.", "error");
      return;
    }
    setBusy(true);
    try {
      const summary = await api.uploadProfile(file);
      setProfile(summary);
      toast(`All set. ${summary.itemCount} entries and ${summary.skillCount} skills.`, "success");
    } catch (err) {
      toast(err instanceof Error ? err.message : "That upload didn't work", "error");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="mx-auto flex min-h-dvh max-w-[38rem] flex-col justify-center px-6 py-16">
      <Wordmark />

      <h1 className="mt-7 font-display text-[26px] font-extrabold leading-[1.15] tracking-[-0.03em]">
        First, add your resume.
      </h1>
      <p className="mt-3 text-[14.5px] leading-relaxed text-fg-muted">
        Upload the resume you use now and cvx will pull out everything you&rsquo;ve done.
        That becomes your profile, and every resume it makes later comes from it, so
        you only have to do this once.
      </p>

      <div
        onDragOver={(e) => {
          e.preventDefault();
          setDragging(true);
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={(e) => {
          e.preventDefault();
          setDragging(false);
          const file = e.dataTransfer.files?.[0];
          if (file) upload(file);
        }}
        className={cx(
          "mt-8 flex flex-col items-center rounded-[var(--radius-panel)] border border-dashed px-6 py-12",
          "transition-colors duration-[130ms]",
          dragging ? "border-ink bg-ink-soft" : "border-line-strong bg-raised",
        )}
      >
        <Upload size={20} className="text-fg-faint" />
        <p className="mt-3 text-[13.5px] text-fg-muted">Drop a PDF here</p>
        <Button
          variant="primary"
          className="mt-4"
          loading={busy}
          onClick={() => inputRef.current?.click()}
        >
          {busy ? "Reading your resume" : "Choose a file"}
        </Button>
        <input
          ref={inputRef}
          type="file"
          accept="application/pdf,.pdf"
          className="sr-only"
          onChange={(e) => {
            const file = e.target.files?.[0];
            if (file) upload(file);
            e.target.value = "";
          }}
        />
      </div>

      <p className="num mt-4 text-[11.5px] text-fg-faint">PDF, up to 15 MB</p>
    </div>
  );
}
