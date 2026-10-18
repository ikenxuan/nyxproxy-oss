'use client';

import { CheckCircle2, CircleAlert, Info } from 'lucide-react';
import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from 'react';
import { cn } from '@/lib/cn';

type Kind = 'success' | 'error' | 'info';

interface ToastItem {
  id: number;
  message: string;
  kind: Kind;
}

interface ToastValue {
  toast: (message: string, kind?: Kind) => void;
}

const ToastContext = createContext<ToastValue | null>(null);

const styles: Record<Kind, { box: string; icon: typeof Info }> = {
  success: { box: 'border-success/25 bg-success-soft text-ink', icon: CheckCircle2 },
  error: { box: 'border-danger/25 bg-danger-soft text-ink', icon: CircleAlert },
  info: { box: 'border-accent/20 bg-accent-soft text-ink', icon: Info },
};

const iconTone: Record<Kind, string> = {
  success: 'text-success',
  error: 'text-danger',
  info: 'text-accent',
};

export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([]);

  const toast = useCallback((message: string, kind: Kind = 'info') => {
    const id = Date.now() + Math.random();
    setItems((current) => [...current, { id, message, kind }]);
    window.setTimeout(() => {
      setItems((current) => current.filter((item) => item.id !== id));
    }, 5000);
  }, []);

  const value = useMemo(() => ({ toast }), [toast]);
  return (
    <ToastContext.Provider value={value}>
      {children}
      <div
        aria-live="polite"
        className="pointer-events-none fixed inset-x-4 bottom-4 z-50 flex flex-col items-center gap-2 sm:inset-x-auto sm:right-5 sm:bottom-5 sm:items-end"
      >
        {items.map((item) => {
          const { box, icon: Icon } = styles[item.kind];
          return (
            <div
              key={item.id}
              role={item.kind === 'error' ? 'alert' : 'status'}
              className={cn(
                'animate-pop-in pointer-events-auto flex w-full max-w-sm items-start gap-3 rounded-field border px-4 py-3 text-sm shadow-float backdrop-blur-xl',
                box,
              )}
            >
              <Icon size={16} aria-hidden="true" className={cn('mt-0.5 shrink-0', iconTone[item.kind])} />
              <span className="min-w-0 flex-1 text-pretty">{item.message}</span>
            </div>
          );
        })}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast(): ToastValue {
  const value = useContext(ToastContext);
  if (!value) throw new Error('useToast must be used inside ToastProvider');
  return value;
}
