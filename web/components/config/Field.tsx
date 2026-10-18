import { cloneElement, isValidElement, type ReactNode } from 'react';
import { Badge } from '@/components/ui/Badge';

interface Props {
  id?: string;
  label: string;
  hint?: string;
  error?: string;
  locked?: 'env' | 'restart_required';
  lockLabel?: string;
  children: ReactNode;
}

export function Field({ id, label, hint, error, locked, lockLabel, children }: Props) {
  const hintId = id ? `${id}-hint` : undefined;
  const errorId = id ? `${id}-error` : undefined;
  const describedBy =
    [hintId && hint && !error ? hintId : null, errorId && error ? errorId : null]
      .filter(Boolean)
      .join(' ') || undefined;

  const control =
    id && isValidElement<Record<string, unknown>>(children)
      ? cloneElement(children, {
          'aria-describedby': describedBy,
          'aria-invalid': error ? true : undefined,
        })
      : children;

  return (
    <div className="min-w-0 space-y-1.5">
      <div className="flex min-h-5 flex-wrap items-center justify-between gap-x-2 gap-y-1">
        <label htmlFor={id} className="text-[13px] font-medium text-ink">
          {label}
        </label>
        {locked && <Badge tone={locked === 'env' ? 'warning' : 'neutral'}>{lockLabel}</Badge>}
      </div>
      {control}
      {hint && !error && (
        <p id={hintId} className="text-xs leading-relaxed text-muted">
          {hint}
        </p>
      )}
      {error && (
        <p id={errorId} role="alert" className="text-xs font-medium leading-relaxed text-danger">
          {error}
        </p>
      )}
    </div>
  );
}
