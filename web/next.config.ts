import type { NextConfig } from "next";

const API_ORIGIN = process.env.CVX_API_ORIGIN ?? "http://localhost:8080";

const nextConfig: NextConfig = {
  // Standalone bundles the server and only the modules it uses, so the whole
  // app ships as one small container next to the Go binary.
  output: "standalone",
  async redirects() {
    // The list lived at /history before the rename.
    return [{ source: "/history/:path*", destination: "/resumes/:path*", permanent: false }];
  },
  async rewrites() {
    return [
      // Health goes through the proxy on purpose: a check that passes proves
      // both halves of the container are up, not just the web one.
      { source: "/healthz", destination: `${API_ORIGIN}/healthz` },
      { source: "/api/:path*", destination: `${API_ORIGIN}/api/:path*` },
      { source: "/auth/:path*", destination: `${API_ORIGIN}/auth/:path*` },
      // The URLs a person actually sees (address bar, downloads) read as
      // documents, not API plumbing.
      { source: "/resume/:id.pdf", destination: `${API_ORIGIN}/api/generations/:id/pdf` },
      { source: "/cover/:id.pdf", destination: `${API_ORIGIN}/api/generations/:id/cover` },
      { source: "/preview.pdf", destination: `${API_ORIGIN}/api/settings/preview` },
    ];
  },
};

export default nextConfig;
