import type { MetadataRoute } from "next";

export default function manifest(): MetadataRoute.Manifest {
  return {
    name: "cvx",
    short_name: "cvx",
    description: "Keep one profile. Get a resume cut to fit any role.",
    start_url: "/",
    display: "standalone",
    background_color: "#EEF0EF",
    theme_color: "#EEF0EF",
    icons: [
      { src: "/icon-192.png", sizes: "192x192", type: "image/png" },
      { src: "/icon-512.png", sizes: "512x512", type: "image/png" },
      {
        src: "/icon-512.png",
        sizes: "512x512",
        type: "image/png",
        purpose: "maskable",
      },
    ],
  };
}
