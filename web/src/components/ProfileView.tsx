"use client";

import { useState } from "react";

import { ProfileUpdate } from "./ProfileUpdate";
import { Uploader, type ProfileSummary } from "./Uploader";

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
  const [replacing, setReplacing] = useState(false);
  const [busy, setBusy] = useState(false);

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
        <button
          type="button"
          className="btn btn--secondary btn--block"
          aria-expanded={replacing}
          onClick={() => setReplacing((open) => !open)}
        >
          Replace resume
        </button>

        {replacing ? (
          <div className="profile-replace-panel">
            <Uploader
              replace
              onUploaded={(next) => {
                onProfile(next);
                setReplacing(false);
              }}
            />
          </div>
        ) : null}
      </div>

      <div className="profile-account">
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
