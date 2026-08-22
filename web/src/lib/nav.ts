import { Gauge, Layers, Tags, Target, Type, User, UserPen } from "lucide-react";

export const NAV = [
  { href: "/compose", label: "Compose", hint: "Paste a job ad, get a resume", icon: Type },
  { href: "/resumes", label: "Resumes", hint: "Resumes you've made", icon: Layers },
  { href: "/gaps", label: "Gaps", hint: "What jobs keep asking for", icon: Target },
  { href: "/skills", label: "Skills", hint: "What's on your profile", icon: Tags },
  { href: "/profile", label: "Profile", hint: "Your history, in your own words", icon: UserPen },
  { href: "/account", label: "Account", hint: "Your profile and settings", icon: User },
] as const;

/** Shown only to accounts on the server's admin allowlist. */
export const ADMIN_NAV = {
  href: "/admin",
  label: "Admin",
  hint: "Logs, usage and cost",
  icon: Gauge,
} as const;
