import type { MetadataRoute } from "next";

export default function manifest(): MetadataRoute.Manifest {
  return {
    name: "cvx",
    short_name: "cvx",
    description:
      "Paste a job ad and get a one page resume made for it, using only what's already in your profile.",
    start_url: "/compose",
    display: "standalone",
    background_color: "#f5f6f8",
    theme_color: "#2244D9",
    icons: [{ src: "/icon.svg", sizes: "any", type: "image/svg+xml", purpose: "any" }],
  };
}
