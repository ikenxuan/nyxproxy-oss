'use client';

import { useRouter } from 'next/navigation';
import {
  Activity,
  ArrowUpFromLine,
  ArrowUpRight,
  CheckCircle2,
  CircleAlert,
  Clock3,
  Copy,
  FileText,
  Network,
  RefreshCw,
  Settings,
  Zap,
} from 'lucide-react';
import type { LucideIcon } from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';
import {
  Alert,
  Button,
  Card,
  Chip,
  ProgressBar,
  Skeleton,
  Spinner,
  Tooltip,
  toast,
} from '@heroui/react';
import { api } from '@/lib/api';
import { formatBytes, formatDate, formatDuration, formatNumber } from '@/lib/format';
import { errorMessage, useI18n } from '@/lib/i18n';
import type { PoolStatus, Status } from '@/lib/types';

const stateDot: Record<Status['proxy']['state'], string> = {
  running: 'bg-success',
  starting: 'bg-accent',
  error: 'bg-danger',
  stopped: 'bg-muted',
};

export default function DashboardPage() {
  const { t, lang } = useI18n();
  const router = useRouter();
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

  const copyAddress = async (value: string) => {
    try {
      await navigator.clipboard.writeText(value);
      toast.success(t('common.copied'));
    } catch {
      toast.danger(t('common.copy_failed'));
    }
  };

  if (!status) {
    return (
      <div className="space-y-4">
        {error && <Alert status="danger"><Alert.Indicator /><Alert.Content><Alert.Title>{error}</Alert.Title></Alert.Content></Alert>}
        <Skeleton className="h-48 w-full rounded-3xl" />
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
          {Array.from({ length: 4 }).map((_, index) => (
            <Skeleton key={index} className="h-28 w-full rounded-3xl" />
          ))}
        </div>
        <div className="grid gap-4 lg:grid-cols-2">
          <Skeleton className="h-64 w-full rounded-3xl" />
          <Skeleton className="h-64 w-full rounded-3xl" />
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
        <Alert status="accent">
          <Alert.Indicator><Spinner size="sm" /></Alert.Indicator>
          <Alert.Content>
            <Alert.Title>{t('dashboard.apply_in_progress', { step: status.apply.step || t('config.applying') })}</Alert.Title>
          </Alert.Content>
        </Alert>
      )}
      {status.platform.warnings.map((warning) => (
        <Alert key={warning} status="warning">
          <Alert.Indicator />
          <Alert.Content><Alert.Title>{warningText(warning)}</Alert.Title></Alert.Content>
        </Alert>
      ))}
      {error && (
        <Alert status="danger">
          <Alert.Indicator />
          <Alert.Content><Alert.Title>{error}</Alert.Title></Alert.Content>
        </Alert>
      )}

      <Card className="relative overflow-hidden p-6 sm:p-8">
        <div className="aurora pointer-events-none absolute inset-0 opacity-80" aria-hidden="true" />
        <div className="relative flex flex-col gap-6 lg:flex-row lg:items-start lg:justify-between">
          <div className="min-w-0">
            <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-muted">
              {t('dashboard.proxy')}
            </p>
            <div className="mt-3 flex items-center gap-3" aria-live="polite">
              <span
                className={`h-2.5 w-2.5 shrink-0 rounded-full ${stateDot[proxy.state]} ${
                  proxy.state === 'running' || proxy.state === 'starting'
                    ? 'animate-pulse motion-reduce:animate-none'
                    : ''
                }`}
                aria-hidden="true"
              />
              <h2 className="text-3xl font-semibold leading-none tracking-[-0.02em] text-foreground">
                {stateLabel}
              </h2>
            </div>
            <div className="mt-4">
              <p className="text-[10px] font-semibold uppercase tracking-[0.14em] text-muted">
                {t('dashboard.proxy_address')}
              </p>
              <div className="mt-1.5 flex flex-wrap items-center gap-2">
                <span className="break-all rounded-xl border border-border bg-surface-secondary/60 px-3 py-1.5 font-mono text-[13px] leading-relaxed text-foreground">
                  {proxy.listen || '—'}
                </span>
                {proxy.listen && (
                  <Tooltip>
                    <Button
                      variant="ghost"
                      size="sm"
                      isIconOnly
                      aria-label={t('dashboard.copy_address')}
                      onPress={() => void copyAddress(proxy.listen)}
                    >
                      <Copy size={15} aria-hidden="true" />
                    </Button>
                    <Tooltip.Content>{t('dashboard.copy_address')}</Tooltip.Content>
                  </Tooltip>
                )}
                <span className="text-[11px] font-semibold uppercase tracking-wide text-muted">
                  {proxy.type}
                </span>
              </div>
            </div>
            <div className="mt-5 flex flex-wrap items-center gap-2">
              <Chip size="sm" variant="soft">
                <Chip.Label>IPv{proxy.protocol}</Chip.Label>
              </Chip>
              <Chip size="sm" variant="soft">
                <Clock3 size={11} aria-hidden="true" />
                <Chip.Label>{formatDuration(status.uptime_seconds)}</Chip.Label>
              </Chip>
              <Chip size="sm" variant="soft" color={status.monitoring.enabled ? 'accent' : 'default'}>
                <Chip.Label>
                  {t('dashboard.monitoring')}: {monitoringLabel}
                </Chip.Label>
              </Chip>
            </div>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            {proxy.state === 'error' && (
              <Button onPress={retry} isDisabled={retrying}>
                {retrying ? (
                  <Spinner size="sm" color="current" />
                ) : (
                  <RefreshCw size={16} aria-hidden="true" />
                )}
                {retrying ? t('common.loading') : t('common.retry')}
              </Button>
            )}
            <Button variant="secondary" onPress={() => router.push('/config')}>
              <Settings size={16} aria-hidden="true" />
              {t('dashboard.open_config')}
              <ArrowUpRight size={14} aria-hidden="true" />
            </Button>
          </div>
        </div>
        {proxy.error && (
          <div className="relative mt-5 rounded-2xl border border-danger/25 bg-danger-soft px-4 py-3 text-sm text-foreground">
            <span className="font-semibold text-danger">{t('dashboard.proxy_not_running')}. </span>
            <span className="break-words">{proxy.error}</span>
          </div>
        )}
      </Card>

      <section aria-label={t('dashboard.traffic')} className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
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
        <PoolCard pool={status.pool} lang={lang} onOpen={() => router.push('/pool')} />
        <ConfigCard status={status} lang={lang} applyOk={applyOk} hasApplyResult={hasApplyResult} />
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
    <Card className="p-4 transition-colors duration-200 hover:border-border sm:p-5">
      <div className="flex items-center justify-between gap-3">
        <p className="min-w-0 truncate text-[11px] font-semibold uppercase tracking-[0.12em] text-muted">
          {label}
        </p>
        <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-xl bg-accent-soft text-accent-soft-foreground">
          <Icon size={15} aria-hidden="true" />
        </span>
      </div>
      <p className="tabular mt-3 text-2xl font-semibold tracking-tight text-foreground">{value}</p>
      {hint && <p className="mt-1 truncate text-[11px] text-muted">{hint}</p>}
    </Card>
  );
}

function PoolCard({ pool, lang, onOpen }: { pool: PoolStatus; lang: string; onOpen: () => void }) {
  const { t } = useI18n();
  const utilization = Math.round(Math.max(0, Math.min(1, pool.utilization)) * 100);
  const color = utilization >= 90 ? 'danger' : utilization >= 70 ? 'warning' : 'accent';

  return (
    <Card>
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <Card.Title>{t('dashboard.pool')}</Card.Title>
          <Card.Description>{t('pool.distribution_hint')}</Card.Description>
        </div>
        <Chip size="sm" variant="soft" color={pool.enabled ? 'accent' : 'default'}>
          <span className="h-1.5 w-1.5 rounded-full bg-current" aria-hidden="true" />
          <Chip.Label>{pool.enabled ? t('common.enabled') : t('common.disabled')}</Chip.Label>
        </Chip>
      </div>

      {pool.enabled ? (
        <div className="mt-2 space-y-4">
          <div className="grid grid-cols-3 gap-3">
            <Stat label={t('pool.total_addresses')} value={formatNumber(pool.size, lang)} />
            <Stat label={t('dashboard.pool_used')} value={formatNumber(pool.used_addresses, lang)} />
            <Stat label={t('pool.total_rotations')} value={formatNumber(pool.rotations, lang)} />
          </div>

          <div>
            <div className="mb-1.5 flex items-center justify-between text-xs">
              <span className="font-medium text-muted">{t('dashboard.pool_utilization')}</span>
              <span className="tabular font-semibold text-foreground">{utilization}%</span>
            </div>
            <ProgressBar aria-label={t('dashboard.pool_utilization')} value={utilization} color={color} size="sm">
              <ProgressBar.Track>
                <ProgressBar.Fill />
              </ProgressBar.Track>
            </ProgressBar>
          </div>

          <div className="rounded-2xl border border-border bg-surface-secondary/60 px-3.5 py-3">
            <p className="text-[10px] font-semibold uppercase tracking-[0.14em] text-muted">
              {t('pool.prefix')}
            </p>
            <p className="mt-1 break-all font-mono text-[12px] leading-relaxed text-foreground">
              {pool.prefix || '—'}
            </p>
          </div>

          <Button variant="secondary" size="sm" fullWidth onPress={onOpen}>
            <Network size={15} aria-hidden="true" />
            {t('dashboard.view_pool')}
            <ArrowUpRight size={13} aria-hidden="true" />
          </Button>
        </div>
      ) : (
        <div className="mt-2 rounded-2xl border border-dashed border-border bg-surface-secondary/40 p-5 text-center">
          <Network size={22} aria-hidden="true" className="mx-auto text-muted" />
          <p className="mx-auto mt-2 max-w-xs text-pretty text-sm text-muted">
            {t('dashboard.pool_disabled')}
          </p>
        </div>
      )}
    </Card>
  );
}

function ConfigCard({
  status,
  lang,
  applyOk,
  hasApplyResult,
}: {
  status: Status;
  lang: string;
  applyOk: boolean;
  hasApplyResult: boolean;
}) {
  const { t } = useI18n();
  return (
    <Card>
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <Card.Title>{t('dashboard.config')}</Card.Title>
          <Card.Description>{t('page.config.subtitle')}</Card.Description>
        </div>
        <Chip
          size="sm"
          variant="soft"
          color={!hasApplyResult ? 'default' : applyOk ? 'success' : 'danger'}
        >
          <span className="h-1.5 w-1.5 rounded-full bg-current" aria-hidden="true" />
          <Chip.Label>
            {!hasApplyResult ? '—' : applyOk ? t('dashboard.apply_ok') : t('dashboard.apply_failed')}
          </Chip.Label>
        </Chip>
      </div>

      <div className="mt-2 space-y-4">
        <div className="flex items-end justify-between gap-6">
          <div>
            <p className="tabular text-3xl font-semibold tracking-tight text-foreground">
              {formatNumber(status.config.generation, lang)}
            </p>
            <p className="mt-1 text-xs text-muted">{t('dashboard.generation')}</p>
          </div>
          <div className="min-w-0 text-right">
            <p className="text-sm font-medium text-foreground">
              {formatDate(status.config.last_applied_at, lang)}
            </p>
            <p className="mt-1 text-xs text-muted">{t('dashboard.last_applied')}</p>
          </div>
        </div>

        <div className="rounded-2xl border border-border bg-surface-secondary/60 px-3.5 py-3">
          <p className="flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-[0.14em] text-muted">
            <FileText size={11} aria-hidden="true" />
            {t('dashboard.config_path')}
          </p>
          <p className="mt-1 break-all font-mono text-[12px] leading-relaxed text-foreground">
            {status.config.path}
          </p>
        </div>

        {hasApplyResult && !applyOk && status.config.last_apply.error && (
          <div className="flex items-start gap-2.5 rounded-2xl border border-danger/25 bg-danger-soft px-3.5 py-3 text-sm">
            <CircleAlert size={15} aria-hidden="true" className="mt-0.5 shrink-0 text-danger" />
            <p className="min-w-0 break-words text-foreground">{status.config.last_apply.error}</p>
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
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-2xl border border-border bg-surface-secondary/60 px-3 py-2.5">
      <p className="tabular text-lg font-semibold tracking-tight text-foreground">{value}</p>
      <p className="mt-0.5 truncate text-[10px] text-muted">{label}</p>
    </div>
  );
}
