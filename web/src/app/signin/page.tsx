"use client";

import { useEffect, useState } from "react";
import { api, signInURL } from "@/lib/api";
import { Button, Spinner } from "@/components/ui";
import { Wordmark } from "@/components/Wordmark";

const PROVIDER_LABEL: Record<string, string> = {
  google: "Continue with Google",
  github: "Continue with GitHub",
};

export default function SignInPage() {
  const [providers, setProviders] = useState<string[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api
      .providers()
      .then(setProviders)
      .catch((err) => setError(err instanceof Error ? err.message : "Couldn't load your sign in options"));
  }, []);

  return (
    <div className="mx-auto flex min-h-dvh max-w-[34rem] flex-col justify-center px-6 py-16">
      <Wordmark />

      <h1 className="mt-8 font-display text-[30px] font-extrabold leading-[1.1] tracking-[-0.035em] sm:text-[36px]">
        A resume made for
        <br />
        the job you&rsquo;re
        <br />
        applying to.
      </h1>

      <p className="mt-5 max-w-[46ch] text-[14.5px] leading-relaxed text-fg-muted">
        Paste the job ad and cvx writes you a one page resume for it, using only
        what&rsquo;s already in your profile. It reworks how your experience reads so it
        fits the job. It won&rsquo;t make anything up.
      </p>

      <div className="mt-9 space-y-2.5">
        {providers === null && !error && (
          <div className="flex items-center gap-2 text-[13px] text-fg-faint">
            <Spinner />
            One moment
          </div>
        )}

        {error && (
          <div className="rounded-[var(--radius-ctl)] border border-line bg-raised px-3.5 py-3 text-[13px]">
            <p className="text-missing">{error}</p>
            <p className="mt-1 text-fg-muted">Make sure the cvx server is running, then reload.</p>
          </div>
        )}

        {providers?.map((p) => (
          <a key={p} href={signInURL(p)} className="block">
            <Button variant={p === providers[0] ? "primary" : "secondary"} className="w-full">
              {PROVIDER_LABEL[p] ?? `Continue with ${p}`}
            </Button>
          </a>
        ))}

        {providers?.length === 0 && (
          <div className="rounded-[var(--radius-ctl)] border border-line bg-raised px-3.5 py-3 text-[13px] text-fg-muted">
            There&rsquo;s no way to sign in set up yet. The server is in dev mode, so go
            straight to{" "}
            <span className="num text-fg">/compose</span>.
          </div>
        )}
      </div>
    </div>
  );
}
