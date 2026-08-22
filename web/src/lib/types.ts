/** Mirrors the Go server's JSON contract (server/internal/{httpapi,store,model,pdfgen}). */

export type Severity = "missing" | "weak";

export interface Gap {
  requirement: string;
  evidence: string;
  severity: Severity | string;
}

export interface KeywordHit {
  keyword: string;
  covered: boolean;
  where?: string;
}

export interface Coverage {
  hits: KeywordHit[];
  covered: number;
  total: number;
}

export type FitBand = "strong" | "fair" | "stretch";

export interface Fit {
  score: number;
  band: FitBand;
  reasons: string[];
}

export interface AvailableBullet {
  sourceBulletId: string;
  text: string;
}

export interface AvailableItem {
  sourceId: string;
  kind: string;
  title: string;
  organization: string;
  dates: string;
  onResume: boolean;
  bullets: AvailableBullet[];
}

export interface AvailableContent {
  items: AvailableItem[];
  skills: string[];
  certifications: string[];
  languages: string[];
  interests: string[];
}

export interface PreviewResult {
  pdf: string;
  fill: number;
  trimmed?: string[] | null;
  stretched: boolean;
  pageAdvice?: string[];
}

/* ---------------------------------- admin --------------------------------- */

export interface AdminCount {
  label: string;
  total: number;
  failed: number;
  ms?: number;
  tokens?: number;
  cost?: number;
}

export interface AdminEvent {
  id: number;
  at: string;
  userId?: number;
  kind: string;
  target?: string;
  ms?: number;
  ok: boolean;
  detail?: string;
  meta?: Record<string, unknown>;
}

export interface AdminLatency {
  label: string;
  count: number;
  p50: number;
  p95: number;
  max: number;
}

export interface AdminQuality {
  generations: number;
  avgFit: number;
  avgFill: number;
  trimmedShare: number;
  warnedShare: number;
  shortPageRate: number;
}

export interface AdminOverview {
  totals: {
    users: number;
    activeUsers: number;
    profiles: number;
    generations: number;
    events: number;
    dbBytes: number;
  };
  funnel: {
    signups: number;
    uploads: number;
    generations: number;
    downloads: number;
    sent: number;
  };
  activity: AdminCount[];
  daily: AdminCount[];
  usage: AdminCount[];
  failures: AdminCount[];
  health: {
    uptimeSeconds: number;
    goroutines: number;
    heapBytes: number;
    llm: string;
    mailEnabled: boolean;
    oauthEnabled: boolean;
    production: boolean;
  };
  windowDays: number;
  latency: AdminLatency[];
  quality: AdminQuality;
  slowest: AdminEvent[];
  providers: AdminCount[];
  errorRate: AdminCount[];
}

export interface AdminUser {
  id: number;
  email: string;
  name: string;
  provider: string;
  createdAt: string;
  lastSeen?: string;
  deleted: boolean;
  generations: number;
  tokens: number;
  cost: number;
}

export interface RecruiterEmail {
  subject: string;
  greeting: string;
  paragraphs: string[];
  closing: string;
}

export interface ProvenanceEntry {
  original: string;
  itemTitle: string;
  organization: string;
}

export type Provenance = Record<string, ProvenanceEntry>;

export interface Ungrounded {
  artifact: string;
  claim: string;
  profileSays: string;
}

export interface GenerationMeta {
  id: string;
  targetRole: string;
  roleSummary: string;
  filename: string;
  createdAt: string;
  gaps: Gap[];
  whatChanged: string[];
  hasCoverLetter: boolean;
  pinned: boolean;
  fit?: Fit;
  coverage?: Coverage;
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

export interface Certification {
  name: string;
  issuer: string;
  year: string;
}

/** The facts the profile editor owns. Work history is not among them: it
 *  comes from the uploaded resume and from notes. */
export interface ProfileEdits {
  name: string;
  email: string;
  phone: string;
  location: string;
  github: string;
  linkedin: string;
  portfolio: string;
  certifications?: Certification[];
  languages?: string[];
  interests?: string[];
}

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
  kind: string;
  title: string;
  items: TItem[];
}

export interface Tailored {
  targetRole: string;
  roleSummary: string;
  certifications?: string[];
  languages?: string[];
  interests?: string[];
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

/** A hole in the profile worth asking about, derived from its shape rather
 *  than generated, so it can never ask about a job you do not have. */
export interface IntakeQuestion {
  id: string;
  ask: string;
  why: string;
  itemId?: string;
  placeholder?: string;
}

export interface Me {
  id: number;
  email: string;
  name: string;
  provider: string;
  /** On the server's allowlist. The panel 403s regardless; this only
   *  decides whether the link is worth showing. */
  admin?: boolean;
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
  fit?: Fit;
  coverage?: Coverage;
  proseWarnings?: Ungrounded[];
  pageFill: number;
  pageAdvice?: string[];
}

export type ResumeTheme = "classic" | "modern" | "compact";
export type ResumeDensity = "normal" | "tight";

export interface ResumeStyle {
  theme: ResumeTheme;
  accent: string;
  density: ResumeDensity;
  skillsFirst: boolean;
  hidePhone: boolean;
  hideLocation: boolean;
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
