import { forwardRef, type InputHTMLAttributes } from 'react';
import { cn } from '@/lib/cn';

const field =
  'w-full rounded-field border border-line-strong bg-surface text-ink shadow-[inset_0_1px_0_rgba(255,255,255,0.03)] outline-none transition-[border-color,box-shadow,background-color] duration-150 placeholder:text-faint hover:border-line-strong focus:border-accent focus:ring-4 focus:ring-accent/15 disabled:cursor-not-allowed disabled:bg-sunken disabled:text-faint aria-invalid:border-danger aria-invalid:focus:ring-danger/15';

export const Input = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement>>(
  function Input({ className, ...props }, ref) {
    return (
      <input
        ref={ref}
        className={cn(field, 'h-10 px-3.5 text-base sm:text-sm', className)}
        {...props}
      />
    );
  },
);
