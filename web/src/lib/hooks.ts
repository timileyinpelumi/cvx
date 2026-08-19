"use client";

import { useCallback, useEffect, useState, useSyncExternalStore } from "react";
import { readTheme, subscribeTheme, type Theme } from "./theme";

interface Resource<T> {
  /** undefined until the first response settles. */
  data: T | undefined;
  error: string | null;
  reload: () => void;
}

/** Loads a read endpoint once per mount, with reload. Results are dropped if
 *  the component unmounts or reloads mid-flight, so a slow response can never
 *  overwrite a newer one. `fetcher` must be stable (useCallback). */
export function useResource<T>(fetcher: () => Promise<T>): Resource<T> {
  const [data, setData] = useState<T | undefined>(undefined);
  const [error, setError] = useState<string | null>(null);
  const [nonce, setNonce] = useState(0);

  useEffect(() => {
    let cancelled = false;
    fetcher().then(
      (next) => {
        if (cancelled) return;
        setData(next);
        setError(null);
      },
      (err: unknown) => {
        if (cancelled) return;
        setError(err instanceof Error ? err.message : "Something went wrong");
      },
    );
    return () => {
      cancelled = true;
    };
  }, [fetcher, nonce]);

  const reload = useCallback(() => setNonce((n) => n + 1), []);
  return { data, error, reload };
}

/** Reads the live theme off <html data-theme>. Returns null during SSR and the
 *  hydration pass so nothing renders a guess that then flips. */
export function useTheme(): Theme | null {
  return useSyncExternalStore(subscribeTheme, readTheme, () => null);
}

const REDUCED_MOTION = "(prefers-reduced-motion: reduce)";

function subscribeReducedMotion(onChange: () => void): () => void {
  const mq = window.matchMedia(REDUCED_MOTION);
  mq.addEventListener("change", onChange);
  return () => mq.removeEventListener("change", onChange);
}

export function usePrefersReducedMotion(): boolean {
  return useSyncExternalStore(
    subscribeReducedMotion,
    () => window.matchMedia(REDUCED_MOTION).matches,
    () => false,
  );
}
