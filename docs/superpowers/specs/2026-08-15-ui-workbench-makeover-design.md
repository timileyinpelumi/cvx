# cvx UI Workbench Makeover — Design

Full frontend visual + information-architecture rebuild of `web/`. No backend/API changes. Built on a feature branch for preview; merge is the user's call after trying it.

## Motivation

Current UI (v1.1–v1.9) is already intentionally designed — token system, a stitch motif, flat-row lists — but both the visual feel and the day-to-day usability need work: no routing (state-only view switching, refresh loses place, no back button, no deep links), and a flat 5-item nav that weights rare actions (Settings, Profile) the same as the primary action (Tailor).

## Direction: Workbench

Dense, fast, tool-like — closer to Linear/Raycast than a marketing site. Optimized for someone who runs this often and wants minimum friction between "paste a role" and "have the PDF." Not a redesign of what cvx does, a redesign of how quickly and clearly it lets you do it.

## 1. Visual language & tokens

- Keep the existing type trio — Bricolage Grotesque (display), Instrument Sans (body), IBM Plex Mono (data). Plex Mono already reads "tool"; lean on it harder for anything data-shaped (dates, ids, gap counts, filenames).
- Tighten body density slightly on list-heavy screens (History, Gaps) vs. the current 15px/1.5 baseline.
- Replace hardcoded light tokens with semantic tokens defined once and re-mapped per theme:
  `--surface`, `--surface-raised`, `--text`, `--text-muted`, `--border`, `--accent` (replaces `--paper/--card/--ink/--muted/--edge/--chalk`). Status colors (`--gap-weak`, `--gap-missing`) carry forward, re-mapped per theme.
- Dark mode: `data-theme="dark"|"light"` on `<html>`, defaults to `prefers-color-scheme`, manual toggle persisted to `localStorage`.
- Motion is functional only — ~120–150ms transitions on state changes (nav switch, panel open, generate→result), nothing decorative. Respects existing `prefers-reduced-motion` handling.
- Lists stay flat rows with hairline dividers, no card surfaces — carries forward the standing preference; density goal reinforces it.

## 2. IA & navigation

- Move from client-state view switching to real Next.js App Router routes: `/tailor` (home/default), `/history`, `/history/[id]`, `/gaps`, `/account`, plus the existing sign-in gate. Fixes refresh/back-button/deep-link gaps for free.
- Nav rebalanced to 3 primary items — **Tailor, History, Gaps** — plus a single **Account** entry (merges the current separate Settings and Profile nav items).
- Desktop: persistent left sidebar (logo, primary nav, spacer, theme toggle, account entry pinned at bottom) replaces the current top masthead + separate icon rail. Denser, more tool-like, standard Workbench-genre pattern.
- Mobile: bottom tab bar keeps 4 items (Tailor, History, Gaps, Account).

## 3. Screen designs

**Tailor** — Two-pane on desktop once a result exists: composer (left) + result (right), replacing the current stacked-with-scroll layout. Mobile stays stacked. Cover-letter/recruiter-email checkboxes become a compact side-by-side toggle pair instead of two full-width rows. Result panel: header row inline (filename + actions, not stacked), "What changed" and "Gaps" as collapsible sections since gap lists can run long.

**History** — Same flat-row list, retheme only. New: rows become clickable into `/history/[id]`, a detail view reusing the result body (currently a past generation can only be downloaded or deleted, never reviewed in full — this closes that gap). Bulk select/download/delete stays functionally as-is.

**Gaps** — Same flat list, retheme only (denser rows, mono counts).

**Account** (merged Settings + Profile) — Single `/account` route, two sections: Profile (resume data, re-upload) and Preferences (PDF theme/style knobs, sign out). Matches the nav-weight rebalance — both are low-frequency, both about "your data/config."

## 4. Component & tooling approach

- Migrate to Tailwind v4 (CSS-first `@theme`, plays fine with the existing CSS-custom-property token approach — tokens become the Tailwind theme, not a competing system). Replaces the hand-rolled utility classes in `page.css` (1287 lines, mostly goes away). Same components, same props/behavior — styling layer only, not a logic rewrite.
- Stitch motif: currently two placements (masthead, result panel). The sidebar leaves no natural masthead slot, and Workbench density argues against decoration for its own sake — **dropping to one placement**, on the result panel at generation-complete, the one moment worth a flourish in an otherwise utilitarian UI. Flagging this explicitly since it changes a constraint you'd previously set, not silently dropping it.
- No new state-management or data-fetching library — same `fetch`-based patterns, just re-homed into route components.

## 5. Testing & verification

- No backend/API changes; same endpoints and data shapes throughout.
- `bun run lint` and `bun run build` (the App Router migration is exactly the kind of thing that surfaces as build/type errors).
- Manual pass, desktop + mobile viewport: sign in → onboarding upload → tailor → view result → history → open a past generation's detail → gaps → account → theme toggle → sign out.
- No automated UI tests exist today and this makeover doesn't introduce a testing framework — scope is visual/IA, not the guardrail/generation logic that already has Go test coverage. Flagging as a call, not a silent gap.

## Branch & rollout

Feature branch off `main`; user previews locally and decides merge vs. discard. Nothing here touches `server/`.
