"use client";

import { useCallback, useEffect, useState } from "react";

import { Archive, type GenerationMeta } from "@/components/Archive";
import { GapTracker, type GapTrend } from "@/components/GapTracker";
import { Generator } from "@/components/Generator";
import { Logo } from "@/components/Logo";
import { Passcode } from "@/components/Passcode";
import { ProfileUpdate } from "@/components/ProfileUpdate";
import { Stitch } from "@/components/Stitch";
import { Uploader, type ProfileSummary } from "@/components/Uploader";
import type { GenerateResult } from "@/components/ResultTag";

import "./page.css";

export default function Home() {
  const [loaded, setLoaded] = useState(false);
  const [profile, setProfile] = useState<ProfileSummary | null>(null);
  const [generations, setGenerations] = useState<GenerationMeta[]>([]);
  const [result, setResult] = useState<GenerateResult | null>(null);
  const [gapTrends, setGapTrends] = useState<GapTrend[]>([]);
  const [gapTotal, setGapTotal] = useState(0);
  const [unauthorized, setUnauthorized] = useState(false);

  // Each loader reports whether it hit a 401 rather than setting unauthorized
  // itself, so load() below can set that state once after Promise.all
  // resolves instead of synchronously inside the effect.
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
      // The archive is supplementary; a failed load just leaves it hidden.
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
      // The gap tracker is supplementary; a failed load just leaves it hidden.
      return false;
    }
  }, []);

  // reloadKey bumps re-run the bootstrap effect below — used by
  // Passcode.onUnlocked so a successful login re-runs the exact same
  // bootstrap that 401'd on the previous attempt.
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    async function load() {
      const results = await Promise.all([loadProfile(), loadGenerations(), loadGaps()]);
      setUnauthorized(results.some(Boolean));
      setLoaded(true);
    }
    void load();
  }, [loadProfile, loadGenerations, loadGaps, reloadKey]);

  function handleResult(next: GenerateResult) {
    setResult(next);
    void (async () => {
      const results = await Promise.all([loadGenerations(), loadGaps()]);
      if (results.some(Boolean)) setUnauthorized(true);
    })();
  }

  const earlier = generations.filter((row) => row.id !== result?.id);

  return (
    <div className="shell">
      <header className="masthead">
        <h1 className="masthead-mark">
          <Logo height={28} />
        </h1>
        <Stitch width={60} className="masthead-stitch" />
        <p className="tagline">
          Keep one profile. Get a resume cut to fit any role.
        </p>
      </header>

      <main className="stage">
        {unauthorized ? (
          <Passcode onUnlocked={() => setReloadKey((k) => k + 1)} />
        ) : (
          <>
            {loaded && profile === null ? (
              <Uploader onUploaded={setProfile} />
            ) : null}

            {loaded && profile !== null ? (
              <>
                <p className="profile-line">
                  <span className="profile-name">{profile.name}</span>
                  <span className="profile-sep">·</span>
                  <span className="profile-count">{profile.itemCount}</span> items
                  <span className="profile-sep">·</span>
                  <span className="profile-count">{profile.skillCount}</span> skills
                </p>
                <ProfileUpdate onUpdated={setProfile} />
                <Generator result={result} onResult={handleResult} />
              </>
            ) : null}
          </>
        )}
      </main>

      {unauthorized ? null : (
        <>
          <Archive rows={earlier} />
          <GapTracker trends={gapTrends} total={gapTotal} />
        </>
      )}
    </div>
  );
}
