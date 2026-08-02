"use client";

import { useRef, useState } from "react";
import { FileUp } from "lucide-react";

import { ProfileUpdate } from "./ProfileUpdate";
import type { ProfileSummary } from "./Uploader";

export type Me = {
  id: number;
  email: string;
  name: string;
  provider: string;
};

type ProfileViewProps = {
  profile: ProfileSummary;
  me: Me;
  onProfile: (profile: ProfileSummary) => void;
  onSignedOut: () => void;
};

export function ProfileView({
  profile,
  me,
  onProfile,
  onSignedOut,
}: ProfileViewProps) {
  const fileRef = useRef<HTMLInputElement>(null);
  const [replaceStatus, setReplaceStatus] = useState<
    "idle" | "uploading" | "error"
  >("idle");
  const [replaceDetail, setReplaceDetail] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const replacing = replaceStatus === "uploading";

  async function replaceUpload(file: File) {
    setReplaceStatus("uploading");
    setReplaceDetail(null);
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
        setReplaceDetail(detail || null);
        setReplaceStatus("error");
        return;
      }
      const next = (await res.json()) as ProfileSummary;
      setReplaceStatus("idle");
      onProfile(next);
    } catch {
      setReplaceDetail("server unreachable");
      setReplaceStatus("error");
    }
  }

  function handleFile(event: React.ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    // Reset so picking the same file twice still fires a change event.
    event.target.value = "";
    if (file) void replaceUpload(file);
  }

  async function signOut() {
    setBusy(true);
    try {
      await fetch("/api/logout", { method: "POST" });
    } catch {
      // The cookie may outlive an unreachable server, but this tab is done
      // with the session either way.
    }
    onSignedOut();
  }

  return (
    <section className="profile-view">
      <h2 className="view-heading">Your profile</h2>

      <p className="profile-line">
        <span className="profile-name">{profile.name}</span>
        <span className="profile-sep">·</span>
        <span className="profile-count">{profile.itemCount}</span> items
        <span className="profile-sep">·</span>
        <span className="profile-count">{profile.skillCount}</span> skills
      </p>

      <ProfileUpdate onUpdated={onProfile} />

      <div className="profile-replace">
        <input
          ref={fileRef}
          id="replace-file"
          className="file-input"
          type="file"
          accept="application/pdf"
          disabled={replacing}
          onChange={handleFile}
        />
        <label
          className={
            replacing
              ? "btn btn--secondary is-disabled"
              : "btn btn--secondary"
          }
          htmlFor="replace-file"
          aria-busy={replacing || undefined}
        >
          <FileUp size={16} aria-hidden="true" />
          Replace resume
        </label>

        <div className="notice-slot" role="status">
          {replacing ? (
            <p className="notice">
              Reading your resume. This takes about half a minute.
            </p>
          ) : null}
          {replaceStatus === "error" ? (
            <>
              <p className="notice notice--error">
                We couldn&apos;t read that file. Check that it&apos;s a PDF and
                try again.
              </p>
              {replaceDetail ? (
                <p className="error-detail">{replaceDetail}</p>
              ) : null}
            </>
          ) : null}
        </div>
      </div>

      <div className="profile-account">
        <h3 className="account-heading">Account</h3>
        <p className="account-line">
          {me.email}
          <span className="profile-sep">·</span>
          {me.provider}
        </p>
        <button
          type="button"
          className="btn btn--secondary btn--block"
          disabled={busy}
          onClick={() => void signOut()}
        >
          Sign out
        </button>
      </div>
    </section>
  );
}
