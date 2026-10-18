'use client';

import Link from 'next/link';
import { usePathname, useRouter } from 'next/navigation';
import {
  LayoutDashboard,
  LogOut,
  Menu,
  Network,
  ScrollText,
  Settings,
  Wifi,
  WifiOff,
} from 'lucide-react';
import { useEffect, useState, useSyncExternalStore, type MouseEvent, type ReactNode } from 'react';
import {
  AlertDialog,
  Avatar,
  Button,
  Chip,
  Drawer,
  Skeleton,
  Tooltip,
  useOverlayState,
} from '@heroui/react';
import { Brand, BrandMark } from '@/components/BrandMark';
import { LanguageMenu, ThemeMenu } from '@/components/ChromeControls';
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

type Pending = { kind: 'navigate'; href: string } | { kind: 'logout' } | null;

export function AppShell({ children }: { children: ReactNode }) {
  const { t } = useI18n();
  const { username, loading, logout } = useAuth();
  const { dirty, setDirty } = useDirty();
  const { theme } = useTheme();
  const pathname = usePathname();
  const router = useRouter();
  const drawer = useOverlayState();
  const online = useSyncExternalStore(subscribeOnline, getOnline, getServerOnline);
  const [pending, setPending] = useState<Pending>(null);

  useEffect(() => {
    if (!loading && !username) router.replace('/login');
  }, [loading, username, router]);

  if (loading) {
    return (
      <div className="flex min-h-dvh items-center justify-center bg-background">
        <div className="flex w-full max-w-xs flex-col items-center gap-4 px-6">
          <BrandMark size="lg" />
          <Skeleton className="h-4 w-32 rounded-full" />
          <Skeleton className="h-2 w-20 rounded-full" />
        </div>
      </div>
    );
  }
  if (!username) return null;

  const section = pathname.startsWith('/config')
    ? { title: t('config.title'), subtitle: t('page.config.subtitle') }
    : pathname.startsWith('/logs')
      ? { title: t('logs.title'), subtitle: t('page.logs.subtitle') }
      : pathname.startsWith('/pool')
        ? { title: t('pool.title'), subtitle: t('page.pool.subtitle') }
        : { title: t('dashboard.title'), subtitle: t('page.dashboard.subtitle') };

  const links = [
    { href: '/', label: t('nav.dashboard'), icon: LayoutDashboard },
    { href: '/pool/', label: t('nav.pool'), icon: Network },
    { href: '/config/', label: t('nav.config'), icon: Settings },
    { href: '/logs/', label: t('nav.logs'), icon: ScrollText },
  ];

  const isActive = (href: string) => {
    if (href === '/') return pathname === '/';
    return pathname === href.replace(/\/$/, '') || pathname.startsWith(href);
  };

  const go = (href: string) => {
    drawer.close();
    router.push(href);
  };

  const requestNavigation = (event: MouseEvent<HTMLAnchorElement>, href: string) => {
    if (dirty) {
      event.preventDefault();
      setPending({ kind: 'navigate', href });
      return;
    }
    drawer.close();
  };

  const requestLogout = async () => {
    if (dirty) {
      setPending({ kind: 'logout' });
      return;
    }
    await performLogout();
  };

  const performLogout = async () => {
    try {
      await logout();
    } finally {
      setDirty(false);
      drawer.close();
      router.replace('/login');
    }
  };

  const confirmPending = async () => {
    const action = pending;
    setPending(null);
    if (!action) return;
    setDirty(false);
    if (action.kind === 'navigate') go(action.href);
    else await performLogout();
  };

  const nav = (
    <nav className="space-y-1" aria-label={t('nav.primary')}>
      {links.map(({ href, label, icon: Icon }) => {
        const active = isActive(href);
        return (
          <Link
            key={href}
            href={href}
            onClick={(event) => requestNavigation(event, href)}
            aria-current={active ? 'page' : undefined}
            className={cn(
              'group flex items-center gap-3 rounded-2xl px-3 py-2.5 text-[13px] font-medium transition-[background-color,color,transform] duration-150',
              active
                ? 'bg-accent-soft text-accent-soft-foreground'
                : 'text-muted hover:bg-default hover:text-foreground',
            )}
          >
            <span
              aria-hidden="true"
              className={cn(
                'flex h-7 w-7 items-center justify-center rounded-xl transition-colors',
                active ? 'bg-accent text-accent-foreground' : 'bg-default/70 text-muted group-hover:text-foreground',
              )}
            >
              <Icon size={15} />
            </span>
            <span className="truncate">{label}</span>
          </Link>
        );
      })}
    </nav>
  );

  const userCard = (
    <div className="rounded-2xl border border-border bg-surface-secondary/60 p-3">
      <p className="text-[11px] text-muted">{t('top.signed_in_as')}</p>
      <div className="mt-2 flex items-center gap-2.5">
        <Avatar size="sm" className="bg-accent-soft text-accent-soft-foreground">
          <Avatar.Fallback>{username.trim().charAt(0).toUpperCase() || '?'}</Avatar.Fallback>
        </Avatar>
        <span className="min-w-0 flex-1 truncate text-[13px] font-medium text-foreground">{username}</span>
        <Tooltip>
          <Button
            variant="ghost"
            size="sm"
            isIconOnly
            aria-label={t('nav.logout')}
            className="text-muted"
            onPress={() => void requestLogout()}
          >
            <LogOut size={15} aria-hidden="true" />
          </Button>
          <Tooltip.Content>{t('nav.logout')}</Tooltip.Content>
        </Tooltip>
      </div>
    </div>
  );

  const footer = (
    <div className="mt-auto space-y-3 pt-6">
      <div className="flex items-center gap-2 px-3 text-[11px] text-muted">
        {online ? (
          <Wifi size={13} aria-hidden="true" className="text-success" />
        ) : (
          <WifiOff size={13} aria-hidden="true" className="text-danger" />
        )}
        <span>{online ? t('top.connected') : t('top.disconnected')}</span>
      </div>
      {userCard}
      <div className="flex items-center gap-1 px-1">
        <LanguageMenu />
        <ThemeMenu />
        <span className="ml-auto text-[10px] font-medium uppercase tracking-[0.14em] text-muted/70">
          {theme === 'system' ? t('theme.system') : theme === 'dark' ? t('theme.dark') : t('theme.light')}
        </span>
      </div>
    </div>
  );

  return (
    <div className="min-h-dvh bg-background">
      <a
        href="#main-content"
        className="sr-only focus:not-sr-only focus:fixed focus:left-4 focus:top-4 focus:z-[60] focus:rounded-full focus:bg-accent focus:px-4 focus:py-2 focus:text-sm focus:font-medium focus:text-accent-foreground focus:shadow-overlay"
      >
        {t('nav.skip')}
      </a>

      <aside className="glass fixed inset-y-0 left-0 z-40 hidden w-[264px] flex-col border-r border-border lg:flex">
        <div className="flex h-16 items-center px-5">
          <Brand subtitle={t('app.console')} />
        </div>
        <div className="flex flex-1 flex-col px-3 pb-4">
          <p className="px-3 pb-2 pt-2 text-[10px] font-semibold uppercase tracking-[0.16em] text-muted">
            {t('nav.primary')}
          </p>
          {nav}
          {footer}
        </div>
      </aside>

      <Drawer state={drawer}>
        <Drawer.Backdrop variant="blur">
          <Drawer.Content placement="left">
            <Drawer.Dialog aria-label={t('nav.primary')}>
              <Drawer.CloseTrigger aria-label={t('common.close')} />
              <Drawer.Header>
                <Brand subtitle={t('app.console')} />
              </Drawer.Header>
              <Drawer.Body className="flex flex-col">
                {nav}
                {footer}
              </Drawer.Body>
            </Drawer.Dialog>
          </Drawer.Content>
        </Drawer.Backdrop>
      </Drawer>

      <div className="lg:pl-[264px]">
        <header className="glass sticky top-0 z-30 flex h-16 items-center gap-3 border-b border-border px-4 sm:px-6 lg:px-8">
          <Button
            variant="ghost"
            size="sm"
            isIconOnly
            aria-label={t('nav.open_menu')}
            className="-ml-1 text-muted lg:hidden"
            onPress={drawer.open}
          >
            <Menu size={20} aria-hidden="true" />
          </Button>
          <div className="min-w-0">
            <h1 className="truncate text-[15px] font-semibold tracking-tight text-foreground sm:text-base">
              {section.title}
            </h1>
            <p className="hidden truncate text-[11px] text-muted sm:block">{section.subtitle}</p>
          </div>
          <div className="ml-auto flex items-center gap-0.5 sm:gap-1">
            <Chip
              size="sm"
              variant="soft"
              color={online ? 'success' : 'danger'}
              className="mr-1 hidden md:inline-flex"
            >
              <span className="h-1.5 w-1.5 rounded-full bg-current" aria-hidden="true" />
              <Chip.Label>{online ? t('top.connected') : t('top.disconnected')}</Chip.Label>
            </Chip>
            <LanguageMenu />
            <ThemeMenu />
            <span className="mx-1 hidden h-5 w-px bg-border sm:block" aria-hidden="true" />
            <Tooltip>
              <Button
                variant="ghost"
                size="sm"
                isIconOnly
                aria-label={t('nav.logout')}
                className="text-muted"
                onPress={() => void requestLogout()}
              >
                <LogOut size={17} aria-hidden="true" />
              </Button>
              <Tooltip.Content>{t('nav.logout')}</Tooltip.Content>
            </Tooltip>
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

      <AlertDialog isOpen={pending !== null} onOpenChange={(open) => !open && setPending(null)}>
        <AlertDialog.Backdrop>
          <AlertDialog.Container>
            <AlertDialog.Dialog className="sm:max-w-[420px]">
              <AlertDialog.CloseTrigger aria-label={t('common.close')} />
              <AlertDialog.Header>
                <AlertDialog.Icon status="warning" />
                <AlertDialog.Heading>
                  {pending?.kind === 'logout' ? t('nav.logout') : t('config.leave_title')}
                </AlertDialog.Heading>
              </AlertDialog.Header>
              <AlertDialog.Body>
                <p className="text-sm leading-relaxed text-muted">
                  {pending?.kind === 'logout' ? t('config.unsaved_confirm') : t('config.leave_body')}
                </p>
              </AlertDialog.Body>
              <AlertDialog.Footer>
                <Button slot="close" variant="secondary">
                  {t('common.stay')}
                </Button>
                <Button variant="danger" onPress={() => void confirmPending()}>
                  {pending?.kind === 'logout' ? t('nav.logout') : t('config.discard')}
                </Button>
              </AlertDialog.Footer>
            </AlertDialog.Dialog>
          </AlertDialog.Container>
        </AlertDialog.Backdrop>
      </AlertDialog>
    </div>
  );
}
