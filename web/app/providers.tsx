'use client';

import type { ReactNode } from 'react';
import { ToastProvider } from '@/components/ui/Toast';
import { AuthProvider } from '@/lib/auth';
import { DirtyProvider } from '@/lib/dirty';
import { I18nProvider } from '@/lib/i18n';
import { ThemeProvider } from '@/lib/theme';

export function Providers({ children }: { children: ReactNode }) {
  return (
    <I18nProvider>
      <ThemeProvider>
        <AuthProvider>
          <DirtyProvider>
            <ToastProvider>{children}</ToastProvider>
          </DirtyProvider>
        </AuthProvider>
      </ThemeProvider>
    </I18nProvider>
  );
}
