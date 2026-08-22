"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { ADMIN_NAV, NAV } from "@/lib/nav";
import { cx } from "@/lib/format";
import { useSession } from "./Session";
import { Wordmark } from "./Wordmark";
import { ThemeToggle } from "./ThemeToggle";
import { CommandPalette } from "./CommandPalette";

export function Shell({ children }: { children: React.ReactNode }) {
  const [paletteOpen, setPaletteOpen] = useState(false);

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setPaletteOpen((o) => !o);
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  return (
    <div className="min-h-dvh">
      <Sidebar onOpenPalette={() => setPaletteOpen(true)} />
      <main className="min-h-dvh pb-[env(safe-area-inset-bottom)] md:ml-[232px]">
        <div className="pb-20 md:pb-0">{children}</div>
      </main>
      <MobileTabs />
      {paletteOpen && <CommandPalette onClose={() => setPaletteOpen(false)} />}
    </div>
  );
}

function useIsActive() {
  const pathname = usePathname();
  return (href: string) => pathname === href || pathname.startsWith(href + "/");
}

function Sidebar({ onOpenPalette }: { onOpenPalette: () => void }) {
  const isActive = useIsActive();
  const { me, profile } = useSession();

  return (
    <aside className="fixed inset-y-0 left-0 z-30 hidden w-[232px] flex-col border-r border-line bg-raised md:flex">
      <div className="flex h-14 items-center px-5">
        <Link href="/compose" className="rounded" aria-label="cvx, go to Compose">
          <Wordmark />
        </Link>
      </div>

      <button
        type="button"
        onClick={onOpenPalette}
        className={cx(
          "mx-3 mb-3 flex h-8 items-center gap-2 rounded-[var(--radius-ctl)] border border-line",
          "px-2.5 text-[13px] text-fg-faint transition-colors duration-[130ms]",
          "hover:border-line-strong hover:text-fg-muted",
        )}
      >
        <span className="flex-1 text-left">Search</span>
        <kbd className="num rounded border border-line px-1 py-px text-[10px] leading-[15px]">⌘K</kbd>
      </button>

      <nav className="flex-1 px-3">
        <ul className="space-y-0.5">
          {useNav()
            .filter((n) => n.href !== "/account")
            .map((item) => (
              <li key={item.href}>
                <NavLink href={item.href} label={item.label} icon={item.icon} active={isActive(item.href)} />
              </li>
            ))}
        </ul>
      </nav>

      <div className="border-t border-line px-3 py-3">
        {profile && (
          <p className="num mb-2.5 px-2.5 text-[11px] text-fg-faint">
            {profile.itemCount} entries · {profile.skillCount} skills
          </p>
        )}
        <div className="flex items-center gap-1">
          <div className="min-w-0 flex-1">
            <NavLink
              href="/account"
              label={me?.name || me?.email || "Account"}
              icon={NAV[NAV.length - 1].icon}
              active={isActive("/account")}
            />
          </div>
          <ThemeToggle />
        </div>
      </div>
    </aside>
  );
}

function NavLink({
  href,
  label,
  icon: Icon,
  active,
}: {
  href: string;
  label: string;
  icon: React.ComponentType<{ size?: number | string }>;
  active: boolean;
}) {
  return (
    <Link
      href={href}
      aria-current={active ? "page" : undefined}
      className={cx(
        "flex h-8 items-center gap-2.5 rounded-[var(--radius-ctl)] px-2.5",
        "text-[13.5px] font-medium transition-colors duration-[130ms]",
        active ? "bg-ink-soft text-ink" : "text-fg-muted hover:bg-sunken hover:text-fg",
      )}
    >
      <Icon size={15} />
      <span className="truncate">{label}</span>
    </Link>
  );
}

/** The destinations this account can actually reach: the admin panel is
 *  appended only for accounts on the server's allowlist, so nobody is shown
 *  a link that would 403. */
function useNav() {
  const { me } = useSession();
  return me?.admin ? [...NAV, ADMIN_NAV] : [...NAV];
}

function MobileTabs() {
  const isActive = useIsActive();
  const nav = useNav();

  return (
    <nav className="fixed inset-x-0 bottom-0 z-30 border-t border-line bg-raised pb-[env(safe-area-inset-bottom)] md:hidden">
      {/* One column per destination: hardcoding five wrapped the bar onto a
          second row the moment a sixth was added. */}
      <ul className="grid" style={{ gridTemplateColumns: `repeat(${nav.length}, minmax(0, 1fr))` }}>
        {nav.map((item) => {
          const active = isActive(item.href);
          const Icon = item.icon;
          return (
            <li key={item.href}>
              <Link
                href={item.href}
                aria-current={active ? "page" : undefined}
                className={cx(
                  "flex h-14 flex-col items-center justify-center gap-1 px-0.5 text-[10px] font-medium",
                  "min-w-0 [&>span]:w-full [&>span]:truncate [&>span]:text-center",
                  active ? "text-ink" : "text-fg-faint",
                )}
              >
                <Icon size={17} className="shrink-0" />
                <span>{item.label}</span>
              </Link>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}
