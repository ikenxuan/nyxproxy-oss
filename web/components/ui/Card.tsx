import type { ReactNode } from 'react';
import { cn } from '@/lib/cn';

interface Props {
  id?: string;
  title?: ReactNode;
  description?: ReactNode;
  eyebrow?: ReactNode;
  actions?: ReactNode;
  children: ReactNode;
  className?: string;
  contentClassName?: string;
}

export function Card({
  id,
  title,
  description,
  eyebrow,
  actions,
  children,
  className,
  contentClassName,
}: Props) {
  const hasHeader = Boolean(title || description || eyebrow || actions);
  return (
    <section
      id={id}
      className={cn(
        'rounded-card border border-line bg-surface p-5 shadow-card sm:p-6',
        className,
      )}
    >
      {hasHeader && (
        <header className="flex items-start justify-between gap-4">
          <div className="min-w-0">
            {eyebrow && (
              <p className="text-[11px] font-semibold uppercase tracking-[0.14em] text-faint">{eyebrow}</p>
            )}
            {title && (
              <h2 className="text-base font-semibold tracking-tight text-ink text-balance">{title}</h2>
            )}
            {description && <p className="mt-1 text-sm text-muted text-pretty">{description}</p>}
          </div>
          {actions && <div className="shrink-0">{actions}</div>}
        </header>
      )}
      <div className={cn(hasHeader && 'mt-5', contentClassName)}>{children}</div>
    </section>
  );
}
