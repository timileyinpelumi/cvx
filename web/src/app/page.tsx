"use client";

import { useCallback, useEffect, useState } from "react";

import { Archive, type GenerationMeta } from "@/components/Archive";
import { GapTracker, type GapTrend } from "@/components/GapTracker";
import { Generator } from "@/components/Generator";
import { Logo } from "@/components/Logo";
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

  const loadGenerations = useCallback(async () => {
    try {
      const res = await fetch("/api/generations");
      if (!res.ok) return;
      setGenerations((await res.json()) as GenerationMeta[]);
    } catch {
      // The archive is supplementary; a failed load just leaves it hidden.
    }
  }, []);

  const loadGaps = useCallback(async () => {
    try {
      const res = await fetch("/api/gaps");
      if (!res.ok) return;
      const data = (await res.json()) as { total: number; trends: GapTrend[] };
      setGapTotal(data.total);
      setGapTrends(data.trends);
    } catch {
      // The gap tracker is supplementary; a failed load just leaves it hidden.
    }
  }, []);

  useEffect(() => {
    async function load() {
      const [summary] = await Promise.all([
        fetch("/api/profile")
          .then((res) => (res.ok ? (res.json() as Promise<ProfileSummary>) : null))
          // A failed load is treated as "no profile yet": upload state shows.
          .catch(() => null),
        loadGenerations(),
        loadGaps(),
      ]);
      if (summary) setProfile(summary);
      setLoaded(true);
    }
    void load();
  }, [loadGenerations, loadGaps]);

  function handleResult(next: GenerateResult) {
    setResult(next);
    void loadGenerations();
    void loadGaps();
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
      </main>

      <Archive rows={earlier} />
      <GapTracker trends={gapTrends} total={gapTotal} />
    </div>
  );
}
