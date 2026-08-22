import type {
  AvailableContent,
  GapsSummary,
  GenerateRequest,
  GenerateResult,
  GenerationMeta,
  Me,
  PreviewResult,
  ProfileEdits,
  ProfileSummary,
  Provenance,
  RecruiterEmail,
  Settings,
  Tailored,
} from "./types";

/** Thrown for any non-2xx response. `status` lets callers branch on 401
 *  (signed out), 404 (no profile yet) and 409 (profile required) without
 *  string-matching the message. */
export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  let res: Response;
  try {
    res = await fetch(path, { credentials: "include", ...init });
  } catch {
    throw new ApiError(0, "Can't reach the cvx server. Check that it's running on :8080.");
  }

  if (res.status === 204) return undefined as T;

  if (!res.ok) {
    let message = `Request failed (${res.status})`;
    try {
      const body = (await res.json()) as { error?: string };
      if (body?.error) message = body.error;
    } catch {
      /* non-JSON error body — keep the status-derived message */
    }
    throw new ApiError(res.status, message);
  }

  return (await res.json()) as T;
}

export const api = {
  me: () => request<Me>("/api/me"),

  logout: () => request<void>("/api/logout", { method: "POST" }),

  deleteAccount: () => request<void>("/api/account", { method: "DELETE" }),

  providers: () =>
    request<{ providers: string[] }>("/auth/providers").then((r) => r.providers),

  profile: () => request<ProfileSummary>("/api/profile"),

  profileEdits: () => request<ProfileEdits>("/api/profile/edits"),

  saveProfileEdits: (p: ProfileEdits) =>
    request<ProfileEdits>("/api/profile", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(p),
    }),

  uploadProfile: (file: File) => {
    const form = new FormData();
    form.append("file", file);
    return request<ProfileSummary>("/api/profile", { method: "POST", body: form });
  },

  extendProfile: (note: string, context: string) =>
    request<ProfileSummary>("/api/profile/extend", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ note, context }),
    }),

  generate: (req: GenerateRequest) =>
    request<GenerateResult>("/api/generate", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(req),
    }),

  generations: () => request<GenerationMeta[]>("/api/generations"),

  deleteGeneration: (id: string) =>
    request<void>(`/api/generations/${encodeURIComponent(id)}`, { method: "DELETE" }),

  setStatus: (id: string, status: string) =>
    request<void>(`/api/generations/${encodeURIComponent(id)}/status`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ status }),
    }),

  tailored: (id: string) =>
    request<Tailored>(`/api/generations/${encodeURIComponent(id)}/tailored`),

  saveTailored: (id: string, t: Tailored) =>
    request<Tailored>(`/api/generations/${encodeURIComponent(id)}`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(t),
    }),

  provenance: (id: string) => request<Provenance>(`/api/generations/${id}/provenance`),

  available: (id: string) => request<AvailableContent>(`/api/generations/${id}/available`),

  previewTailored: (id: string, t: Tailored) =>
    request<PreviewResult>(`/api/generations/${id}/preview`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(t),
    }),

  followUp: (id: string) =>
    request<RecruiterEmail>(`/api/generations/${id}/followup`, { method: "POST" }),

  rewriteBullet: (id: string, bulletId: string, current: string, instruction = "") =>
    request<{ text: string }>(`/api/generations/${id}/bullet`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ bulletId, current, instruction }),
    }).then((r) => r.text),

  profileHistory: () =>
    request<{ count: number; lastSavedAt: string }>("/api/profile/history"),

  restoreProfile: () =>
    request<ProfileSummary>("/api/profile/restore", { method: "POST" }),

  setPinned: (id: string, pinned: boolean) =>
    request<void>(`/api/generations/${encodeURIComponent(id)}/pin`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ pinned }),
    }),

  gaps: () => request<GapsSummary>("/api/gaps"),

  skills: () =>
    request<{ skills: string[] }>("/api/profile/skills").then((r) => r.skills),

  saveSkills: (skills: string[]) =>
    request<{ skills: string[] }>("/api/profile/skills", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ skills }),
    }).then((r) => r.skills),

  settings: () => request<Settings>("/api/settings"),

  saveSettings: (settings: Settings) =>
    request<Settings>("/api/settings", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(settings),
    }),
};

/** Download URLs are plain hrefs so the browser handles Content-Disposition
 *  itself — no blob juggling, and the session cookie rides along. */
export const pdfURL = (id: string) => `/resume/${encodeURIComponent(id)}.pdf`;
export const coverURL = (id: string) => `/cover/${encodeURIComponent(id)}.pdf`;
export const previewURL = () => "/preview.pdf";
export const signInURL = (provider: string) => `/auth/${encodeURIComponent(provider)}/start`;
