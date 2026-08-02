"use client";

import { useCallback, useEffect, useState } from "react";

import { Archive, type GenerationMeta } from "@/components/Archive";
import { GapTracker, type GapTrend } from "@/components/GapTracker";
import { Generator } from "@/components/Generator";
import { Logo } from "@/components/Logo";
import { Nav, type View } from "@/components/Nav";
import { ProfileView, type Me } from "@/components/ProfileView";
import { SignIn } from "@/components/SignIn";
import { Stitch } from "@/components/Stitch";
import { Uploader, type ProfileSummary } from "@/components/Uploader";
import type { GenerateResult } from "@/components/ResultTag";

import "./page.css";

export default function Home() {
  const [loaded, setLoaded] = useState(false);
  const [me, setMe] = useState<Me | null>(null);
  const [view, setView] = useState<View>("tailor");
  const [profile, setProfile] = useState<ProfileSummary | null>(null);
  const [generations, setGenerations] = useState<GenerationMeta[]>([]);
  const [result, setResult] = useState<GenerateResult | null>(null);
  const [gapTrends, setGapTrends] = useState<GapTrend[]>([]);
  const [gapTotal, setGapTotal] = useState(0);

  // Each loader reports whether it hit a 401 rather than signing the user out
  // itself, so the caller can act once after Promise.all resolves instead of
  // tearing the page down mid-flight.
  const loadProfile = useCallback(async () => {
    try {
      const res = await fetch("/api/profile");
      if (res.status === 401) return true;
      if (!res.ok) return false;
      setProfile((await res.json()) as ProfileSummary);
      return false;
    } catch {
      // A failed load is treated as "no profile yet": upload state shows.
      return false;
    }
  }, []);

  const loadGenerations = useCallback(async () => {
    try {
      const res = await fetch("/api/generations");
      if (res.status === 401) return true;
      if (!res.ok) return false;
      setGenerations((await res.json()) as GenerationMeta[]);
      return false;
    } catch {
      // The archive is supplementary; a failed load just leaves it empty.
      return false;
    }
  }, []);

  const loadGaps = useCallback(async () => {
    try {
      const res = await fetch("/api/gaps");
      if (res.status === 401) return true;
      if (!res.ok) return false;
      const data = (await res.json()) as { total: number; trends: GapTrend[] };
      setGapTotal(data.total);
      setGapTrends(data.trends);
      return false;
    } catch {
      // The gap tracker is supplementary; a failed load just leaves it empty.
      return false;
    }
  }, []);

  const signOut = useCallback(() => {
    setMe(null);
    setView("tailor");
    setProfile(null);
    setGenerations([]);
    setResult(null);
    setGapTrends([]);
    setGapTotal(0);
  }, []);

  useEffect(() => {
    async function boot() {
      let user: Me | null = null;
      try {
        const res = await fetch("/api/me");
        if (res.ok) user = (await res.json()) as Me;
      } catch {
        // An unreachable server reads as signed out: sign-in is the only
        // screen that can help, and it retries the moment it mounts.
      }
      if (user === null) {
        setLoaded(true);
        return;
      }
      const results = await Promise.all([
        loadProfile(),
        loadGenerations(),
        loadGaps(),
      ]);
      setMe(results.some(Boolean) ? null : user);
      setLoaded(true);
    }
    void boot();
  }, [loadProfile, loadGenerations, loadGaps]);

  function handleResult(next: GenerateResult) {
    setResult(next);
    void (async () => {
      const results = await Promise.all([loadGenerations(), loadGaps()]);
      if (results.some(Boolean)) signOut();
    })();
  }

  const signedIn = loaded && me !== null;
  // A fresh account has nothing to put behind tabs yet, so the upload sits on
  // the page on its own until there is a profile to navigate.
  const onboarding = signedIn && profile === null;
  const navigating = signedIn && profile !== null;

  return (
    <div className="shell">
      <header className="masthead">
        <div className="masthead-brand">
          <h1 className="masthead-mark">
            <Logo height={28} />
          </h1>
          <Stitch width={60} className="masthead-stitch" />
        </div>
        {navigating ? (
          <Nav view={view} onChange={setView} variant="rail" />
        ) : null}
      </header>

      {!navigating ? (
        <p className="tagline">
          Keep one profile. Get a resume cut to fit any role.
        </p>
      ) : null}

      <main
        className="stage"
        {...(navigating ? { tabIndex: 0, "aria-label": "Content" } : {})}
      >
        {loaded && me === null ? <SignIn /> : null}

        {onboarding ? <Uploader onUploaded={setProfile} /> : null}

        {navigating && view === "tailor" ? (
          <Generator result={result} onResult={handleResult} />
        ) : null}

        {navigating && view === "history" ? (
          generations.length > 0 ? (
            <Archive rows={generations} />
          ) : (
            <p className="view-empty">Nothing here yet. Tailor your first resume.</p>
          )
        ) : null}

        {navigating && view === "gaps" ? (
          gapTrends.length > 0 ? (
            <GapTracker trends={gapTrends} total={gapTotal} />
          ) : (
            <p className="view-empty">
              No recurring gaps yet. They show up after a few generations.
            </p>
          )
        ) : null}

        {navigating && view === "profile" && me !== null && profile !== null ? (
          <ProfileView
            profile={profile}
            me={me}
            onProfile={setProfile}
            onSignedOut={signOut}
          />
        ) : null}
      </main>

      {navigating ? <Nav view={view} onChange={setView} variant="bar" /> : null}
    </div>
  );
}
