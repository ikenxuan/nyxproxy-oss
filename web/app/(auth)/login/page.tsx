'use client';

import { Languages, Monitor, Moon, Sun } from 'lucide-react';
import { useRouter } from 'next/navigation';
import { useEffect, useState } from 'react';
import { BrandMark } from '@/components/BrandMark';
import { LoginForm } from '@/components/LoginForm';
import { Button } from '@/components/ui/Button';
import { errorMessage, useI18n } from '@/lib/i18n';
import { useAuth } from '@/lib/auth';
import { useTheme } from '@/lib/theme';

export default function LoginPage() {
  const { t, lang, setLang } = useI18n();
  const { theme, cycle } = useTheme();
  const { login, username, loading } = useAuth();
  const router = useRouter();
  const [error, setError] = useState('');

  useEffect(() => {
    if (!loading && username) router.replace('/');
  }, [loading, username, router]);

  const themeLabel =
    theme === 'dark' ? t('theme.dark') : theme === 'light' ? t('theme.light') : t('theme.system');
  const ThemeIcon = theme === 'dark' ? Moon : theme === 'light' ? Sun : Monitor;

  return (
    <main className="relative flex min-h-dvh items-center justify-center overflow-hidden bg-canvas px-4 py-12">
      <div className="aurora pointer-events-none absolute inset-0" aria-hidden="true" />
      <div className="grid-fade pointer-events-none absolute inset-0 opacity-60" aria-hidden="true" />

      <div className="absolute right-4 top-4 z-10 flex items-center gap-1">
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
        <Button
          variant="ghost"
          size="sm"
          onClick={cycle}
          aria-label={`${t('top.theme')}: ${themeLabel}`}
          title={themeLabel}
        >
          <ThemeIcon size={16} aria-hidden="true" />
        </Button>
      </div>

      <div className="relative z-10 w-full max-w-sm">
        <div className="mb-7 flex flex-col items-center text-center">
          <BrandMark size="lg" />
          <p className="mt-4 text-[11px] font-semibold uppercase tracking-[0.18em] text-faint" translate="no">
            NyxProxy
          </p>
          <h1 className="mt-2 text-balance text-[26px] font-semibold leading-tight tracking-[-0.02em] text-ink">
            {t('login.title')}
          </h1>
          <p className="mt-2 max-w-xs text-pretty text-sm leading-relaxed text-muted">
            {t('login.subtitle')}
          </p>
        </div>

        <div className="rounded-card border border-line bg-surface/85 p-6 shadow-float backdrop-blur-xl sm:p-7">
          <LoginForm
            error={error}
            onSubmit={async (user, password) => {
              setError('');
              try {
                await login(user, password);
                router.replace('/');
              } catch (err) {
                setError(errorMessage(t, err));
              }
            }}
          />
        </div>

        <p className="mt-6 text-center text-[11px] leading-relaxed text-faint">{t('login.footer')}</p>
      </div>
    </main>
  );
}
