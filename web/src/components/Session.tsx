"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { ApiError, api } from "@/lib/api";
import type { Me, ProfileSummary } from "@/lib/types";

interface SessionValue {
  me: Me | null;
  profile: ProfileSummary | null;
  /** true once /api/me has resolved either way — gates the first paint. */
  ready: boolean;
  error: string | null;
  refreshProfile: () => Promise<void>;
  setProfile: (p: ProfileSummary) => void;
}

const SessionContext = createContext<SessionValue | null>(null);

export function useSession() {
  const ctx = useContext(SessionContext);
  if (!ctx) throw new Error("useSession must be used inside SessionProvider");
  return ctx;
}

export function SessionProvider({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const [me, setMe] = useState<Me | null>(null);
  const [profile, setProfile] = useState<ProfileSummary | null>(null);
  const [ready, setReady] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refreshProfile = useCallback(async () => {
    try {
      setProfile(await api.profile());
    } catch (err) {
      // 404 is the expected "hasn't uploaded a record yet" state, not a fault.
      if (err instanceof ApiError && err.status === 404) setProfile(null);
      else throw err;
    }
  }, []);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const who = await api.me();
        if (cancelled) return;
        setMe(who);
        await refreshProfile();
      } catch (err) {
        if (cancelled) return;
        if (err instanceof ApiError && err.status === 401) {
          router.replace("/signin");
          return;
        }
        setError(err instanceof Error ? err.message : "Something went wrong");
      } finally {
        if (!cancelled) setReady(true);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [router, refreshProfile]);

  const value = useMemo(
    () => ({ me, profile, ready, error, refreshProfile, setProfile }),
    [me, profile, ready, error, refreshProfile],
  );

  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}
