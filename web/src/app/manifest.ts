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
    icons: [
      // The SVG first for anything that can scale it, then the raster sizes
      // Android actually installs with. The maskable one is inset, because
      // Android crops roughly a tenth off every edge.
      { src: "/icon.svg", sizes: "any", type: "image/svg+xml", purpose: "any" },
      { src: "/icon-192.png", sizes: "192x192", type: "image/png", purpose: "any" },
      { src: "/icon-512.png", sizes: "512x512", type: "image/png", purpose: "any" },
      { src: "/icon-maskable-512.png", sizes: "512x512", type: "image/png", purpose: "maskable" },
    ],
  };
}
