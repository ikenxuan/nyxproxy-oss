'use client';

import Link from 'next/link';
import {
  Activity,
  ArrowUpFromLine,
  ArrowUpRight,
  CheckCircle2,
  CircleAlert,
  CircleSlash2,
  Clock3,
  FileText,
  Network,
  RefreshCw,
  Settings,
  Zap,
} from 'lucide-react';
import type { LucideIcon } from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';
import { Badge } from '@/components/ui/Badge';
import { Banner } from '@/components/ui/Banner';
import { Button, buttonClass } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { Skeleton } from '@/components/ui/Skeleton';
import { api } from '@/lib/api';
import { errorMessage, useI18n } from '@/lib/i18n';
import type { Status } from '@/lib/types';
import { formatBytes, formatDate, formatDuration, formatNumber } from '@/lib/format';

const stateDot = {
  running: 'bg-success',
  starting: 'bg-accent',
  error: 'bg-danger',
  stopped: 'bg-faint',
} as const;

const stateRing = {
  running: 'bg-success/40',
  starting: 'bg-accent/40',
  error: 'bg-danger/40',
  stopped: 'bg-faint/30',
} as const;

export default function DashboardPage() {
  const { t, lang } = useI18n();
  const [status, setStatus] = useState<Status | null>(null);
  const [error, setError] = useState('');
  const [retrying, setRetrying] = useState(false);

  const load = useCallback(async () => {
    try {
      setStatus(await api<Status>('/api/status'));
      setError('');
    } catch (err) {
      setError(errorMessage(t, err));
    }
  }, [t]);

  useEffect(() => {
    // Initial load; state updates happen after the async response.
    // oxlint-disable-next-line react/set-state-in-effect
    void load();
    const interval = window.setInterval(() => {
      if (document.visibilityState === 'visible') void load();
    }, 5000);
    const visibility = () => {
      if (document.visibilityState === 'visible') void load();
    };
    document.addEventListener('visibilitychange', visibility);
    return () => {
      window.clearInterval(interval);
      document.removeEventListener('visibilitychange', visibility);
    };
  }, [load]);

  const retry = async () => {
    setRetrying(true);
    try {
      await api('/api/proxy/retry', { method: 'POST' });
      await load();
    } catch (err) {
      setError(errorMessage(t, err));
    } finally {
      setRetrying(false);
    }
  };

  if (!status) {
    return (
      <div className="space-y-4">
        {error && <Banner tone="danger">{error}</Banner>}
        <Skeleton className="h-44 w-full" />
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
          {Array.from({ length: 4 }).map((_, index) => (
            <Skeleton key={index} className="h-28 w-full" />
          ))}
        </div>
        <div className="grid gap-4 lg:grid-cols-2">
          <Skeleton className="h-56 w-full" />
          <Skeleton className="h-56 w-full" />
        </div>
      </div>
    );
  }

  const proxy = status.proxy;
  const stateLabel = t(`dashboard.state.${proxy.state}` as 'dashboard.state.running');
  const monitoringLabel = status.monitoring.enabled
    ? t(`dashboard.state.${status.monitoring.state}` as 'dashboard.state.running')
    : t('common.disabled');
  const applyAt = new Date(status.config.last_apply.at);
  const hasApplyResult = Number.isFinite(applyAt.getTime()) && applyAt.getUTCFullYear() > 1970;
  const applyOk = status.config.last_apply.ok;

  const warningText = (warning: string): string => {
    if (warning === 'the ip command was not found; IPv6 rotation is unavailable') {
      return t('dashboard.ip_missing');
    }
    if (warning.startsWith('last configuration apply failed: ')) {
      return t('config.last_failed', { error: warning.slice('last configuration apply failed: '.length) });
    }
    if (warning.startsWith('proxy is not running: ')) {
      return `${t('dashboard.proxy_not_running')}: ${warning.slice('proxy is not running: '.length)}`;
    }
    return warning;
  };

  return (
    <div className="space-y-5">
      {status.apply.in_progress && (
        <Banner tone="info">
          {t('dashboard.apply_in_progress', { step: status.apply.step || t('config.applying') })}
        </Banner>
      )}
      {status.platform.warnings.map((warning) => (
        <Banner key={warning} tone="warning">
          {warningText(warning)}
        </Banner>
      ))}
      {error && <Banner tone="danger">{error}</Banner>}

      <section className="relative overflow-hidden rounded-card border border-line bg-surface p-6 shadow-card sm:p-8">
        <div className="aurora pointer-events-none absolute inset-0 opacity-80" aria-hidden="true" />
        <div className="relative flex flex-col gap-6 lg:flex-row lg:items-start lg:justify-between">
          <div className="min-w-0">
            <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-faint">
              {t('dashboard.proxy')}
            </p>
            <div className="mt-3 flex items-center gap-3" aria-live="polite">
              <span className="relative flex h-3 w-3" aria-hidden="true">
                <span
                  className={`absolute inline-flex h-full w-full rounded-full opacity-60 ${
                    proxy.state === 'running' || proxy.state === 'starting'
                      ? 'animate-ping motion-reduce:animate-none'
                      : ''
                  } ${stateRing[proxy.state]}`}
                />
                <span className={`relative inline-flex h-3 w-3 rounded-full ${stateDot[proxy.state]}`} />
              </span>
              <h2 className="text-3xl font-semibold leading-none tracking-[-0.02em] text-ink">
                {stateLabel}
              </h2>
            </div>
            <p className="mt-3 flex flex-wrap items-center gap-x-2 gap-y-1 text-sm text-muted">
              <span className="font-semibold uppercase tracking-wide">{proxy.type}</span>
              <span aria-hidden="true" className="text-line-strong">
                /
              </span>
              <span className="break-all font-mono text-[13px] text-ink">{proxy.listen || '—'}</span>
            </p>
            <div className="mt-5 flex flex-wrap items-center gap-2">
              <Badge tone="neutral">IPv{proxy.protocol}</Badge>
              <Badge tone="neutral">
                <Clock3 size={11} aria-hidden="true" />
                {formatDuration(status.uptime_seconds)}
              </Badge>
              <Badge tone={status.monitoring.enabled ? 'info' : 'neutral'}>
                {t('dashboard.monitoring')}: {monitoringLabel}
              </Badge>
            </div>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            {proxy.state === 'error' && (
              <Button onClick={retry} disabled={retrying}>
                <RefreshCw
                  size={16}
                  aria-hidden="true"
                  className={retrying ? 'animate-spin motion-reduce:animate-none' : undefined}
                />
                {retrying ? t('common.loading') : t('common.retry')}
              </Button>
            )}
            <Link href="/config" className={buttonClass({ variant: 'secondary' })}>
              <Settings size={16} aria-hidden="true" />
              {t('dashboard.open_config')}
              <ArrowUpRight size={14} aria-hidden="true" />
            </Link>
          </div>
        </div>
        {proxy.error && (
          <div className="relative mt-5 rounded-field border border-danger/25 bg-danger-soft px-4 py-3 text-sm text-ink">
            <span className="font-semibold text-danger">{t('dashboard.proxy_not_running')}. </span>
            <span className="break-words">{proxy.error}</span>
          </div>
        )}
      </section>

      <section
        aria-label={t('dashboard.traffic')}
        className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4"
      >
        <Metric
          icon={Activity}
          label={t('dashboard.active_connections')}
          value={formatNumber(status.metrics.active_connections, lang)}
        />
        <Metric
          icon={Network}
          label={t('dashboard.total_connections')}
          value={formatNumber(status.metrics.total_connections, lang)}
        />
        <Metric
          icon={Zap}
          label={t('dashboard.requests')}
          value={formatNumber(status.metrics.total_requests, lang)}
        />
        <Metric
          icon={ArrowUpFromLine}
          label={t('dashboard.bytes_sent')}
          value={formatBytes(status.metrics.bytes_sent)}
          hint={`${t('dashboard.bytes_received')}: ${formatBytes(status.metrics.bytes_received)}`}
        />
      </section>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card
          title={t('dashboard.pool')}
          actions={
            <Badge tone={status.pool.enabled ? 'info' : 'neutral'} dot>
              {status.pool.enabled ? t('common.enabled') : t('common.disabled')}
            </Badge>
          }
        >
          {status.pool.enabled ? (
            <div className="space-y-4">
              <div className="flex items-end justify-between gap-6">
                <div>
                  <p className="tabular text-3xl font-semibold tracking-tight text-ink">
                    {formatNumber(status.pool.size, lang)}
                  </p>
                  <p className="mt-1 text-xs text-faint">{t('dashboard.pool_addresses')}</p>
                </div>
                <div className="text-right">
                  <p className="tabular text-2xl font-semibold tracking-tight text-ink">
                    {formatNumber(status.pool.rotations, lang)}
                  </p>
                  <p className="mt-1 text-xs text-faint">{t('dashboard.rotations')}</p>
                </div>
              </div>
              <div className="rounded-field border border-line bg-surface-2/50 px-3.5 py-3">
                <p className="text-[10px] font-semibold uppercase tracking-[0.14em] text-faint">
                  {t('dashboard.prefix')}
                </p>
                <p className="mt-1 break-all font-mono text-[12px] leading-relaxed text-ink">
                  {status.pool.prefix || '—'}
                </p>
              </div>
              <div className="flex flex-wrap gap-2">
                <Badge tone="neutral">
                  {t('dashboard.max_usage')}: {formatNumber(status.pool.max_usage, lang)}
                </Badge>
                <Badge tone="neutral">
                  {t('dashboard.max_age')}: {status.pool.max_age_minutes}m
                </Badge>
              </div>
            </div>
          ) : (
            <div className="rounded-field border border-dashed border-line-strong bg-surface-2/40 p-5 text-center">
              <CircleSlash2 size={22} aria-hidden="true" className="mx-auto text-faint" />
              <p className="mx-auto mt-2 max-w-xs text-pretty text-sm text-muted">
                {t('dashboard.pool_disabled')}
              </p>
            </div>
          )}
        </Card>

        <Card
          title={t('dashboard.config')}
          actions={
            <Badge tone={!hasApplyResult ? 'neutral' : applyOk ? 'success' : 'danger'} dot>
              {!hasApplyResult
                ? '—'
                : applyOk
                  ? t('dashboard.apply_ok')
                  : t('dashboard.apply_failed')}
            </Badge>
          }
        >
          <div className="space-y-4">
            <div className="flex items-end justify-between gap-6">
              <div>
                <p className="tabular text-3xl font-semibold tracking-tight text-ink">
                  {formatNumber(status.config.generation, lang)}
                </p>
                <p className="mt-1 text-xs text-faint">{t('dashboard.generation')}</p>
              </div>
              <div className="min-w-0 text-right">
                <p className="text-sm font-medium text-ink">
                  {formatDate(status.config.last_applied_at, lang)}
                </p>
                <p className="mt-1 text-xs text-faint">{t('dashboard.last_applied')}</p>
              </div>
            </div>
            <div className="rounded-field border border-line bg-surface-2/50 px-3.5 py-3">
              <p className="flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-[0.14em] text-faint">
                <FileText size={11} aria-hidden="true" />
                {t('dashboard.config_path')}
              </p>
              <p className="mt-1 break-all font-mono text-[12px] leading-relaxed text-ink">
                {status.config.path}
              </p>
            </div>
            {hasApplyResult && !applyOk && status.config.last_apply.error && (
              <div className="flex items-start gap-2.5 rounded-field border border-danger/25 bg-danger-soft px-3.5 py-3 text-sm">
                <CircleAlert size={15} aria-hidden="true" className="mt-0.5 shrink-0 text-danger" />
                <p className="min-w-0 break-words text-ink">{status.config.last_apply.error}</p>
              </div>
            )}
            {hasApplyResult && applyOk && (
              <p className="flex items-center gap-2 text-xs text-muted">
                <CheckCircle2 size={14} aria-hidden="true" className="text-success" />
                {t('config.last_ok', { generation: status.config.last_apply.generation })}
              </p>
            )}
          </div>
        </Card>
      </div>
    </div>
  );
}

function Metric({
  icon: Icon,
  label,
  value,
  hint,
}: {
  icon: LucideIcon;
  label: string;
  value: string;
  hint?: string;
}) {
  return (
    <div className="rounded-card border border-line bg-surface p-4 shadow-card transition-colors duration-200 hover:border-line-strong sm:p-5">
      <div className="flex items-center justify-between gap-3">
        <p className="min-w-0 truncate text-[11px] font-semibold uppercase tracking-[0.12em] text-faint">
          {label}
        </p>
        <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-lg bg-accent-soft text-accent">
          <Icon size={15} aria-hidden="true" />
        </span>
      </div>
      <p className="tabular mt-3 text-2xl font-semibold tracking-tight text-ink">{value}</p>
      {hint && <p className="mt-1 truncate text-[11px] text-faint">{hint}</p>}
    </div>
  );
}
