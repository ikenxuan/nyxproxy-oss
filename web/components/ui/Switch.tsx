'use client';

import { cn } from '@/lib/cn';

interface Props {
  checked: boolean;
  onChange: (checked: boolean) => void;
  label: string;
  disabled?: boolean;
}

export function Switch({ checked, onChange, label, disabled }: Props) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className={cn(
        'relative inline-flex h-[26px] w-[44px] shrink-0 items-center rounded-full border border-transparent transition-[background-color,box-shadow] duration-200 ease-out focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/50 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas disabled:cursor-not-allowed disabled:opacity-45 motion-reduce:transition-none',
        checked
          ? 'bg-accent shadow-[inset_0_1px_2px_rgba(0,0,0,0.12)]'
          : 'bg-sunken shadow-[inset_0_1px_2px_rgba(0,0,0,0.08)] dark:bg-surface-2',
      )}
    >
      <span
        className={cn(
          'pointer-events-none inline-block h-[22px] w-[22px] transform rounded-full bg-white shadow-[0_1px_3px_rgba(0,0,0,0.25)] transition-transform duration-200 ease-out motion-reduce:transition-none',
          checked ? 'translate-x-[19px]' : 'translate-x-[1px]',
        )}
      />
    </button>
  );
}
