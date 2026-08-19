import type { Metadata, Viewport } from "next";
import { Archivo, Public_Sans, JetBrains_Mono } from "next/font/google";
import { themeBootScript } from "@/lib/theme";
import "./globals.css";

const archivo = Archivo({
  subsets: ["latin"],
  weight: ["600", "700", "800"],
  variable: "--font-archivo",
  display: "swap",
});

const publicSans = Public_Sans({
  subsets: ["latin"],
  weight: ["400", "500", "600"],
  variable: "--font-public-sans",
  display: "swap",
});

const jetbrainsMono = JetBrains_Mono({
  subsets: ["latin"],
  weight: ["400", "500", "600"],
  variable: "--font-jetbrains-mono",
  display: "swap",
});

const description =
  "Paste a job ad and get a one page resume made for it, using only what's already in your profile.";

export const metadata: Metadata = {
  metadataBase: new URL(process.env.NEXT_PUBLIC_BASE_URL ?? "http://localhost:3100"),
  title: {
    default: "cvx",
    template: "%s · cvx",
  },
  description,
  applicationName: "cvx",
  keywords: ["resume", "tailored resume", "job application", "cover letter", "one page resume"],
  openGraph: {
    title: "cvx",
    description,
    siteName: "cvx",
    type: "website",
    url: "/",
  },
  twitter: {
    card: "summary_large_image",
    title: "cvx",
    description,
  },
  robots: { index: true, follow: true },
};

export const viewport: Viewport = {
  themeColor: [
    { media: "(prefers-color-scheme: light)", color: "#f5f6f8" },
    { media: "(prefers-color-scheme: dark)", color: "#0e1116" },
  ],
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" suppressHydrationWarning>
      <head>
        <script dangerouslySetInnerHTML={{ __html: themeBootScript }} />
      </head>
      <body className={`${archivo.variable} ${publicSans.variable} ${jetbrainsMono.variable}`}>
        {children}
      </body>
    </html>
  );
}
