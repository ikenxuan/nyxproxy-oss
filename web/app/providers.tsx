'use client';

import type { ReactNode } from 'react';
import { Toast } from '@heroui/react';
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
            {children}
            <Toast.Provider placement="bottom end" maxVisibleToasts={3} />
          </DirtyProvider>
        </AuthProvider>
      </ThemeProvider>
    </I18nProvider>
  );
}
