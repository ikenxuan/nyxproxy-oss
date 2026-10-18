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
        'relative inline-flex shrink-0 select-none items-center justify-center overflow-hidden font-bold text-white shadow-[0_2px_10px_-2px_rgba(94,92,230,0.6)]',
        sizes[size],
        className,
      )}
    >
      <span className="absolute inset-0 bg-[linear-gradient(145deg,#7b78ff_0%,#5e5ce6_48%,#3f3dbb_100%)]" />
      <span className="absolute inset-0 bg-[radial-gradient(80%_60%_at_20%_10%,rgba(255,255,255,0.5),transparent_62%)]" />
      <span className="absolute inset-0 rounded-[inherit] ring-1 ring-inset ring-white/25" />
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
        <span
          translate="no"
          className="block truncate text-sm font-semibold tracking-tight text-ink"
        >
          NyxProxy
        </span>
        {subtitle && <span className="block truncate text-[11px] text-faint">{subtitle}</span>}
      </span>
    </span>
  );
}
