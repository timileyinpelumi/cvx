"use client";

import { useEffect, useState, useSyncExternalStore } from "react";

const PROVIDER_LABELS: Record<string, string> = {
  google: "Continue with Google",
  github: "Continue with GitHub",
};

// Both marks are drawn in currentColor at the same 16px box as the lucide
// icons elsewhere, so a provider button is the app's own button with a glyph
// in it rather than a piece of someone else's brand.
function GoogleMark() {
  return (
    <svg
      width="16"
      height="16"
      viewBox="0 0 48 48"
      fill="currentColor"
      aria-hidden="true"
      focusable="false"
    >
      <path d="M44.5 20H24v8.5h11.8C34.7 33.9 30.1 37 24 37c-7.2 0-13-5.8-13-13s5.8-13 13-13c3.1 0 5.9 1.1 8.1 2.9l6.4-6.4C34.6 4.1 29.6 2 24 2 11.8 2 2 11.8 2 24s9.8 22 22 22c11 0 21-8 21-22 0-1.3-.2-2.7-.5-4z" />
    </svg>
  );
}

function GitHubMark() {
  return (
    <svg
      width="16"
      height="16"
      viewBox="0 0 16 16"
      fill="currentColor"
      aria-hidden="true"
      focusable="false"
    >
      <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27s1.36.09 2 .27c1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8z" />
    </svg>
  );
}

// The query string is read as an external store rather than in an effect: it
// never changes for the life of this screen, and the server snapshot keeps
// the first paint identical on both sides of hydration.
const noopSubscribe = () => () => {};

function readForbidden() {
  return new URLSearchParams(window.location.search).get("error") === "forbidden";
}

export function SignIn() {
  const [providers, setProviders] = useState<string[] | null>(null);
  // The OAuth callback sends denied accounts to /?error=forbidden.
  const forbidden = useSyncExternalStore(
    noopSubscribe,
    readForbidden,
    () => false,
  );

  useEffect(() => {
    let cancelled = false;
    async function load() {
      try {
        const res = await fetch("/auth/providers");
        if (!res.ok) throw new Error(String(res.status));
        const data = (await res.json()) as { providers?: string[] };
        if (!cancelled) setProviders(data.providers ?? []);
      } catch {
        // An empty list reads the same as a failed one: there is no way in,
        // and the only thing the reader can do about it is try again.
        if (!cancelled) setProviders([]);
      }
    }
    void load();
    return () => {
      cancelled = true;
    };
  }, []);

  // null while the list is still in flight, so the fallback notice can't
  // flash before there is anything to say.
  const usable = (providers ?? []).filter(
    (provider) => provider in PROVIDER_LABELS,
  );

  return (
    <div className="signin">
      <h2 className="signin-heading">Sign in to cvx</h2>

      <div className="signin-providers">
        {usable.map((provider) => (
          <a
            key={provider}
            className="btn btn--secondary"
            href={`/auth/${provider}/start`}
          >
            {provider === "google" ? <GoogleMark /> : <GitHubMark />}
            {PROVIDER_LABELS[provider]}
          </a>
        ))}
      </div>

      <div className="notice-slot" role="status">
        {forbidden ? (
          <p className="notice notice--error">
            This account doesn&apos;t have access.
          </p>
        ) : null}
        {providers !== null && usable.length === 0 ? (
          <p className="notice">Can&apos;t reach the server. Reload to try again.</p>
        ) : null}
      </div>
    </div>
  );
}
