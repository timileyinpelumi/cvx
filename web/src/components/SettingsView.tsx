"use client";

import { useEffect, useState } from "react";

type ResumeStyle = {
  theme: "classic" | "modern" | "compact";
  accent: string;
  density: "normal" | "tight";
  skillsFirst: boolean;
};

type SettingsViewProps = {
  onSignedOut: () => void;
};

const THEMES: { value: ResumeStyle["theme"]; label: string; hint: string }[] = [
  { value: "classic", label: "Classic", hint: "Serif headings, quiet rules, traditional." },
  { value: "modern", label: "Modern", hint: "Clean sans with an accent. The default." },
  { value: "compact", label: "Compact", hint: "Smaller and tighter, for full profiles." },
];

const ACCENTS: { value: string; name: string }[] = [
  { value: "#1C2422", name: "Ink" },
  { value: "#2244D9", name: "Chalk" },
  { value: "#0F766E", name: "Teal" },
  { value: "#7C2D92", name: "Plum" },
  { value: "#B3341E", name: "Rust" },
  { value: "#B07818", name: "Bronze" },
];

export function SettingsView({ onSignedOut }: SettingsViewProps) {
  const [style, setStyle] = useState<ResumeStyle | null>(null);
  const [savedTick, setSavedTick] = useState(0);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    let cancelled = false;
    async function load() {
      try {
        const res = await fetch("/api/settings");
        if (res.status === 401) {
          onSignedOut();
          return;
        }
        if (!res.ok) return;
        const data = (await res.json()) as { resumeStyle: ResumeStyle };
        if (!cancelled) setStyle(data.resumeStyle);
      } catch {
        // Leaving style null keeps the view in its loading state; switching
        // views and back retries.
      }
    }
    void load();
    return () => {
      cancelled = true;
    };
  }, [onSignedOut]);

  useEffect(() => {
    if (savedTick === 0) return;
    const timer = setTimeout(() => setSavedTick(0), 3000);
    return () => clearTimeout(timer);
  }, [savedTick]);

  function apply(next: ResumeStyle) {
    setStyle(next);
    setFailed(false);
    void (async () => {
      try {
        const res = await fetch("/api/settings", {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ resumeStyle: next }),
        });
        if (res.status === 401) {
          onSignedOut();
          return;
        }
        if (!res.ok) {
          setFailed(true);
          return;
        }
        setSavedTick((t) => t + 1);
      } catch {
        setFailed(true);
      }
    })();
  }

  if (style === null) {
    return <p className="view-empty">Loading settings.</p>;
  }

  return (
    <section className="settings-view">
      <h2 className="view-heading">Settings</h2>

      <div className="profile-section">
        <h3 className="profile-section-title">Resume style</h3>

        <div className="settings-group" role="radiogroup" aria-label="Theme">
          {THEMES.map((t) => (
            <label key={t.value} className="settings-radio-row">
              <input
                type="radio"
                name="theme"
                className="option-box"
                checked={style.theme === t.value}
                onChange={() => apply({ ...style, theme: t.value })}
              />
              <span className="settings-radio-text">
                <span className="settings-radio-label">{t.label}</span>
                <span className="settings-radio-hint">{t.hint}</span>
              </span>
            </label>
          ))}
        </div>

        <div className="settings-row">
          <span className="settings-row-label">Accent</span>
          <div className="accent-dots">
            {ACCENTS.map((a) => (
              <button
                key={a.value}
                type="button"
                className={
                  style.accent === a.value ? "accent-dot is-active" : "accent-dot"
                }
                style={{ background: a.value }}
                aria-label={a.name}
                title={a.name}
                aria-pressed={style.accent === a.value}
                onClick={() => apply({ ...style, accent: a.value })}
              />
            ))}
          </div>
        </div>

        <div className="settings-row" role="radiogroup" aria-label="Density">
          <span className="settings-row-label">Density</span>
          <label className="settings-inline-option">
            <input
              type="radio"
              name="density"
              className="option-box"
              checked={style.density === "normal"}
              onChange={() => apply({ ...style, density: "normal" })}
            />
            Normal
          </label>
          <label className="settings-inline-option">
            <input
              type="radio"
              name="density"
              className="option-box"
              checked={style.density === "tight"}
              onChange={() => apply({ ...style, density: "tight" })}
            />
            Tight
          </label>
        </div>

        <div className="settings-row" role="radiogroup" aria-label="Order">
          <span className="settings-row-label">Order</span>
          <label className="settings-inline-option">
            <input
              type="radio"
              name="order"
              className="option-box"
              checked={!style.skillsFirst}
              onChange={() => apply({ ...style, skillsFirst: false })}
            />
            Experience first
          </label>
          <label className="settings-inline-option">
            <input
              type="radio"
              name="order"
              className="option-box"
              checked={style.skillsFirst}
              onChange={() => apply({ ...style, skillsFirst: true })}
            />
            Skills first
          </label>
        </div>

        <a
          className="settings-preview-link"
          href="/api/settings/preview"
          target="_blank"
          rel="noreferrer"
        >
          Preview PDF
        </a>

        <div className="notice-slot" role="status">
          {savedTick > 0 ? <p className="profile-update-success">Saved.</p> : null}
          {failed ? (
            <p className="notice notice--error">
              That didn&apos;t save. Try again in a moment.
            </p>
          ) : null}
        </div>
      </div>
    </section>
  );
}
