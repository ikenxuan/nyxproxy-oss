'use client';

import Link from 'next/link';
import { usePathname, useRouter } from 'next/navigation';
import {
  Languages,
  LayoutDashboard,
  LogOut,
  Menu,
  Monitor,
  Moon,
  ScrollText,
  Settings,
  Sun,
  Wifi,
  WifiOff,
  X,
} from 'lucide-react';
import { useEffect, useState, useSyncExternalStore, type MouseEvent, type ReactNode } from 'react';
import { Brand, BrandMark } from '@/components/BrandMark';
import { Button } from '@/components/ui/Button';
import { Skeleton } from '@/components/ui/Skeleton';
import { useAuth } from '@/lib/auth';
import { cn } from '@/lib/cn';
import { useDirty } from '@/lib/dirty';
import { useI18n } from '@/lib/i18n';
import { useTheme } from '@/lib/theme';

function subscribeOnline(onChange: () => void): () => void {
  window.addEventListener('online', onChange);
  window.addEventListener('offline', onChange);
  return () => {
    window.removeEventListener('online', onChange);
    window.removeEventListener('offline', onChange);
  };
}

function getOnline(): boolean {
  return navigator.onLine;
}

function getServerOnline(): boolean {
  return true;
}

function LanguageButton() {
  const { t, lang, setLang } = useI18n();
  return (
    <Button
      variant="ghost"
      size="sm"
      onClick={() => setLang(lang === 'zh' ? 'en' : 'zh')}
      aria-label={t('top.language')}
      title={t('top.language')}
    >
      <Languages size={16} aria-hidden="true" />
      <span className="text-[12px] font-semibold">{lang === 'zh' ? 'EN' : '中文'}</span>
    </Button>
  );
}

function ThemeButton() {
  const { t } = useI18n();
  const { theme, cycle } = useTheme();
  const label = theme === 'dark' ? t('theme.dark') : theme === 'light' ? t('theme.light') : t('theme.system');
  const Icon = theme === 'dark' ? Moon : theme === 'light' ? Sun : Monitor;
  return (
    <Button
      variant="ghost"
      size="sm"
      onClick={cycle}
      aria-label={`${t('top.theme')}: ${label}`}
      title={label}
    >
      <Icon size={16} aria-hidden="true" />
    </Button>
  );
}

function LogoutButton({ withLabel = false }: { withLabel?: boolean }) {
  const { t } = useI18n();
  const { logout } = useAuth();
  const { dirty } = useDirty();
  const router = useRouter();
  return (
    <Button
      variant="ghost"
      size="sm"
      className={withLabel ? 'w-full justify-start' : undefined}
      aria-label={t('nav.logout')}
      onClick={async () => {
        if (dirty && !window.confirm(t('config.unsaved_confirm'))) return;
        try {
          await logout();
        } finally {
          router.replace('/login');
        }
      }}
    >
      <LogOut size={16} aria-hidden="true" />
      {withLabel && <span>{t('nav.logout')}</span>}
    </Button>
  );
}

export function AppShell({ children }: { children: ReactNode }) {
  const { t } = useI18n();
  const { username, loading } = useAuth();
  const { dirty } = useDirty();
  const pathname = usePathname();
  const router = useRouter();
  const [mobileOpen, setMobileOpen] = useState(false);
  const online = useSyncExternalStore(subscribeOnline, getOnline, getServerOnline);

  useEffect(() => {
    if (!loading && !username) router.replace('/login');
  }, [loading, username, router]);

  useEffect(() => {
    if (!mobileOpen) return;
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setMobileOpen(false);
    };
    document.addEventListener('keydown', onKey);
    return () => {
      document.body.style.overflow = previousOverflow;
      document.removeEventListener('keydown', onKey);
    };
  }, [mobileOpen]);

  if (loading) {
    return (
      <div className="flex min-h-dvh items-center justify-center bg-canvas">
        <div className="flex w-full max-w-xs flex-col items-center gap-4 px-6">
          <BrandMark size="lg" />
          <Skeleton className="h-4 w-32" />
          <Skeleton className="h-2 w-20" />
        </div>
      </div>
    );
  }
  if (!username) return null;

  const section = pathname.startsWith('/config')
    ? { title: t('config.title'), subtitle: t('page.config.subtitle') }
    : pathname.startsWith('/logs')
      ? { title: t('logs.title'), subtitle: t('page.logs.subtitle') }
      : { title: t('dashboard.title'), subtitle: t('page.dashboard.subtitle') };

  const links = [
    { href: '/', label: t('nav.dashboard'), icon: LayoutDashboard },
    { href: '/config/', label: t('nav.config'), icon: Settings },
    { href: '/logs/', label: t('nav.logs'), icon: ScrollText },
  ];

  const navigate = (event: MouseEvent<HTMLAnchorElement>) => {
    if (dirty && !window.confirm(t('config.unsaved_confirm'))) {
      event.preventDefault();
      return;
    }
    setMobileOpen(false);
  };

  const isActive = (href: string) => {
    if (href === '/') return pathname === '/';
    return pathname === href.replace(/\/$/, '') || pathname.startsWith(href);
  };

  const nav = (
    <nav className="space-y-0.5" aria-label={t('nav.primary')}>
      {links.map(({ href, label, icon: Icon }) => {
        const active = isActive(href);
        return (
          <Link
            key={href}
            href={href}
            onClick={navigate}
            aria-current={active ? 'page' : undefined}
            className={cn(
              'group flex items-center gap-3 rounded-xl px-3 py-2.5 text-[13px] font-medium transition-[background-color,color] duration-150',
              active
                ? 'bg-accent-soft text-accent'
                : 'text-muted hover:bg-surface-2 hover:text-ink',
            )}
          >
            <Icon
              size={17}
              aria-hidden="true"
              className={cn(
                'transition-colors',
                active ? 'text-accent' : 'text-faint group-hover:text-ink',
              )}
            />
            <span className="truncate">{label}</span>
          </Link>
        );
      })}
    </nav>
  );

  const userCard = (
    <div className="rounded-field border border-line bg-surface-2/60 p-3">
      <p className="text-[11px] text-faint">{t('top.signed_in_as')}</p>
      <div className="mt-1 flex items-center gap-2.5">
        <span
          aria-hidden="true"
          className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-accent-soft text-[12px] font-semibold text-accent"
        >
          {username.trim().charAt(0).toUpperCase() || '?'}
        </span>
        <span className="min-w-0 truncate text-[13px] font-medium text-ink">{username}</span>
      </div>
    </div>
  );

  return (
    <div className="min-h-dvh bg-canvas">
      <a
        href="#main-content"
        className="sr-only focus:not-sr-only focus:fixed focus:left-4 focus:top-4 focus:z-[60] focus:rounded-full focus:bg-accent focus:px-4 focus:py-2 focus:text-sm focus:font-medium focus:text-white focus:shadow-float"
      >
        {t('nav.skip')}
      </a>

      <aside className="glass fixed inset-y-0 left-0 z-40 hidden w-64 flex-col border-r border-line lg:flex">
        <div className="flex h-16 items-center px-5">
          <Brand subtitle={t('app.console')} />
        </div>
        <div className="flex flex-1 flex-col px-3 pb-4">
          <p className="px-3 pb-2 pt-2 text-[10px] font-semibold uppercase tracking-[0.16em] text-faint">
            {t('nav.primary')}
          </p>
          {nav}
          <div className="mt-auto space-y-3 pt-6">
            <div className="flex items-center gap-2 px-3 text-[11px] text-faint">
              {online ? (
                <Wifi size={13} aria-hidden="true" className="text-success" />
              ) : (
                <WifiOff size={13} aria-hidden="true" className="text-danger" />
              )}
              <span>{online ? t('top.connected') : t('top.disconnected')}</span>
            </div>
            {userCard}
            <div className="flex items-center gap-1">
              <LanguageButton />
              <ThemeButton />
              <div className="ml-auto">
                <LogoutButton />
              </div>
            </div>
          </div>
        </div>
      </aside>

      {mobileOpen && (
        <div className="fixed inset-0 z-50 lg:hidden">
          <div
            className="absolute inset-0 bg-black/35 backdrop-blur-sm"
            onClick={() => setMobileOpen(false)}
            aria-hidden="true"
          />
          <aside
            id="mobile-navigation"
            className="animate-slide-in absolute inset-y-0 left-0 flex w-[min(84vw,20rem)] flex-col border-r border-line bg-surface p-4 shadow-float"
            aria-label={t('nav.primary')}
          >
            <div className="flex items-center justify-between gap-3 px-1">
              <Brand subtitle={t('app.console')} />
              <button
                type="button"
                onClick={() => setMobileOpen(false)}
                aria-label={t('common.close')}
                className="inline-flex h-9 w-9 items-center justify-center rounded-full text-faint transition-colors hover:bg-surface-2 hover:text-ink focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/50"
              >
                <X size={18} aria-hidden="true" />
              </button>
            </div>
            <div className="mt-6 flex flex-1 flex-col">
              {nav}
              <div className="mt-auto space-y-3 pt-6">
                <div className="flex items-center gap-2 px-3 text-[11px] text-faint">
                  {online ? (
                    <Wifi size={13} aria-hidden="true" className="text-success" />
                  ) : (
                    <WifiOff size={13} aria-hidden="true" className="text-danger" />
                  )}
                  <span>{online ? t('top.connected') : t('top.disconnected')}</span>
                </div>
                {userCard}
                <div className="flex items-center gap-1 border-t border-line pt-3">
                  <LanguageButton />
                  <ThemeButton />
                  <div className="ml-auto">
                    <LogoutButton />
                  </div>
                </div>
              </div>
            </div>
          </aside>
        </div>
      )}

      <div className="lg:pl-64">
        <header className="glass sticky top-0 z-30 flex h-16 items-center gap-3 border-b border-line px-4 sm:px-6 lg:px-8">
          <button
            type="button"
            onClick={() => setMobileOpen(true)}
            aria-label={t('nav.open_menu')}
            aria-expanded={mobileOpen}
            aria-controls="mobile-navigation"
            className="-ml-1 inline-flex h-9 w-9 items-center justify-center rounded-full text-muted transition-colors hover:bg-surface-2 hover:text-ink focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/50 lg:hidden"
          >
            <Menu size={20} aria-hidden="true" />
          </button>
          <div className="min-w-0">
            <h1 className="truncate text-[15px] font-semibold tracking-tight text-ink sm:text-base">
              {section.title}
            </h1>
            <p className="hidden truncate text-[11px] text-faint sm:block">{section.subtitle}</p>
          </div>
          <div className="ml-auto flex items-center gap-0.5 sm:gap-1">
            <span
              className={cn(
                'mr-1 hidden items-center gap-1.5 rounded-full px-2.5 py-1 text-[11px] font-medium md:inline-flex',
                online ? 'bg-success-soft text-success' : 'bg-danger-soft text-danger',
              )}
            >
              <span className="h-1.5 w-1.5 rounded-full bg-current" aria-hidden="true" />
              {online ? t('top.connected') : t('top.disconnected')}
            </span>
            <LanguageButton />
            <ThemeButton />
            <span className="mx-1 hidden h-5 w-px bg-line sm:block" aria-hidden="true" />
            <LogoutButton />
          </div>
        </header>

        <main
          id="main-content"
          tabIndex={-1}
          className="mx-auto w-full max-w-6xl px-4 py-6 outline-none sm:px-6 lg:px-8 lg:py-9"
        >
          <div key={pathname} className="animate-fade-up">
            {children}
          </div>
        </main>
      </div>
    </div>
  );
}
