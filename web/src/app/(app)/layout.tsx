"use client";

import { SessionProvider, useSession } from "@/components/Session";
import { ToastProvider } from "@/components/Toast";
import { Shell } from "@/components/Shell";
import { Onboarding } from "@/components/Onboarding";
import { ErrorState, Spinner } from "@/components/ui";

export default function AppLayout({ children }: { children: React.ReactNode }) {
  return (
    <SessionProvider>
      <ToastProvider>
        <Gate>{children}</Gate>
      </ToastProvider>
    </SessionProvider>
  );
}

function Gate({ children }: { children: React.ReactNode }) {
  const { ready, error, me, profile } = useSession();

  if (!ready) {
    return (
      <div className="flex min-h-dvh items-center justify-center text-fg-faint">
        <Spinner />
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex min-h-dvh items-center justify-center">
        <ErrorState message={error} onRetry={() => window.location.reload()} />
      </div>
    );
  }

  // A 401 has already redirected to /signin; this only covers the brief frame
  // before the router lands.
  if (!me) return null;

  if (!profile) return <Onboarding />;

  return <Shell>{children}</Shell>;
}
