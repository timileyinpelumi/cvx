export type Theme = "light" | "dark";

const KEY = "cvx-theme";
const EVENT = "cvx-theme-change";

export function readTheme(): Theme {
  if (typeof document === "undefined") return "light";
  return document.documentElement.getAttribute("data-theme") === "dark" ? "dark" : "light";
}

export function applyTheme(theme: Theme) {
  document.documentElement.setAttribute("data-theme", theme);
  window.localStorage.setItem(KEY, theme);
  window.dispatchEvent(new Event(EVENT));
}

/* The <html data-theme> attribute is the source of truth, and it's set by the
   boot script below before first paint. Components read it through
   useSyncExternalStore rather than mirroring it into React state. */

export function subscribeTheme(onChange: () => void): () => void {
  window.addEventListener(EVENT, onChange);
  return () => window.removeEventListener(EVENT, onChange);
}

/** Runs before first paint so the chassis is never briefly the wrong colour. */
export const themeBootScript = `(function(){try{var t=localStorage.getItem("${KEY}");if(t!=="light"&&t!=="dark"){t=matchMedia("(prefers-color-scheme: dark)").matches?"dark":"light"}document.documentElement.setAttribute("data-theme",t)}catch(e){document.documentElement.setAttribute("data-theme","light")}})()`;
