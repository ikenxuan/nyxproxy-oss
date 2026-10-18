import { CheckCircle2, CircleAlert, Info, TriangleAlert } from 'lucide-react';
import type { ReactNode } from 'react';
import { cn } from '@/lib/cn';

type Tone = 'info' | 'warning' | 'danger' | 'success';

const tones: Record<Tone, { box: string; icon: typeof Info }> = {
  info: { box: 'border-accent/20 bg-accent-soft text-ink', icon: Info },
  warning: { box: 'border-warning/25 bg-warning-soft text-ink', icon: TriangleAlert },
  danger: { box: 'border-danger/25 bg-danger-soft text-ink', icon: CircleAlert },
  success: { box: 'border-success/25 bg-success-soft text-ink', icon: CheckCircle2 },
};

export function Banner({ tone = 'info', children }: { tone?: Tone; children: ReactNode }) {
  const { box, icon: Icon } = tones[tone];
  return (
    <div
      role={tone === 'danger' ? 'alert' : 'status'}
      aria-live={tone === 'danger' ? 'assertive' : 'polite'}
      className={cn('flex items-start gap-3 rounded-field border px-4 py-3 text-sm', box)}
    >
      <Icon
        size={16}
        aria-hidden="true"
        className={cn(
          'mt-0.5 shrink-0',
          tone === 'danger' && 'text-danger',
          tone === 'warning' && 'text-warning',
          tone === 'success' && 'text-success',
          tone === 'info' && 'text-accent',
        )}
      />
      <div className="min-w-0 flex-1">{children}</div>
    </div>
  );
}
