/** Mirrors the Go server's JSON contract (server/internal/{httpapi,store,model,pdfgen}). */

export type Severity = "missing" | "weak";

export interface Gap {
  requirement: string;
  evidence: string;
  severity: Severity | string;
}

export interface GenerationMeta {
  id: string;
  targetRole: string;
  filename: string;
  createdAt: string;
  gaps: Gap[];
  whatChanged: string[];
  hasCoverLetter: boolean;
  pinned: boolean;
  status: ApplicationStatus;
  statusAt: string;
}

export type ApplicationStatus = "" | "sent" | "interviewing" | "rejected" | "offer";

export const STATUSES: { value: ApplicationStatus; label: string }[] = [
  { value: "", label: "Not sent" },
  { value: "sent", label: "Sent" },
  { value: "interviewing", label: "Interviewing" },
  { value: "rejected", label: "Rejected" },
  { value: "offer", label: "Offer" },
];

/** Per-status color classes: chip = the active selector pill, badge = the
 *  small label on list rows. */
export const STATUS_TONES: Record<Exclude<ApplicationStatus, "">, { chip: string; badge: string }> = {
  sent: { chip: "border-ink bg-ink-soft text-ink", badge: "bg-ink-soft text-ink" },
  interviewing: { chip: "border-weak bg-weak-soft text-weak", badge: "bg-weak-soft text-weak" },
  rejected: { chip: "border-missing bg-missing-soft text-missing", badge: "bg-missing-soft text-missing" },
  offer: { chip: "border-good bg-good-soft text-good", badge: "bg-good-soft text-good" },
};

export interface TBullet {
  sourceBulletId: string;
  text: string;
}

export interface TItem {
  sourceId: string;
  title: string;
  organization: string;
  dates: string;
  bullets: TBullet[];
}

export interface TSection {
  title: string;
  items: TItem[];
}

export interface Tailored {
  targetRole: string;
  headline: string;
  summary: string;
  selectedSkills: string[];
  sections: TSection[];
  gaps: Gap[];
  whatChanged: string[];
}

export interface GapTrend {
  requirement: string;
  count: number;
  missing: number;
  weak: number;
  lastEvidence: string;
}

export interface GapsSummary {
  total: number;
  trends: GapTrend[];
}

export interface ProfileSummary {
  name: string;
  itemCount: number;
  skillCount: number;
}

export interface Me {
  id: number;
  email: string;
  name: string;
  provider: string;
}

export interface GenerateRequest {
  roleInput: string;
  coverLetter: boolean;
  recruiterEmail: boolean;
}

export interface GenerateResult {
  id: string;
  filename: string;
  gaps: Gap[];
  whatChanged: string[];
  emailed: boolean;
  coverFilename: string;
  coverLetter: boolean;
  recruiterEmail: boolean;
}

export type ResumeTheme = "classic" | "modern" | "compact";
export type ResumeDensity = "normal" | "tight";

export interface ResumeStyle {
  theme: ResumeTheme;
  accent: string;
  density: ResumeDensity;
  skillsFirst: boolean;
}

export type GenerationTone = "plain" | "confident";
export type GenerationSummary = "standard" | "short" | "none";
export type GenerationBullets = "full" | "lean";

export interface GenerationParams {
  tone: GenerationTone;
  summary: GenerationSummary;
  bullets: GenerationBullets;
}

export interface Settings {
  resumeStyle: ResumeStyle;
  emailCopy: boolean;
  recruiterAuto: boolean;
  generation: GenerationParams;
}

/** The accents the server will accept — anything else fails Style.Validate. */
export const ACCENTS: { value: string; name: string }[] = [
  { value: "#1C2422", name: "Graphite" },
  { value: "#2244D9", name: "Ink" },
  { value: "#0F766E", name: "Pine" },
  { value: "#7C2D92", name: "Plum" },
  { value: "#B3341E", name: "Rust" },
  { value: "#B07818", name: "Brass" },
];

export const THEMES: { value: ResumeTheme; name: string; note: string }[] = [
  { value: "modern", name: "Modern", note: "Sans headings, open spacing" },
  { value: "classic", name: "Classic", note: "Serif headings, traditional" },
  { value: "compact", name: "Compact", note: "Tightest fit, most content" },
];
