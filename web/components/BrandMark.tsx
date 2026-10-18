import { cn } from '@/lib/cn';

const sizes = {
  sm: 'h-8 w-8 rounded-[10px] text-[13px]',
  md: 'h-9 w-9 rounded-[11px] text-sm',
  lg: 'h-12 w-12 rounded-2xl text-lg',
};

export function BrandMark({
  size = 'md',
  className,
}: {
  size?: keyof typeof sizes;
  className?: string;
}) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        'relative inline-flex shrink-0 select-none items-center justify-center overflow-hidden font-bold text-[#04222b] shadow-[0_2px_12px_-3px_rgba(0,173,216,0.7)]',
        sizes[size],
        className,
      )}
    >
      <span className="absolute inset-0 bg-[linear-gradient(145deg,#7ad9ea_0%,#00add8_48%,#00758f_100%)]" />
      <span className="absolute inset-0 bg-[radial-gradient(80%_60%_at_20%_10%,rgba(255,255,255,0.55),transparent_62%)]" />
      <span className="absolute inset-0 rounded-[inherit] ring-1 ring-inset ring-white/30" />
      <span className="relative -translate-y-px tracking-tighter">N</span>
    </span>
  );
}

export function Brand({
  subtitle,
  markSize = 'md',
  className,
}: {
  subtitle?: string;
  markSize?: keyof typeof sizes;
  className?: string;
}) {
  return (
    <span className={cn('flex min-w-0 items-center gap-3', className)}>
      <BrandMark size={markSize} />
      <span className="min-w-0">
        <span translate="no" className="block truncate text-sm font-semibold tracking-tight text-foreground">
          NyxProxy
        </span>
        {subtitle && <span className="block truncate text-[11px] text-muted">{subtitle}</span>}
      </span>
    </span>
  );
}
