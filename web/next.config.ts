import type { NextConfig } from "next";

const API_ORIGIN = process.env.CVX_API_ORIGIN ?? "http://localhost:8080";

const nextConfig: NextConfig = {
  async redirects() {
    // The list lived at /history before the rename.
    return [{ source: "/history/:path*", destination: "/resumes/:path*", permanent: false }];
  },
  async rewrites() {
    return [
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
