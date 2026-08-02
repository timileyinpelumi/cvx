"use client";

import { useRef, useState } from "react";
import { FileUp } from "lucide-react";

export type ProfileSummary = {
  name: string;
  itemCount: number;
  skillCount: number;
};

type UploaderProps = {
  onUploaded: (profile: ProfileSummary) => void;
  // Same upload, second time around: the copy can't still say "start".
  replace?: boolean;
};

type Status = "idle" | "uploading" | "error";

export function Uploader({ onUploaded, replace = false }: UploaderProps) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [status, setStatus] = useState<Status>("idle");
  const [errorDetail, setErrorDetail] = useState<string | null>(null);
  const [dragging, setDragging] = useState(false);

  const uploading = status === "uploading";

  async function upload(file: File) {
    setStatus("uploading");
    setErrorDetail(null);
    const body = new FormData();
    body.append("file", file);
    try {
      const res = await fetch("/api/profile", { method: "POST", body });
      if (!res.ok) {
        let detail = "";
        try {
          const errBody = (await res.json()) as { error?: string };
          detail = errBody.error ?? "";
        } catch {
          // non-JSON error body; no detail to show
        }
        setErrorDetail(detail || null);
        setStatus("error");
        return;
      }
      const profile = (await res.json()) as ProfileSummary;
      setStatus("idle");
      onUploaded(profile);
    } catch {
      setErrorDetail("server unreachable");
      setStatus("error");
    }
  }

  function handleChange(event: React.ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    // Reset so picking the same file twice still fires a change event.
    event.target.value = "";
    if (file) void upload(file);
  }

  function handleDrop(event: React.DragEvent<HTMLDivElement>) {
    event.preventDefault();
    setDragging(false);
    if (uploading) return;
    const file = event.dataTransfer.files?.[0];
    if (file) void upload(file);
  }

  function handleDragOver(event: React.DragEvent<HTMLDivElement>) {
    event.preventDefault();
    if (!uploading) setDragging(true);
  }

  function handleCardClick(event: React.MouseEvent<HTMLDivElement>) {
    if (uploading) return;
    // Mobile only: the whole card is the tap target. On pointer devices the
    // card is a drop zone instead, so a stray click must not open the picker.
    if (!window.matchMedia("(max-width: 480px)").matches) return;
    if ((event.target as HTMLElement).closest("label, input")) return;
    inputRef.current?.click();
  }

  return (
    <div
      className={dragging ? "uploader uploader--dragging" : "uploader"}
      onClick={handleCardClick}
      onDrop={handleDrop}
      onDragOver={handleDragOver}
      onDragLeave={() => setDragging(false)}
    >
      <h2 className="uploader-heading">
        {replace ? "Replace your resume" : "Start with your current resume"}
      </h2>
      <p className="uploader-body">
        {replace
          ? "Upload a new PDF. cvx reads it in and replaces the profile it holds now."
          : "Upload it once. cvx reads everything into your profile, and every tailored resume is cut from there."}
      </p>

      <div className="uploader-action">
        <input
          ref={inputRef}
          id="resume-file"
          className="file-input"
          type="file"
          accept="application/pdf"
          disabled={uploading}
          onChange={handleChange}
        />
        <label
          className={
            uploading
              ? "btn btn--primary btn--block is-disabled"
              : "btn btn--primary btn--block"
          }
          htmlFor="resume-file"
          aria-busy={uploading || undefined}
        >
          <FileUp size={16} aria-hidden="true" />
          Upload resume
        </label>
      </div>

      <div className="notice-slot" role="status">
        {uploading ? (
          <p className="notice">
            Reading your resume. This takes about half a minute.
          </p>
        ) : null}
        {status === "error" ? (
          <>
            <p className="notice notice--error">
              We couldn&apos;t read that file. Check that it&apos;s a PDF and
              try again.
            </p>
            {errorDetail ? <p className="error-detail">{errorDetail}</p> : null}
          </>
        ) : null}
      </div>
    </div>
  );
}
