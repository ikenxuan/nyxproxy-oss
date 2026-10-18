'use client';

import { ChevronDown } from 'lucide-react';
import { forwardRef, type SelectHTMLAttributes } from 'react';
import { cn } from '@/lib/cn';

const field =
  'h-10 w-full appearance-none rounded-field border border-line-strong bg-surface px-3.5 pr-9 text-base text-ink outline-none transition-[border-color,box-shadow,background-color] duration-150 hover:border-line-strong focus:border-accent focus:ring-4 focus:ring-accent/15 disabled:cursor-not-allowed disabled:bg-sunken disabled:text-faint sm:text-sm';

export const Select = forwardRef<HTMLSelectElement, SelectHTMLAttributes<HTMLSelectElement>>(
  function Select({ className, children, ...props }, ref) {
    return (
      <span className="relative block w-full">
        <select
          ref={ref}
          className={cn(field, 'dark:[color-scheme:dark]', className)}
          {...props}
        >
          {children}
        </select>
        <ChevronDown
          size={16}
          aria-hidden="true"
          className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-faint"
        />
      </span>
    );
  },
);
