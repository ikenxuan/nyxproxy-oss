'use client';

import { useRouter } from 'next/navigation';
import { useEffect, useState } from 'react';
import { Card } from '@heroui/react';
import { BrandMark } from '@/components/BrandMark';
import { LanguageMenu, ThemeMenu } from '@/components/ChromeControls';
import { LoginForm } from '@/components/LoginForm';
import { useAuth } from '@/lib/auth';
import { errorMessage, useI18n } from '@/lib/i18n';

export default function LoginPage() {
  const { t } = useI18n();
  const { login, username, loading } = useAuth();
  const router = useRouter();
  const [error, setError] = useState('');

  useEffect(() => {
    if (!loading && username) router.replace('/');
  }, [loading, username, router]);

  return (
    <main className="relative flex min-h-dvh items-center justify-center overflow-hidden bg-background px-4 py-12">
      <div className="aurora pointer-events-none absolute inset-0" aria-hidden="true" />
      <div className="grid-fade pointer-events-none absolute inset-0 opacity-60" aria-hidden="true" />

      <div className="absolute right-3 top-3 z-10 flex items-center gap-1 sm:right-5 sm:top-5">
        <LanguageMenu />
        <ThemeMenu />
      </div>

      <div className="relative z-10 w-full max-w-[26rem]">
        <div className="mb-7 flex flex-col items-center text-center">
          <BrandMark size="lg" />
          <p className="mt-4 text-[11px] font-semibold uppercase tracking-[0.18em] text-muted" translate="no">
            NyxProxy
          </p>
          <h1 className="mt-2 text-balance text-[27px] font-semibold leading-tight tracking-[-0.02em] text-foreground">
            {t('login.title')}
          </h1>
          <p className="mt-2 max-w-xs text-pretty text-sm leading-relaxed text-muted">
            {t('login.subtitle')}
          </p>
        </div>

        <Card className="border border-border/70 bg-surface/80 backdrop-blur-2xl">
          <Card.Content className="gap-0">
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
          </Card.Content>
        </Card>

        <p className="mt-6 text-center text-[11px] leading-relaxed text-muted">{t('login.footer')}</p>
      </div>
    </main>
  );
}
