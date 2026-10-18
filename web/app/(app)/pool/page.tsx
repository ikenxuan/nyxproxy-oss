'use client';

import { useRouter } from 'next/navigation';
import {
  CircleCheck,
  CircleDashed,
  Gauge,
  Network,
  RefreshCw,
} from 'lucide-react';
import type { LucideIcon } from 'lucide-react';
import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Alert,
  Button,
  Card,
  Chip,
  Label,
  ListBox,
  ProgressBar,
  SearchField,
  Select,
  Skeleton,
  Spinner,
  Switch,
  Table,
} from '@heroui/react';
import { api } from '@/lib/api';
import { formatCompactDate, formatDate, formatDuration, formatNumber, formatTime } from '@/lib/format';
import { errorMessage, useI18n } from '@/lib/i18n';
import type { PoolAddress, PoolAddressState, PoolDetails } from '@/lib/types';

type SortKey =
  | 'address'
  | 'usage'
  | 'remaining_uses'
  | 'remaining_age_seconds'
  | 'last_used'
  | 'added_at'
  | 'state';

interface Sort {
  column: string;
  direction: 'ascending' | 'descending';
}

type ChipColor = 'default' | 'accent' | 'success' | 'warning' | 'danger';

const stateColor: Record<PoolAddressState, ChipColor> = {
  fresh: 'accent',
  active: 'success',
  near_limit: 'warning',
  expired: 'danger',
};

const stateOrder: Record<PoolAddressState, number> = {
  expired: 0,
  near_limit: 1,
  active: 2,
  fresh: 3,
};

const STATES: PoolAddressState[] = ['fresh', 'active', 'near_limit', 'expired'];

function isUnset(value: string): boolean {
  const date = new Date(value);
  return !Number.isFinite(date.getTime()) || date.getUTCFullYear() <= 1970;
}

export default function PoolPage() {
  const { t, lang } = useI18n();
  const router = useRouter();

  const [details, setDetails] = useState<PoolDetails | null>(null);
  const [error, setError] = useState('');
  const [refreshing, setRefreshing] = useState(false);
  const [autoRefresh, setAutoRefresh] = useState(true);

  const [search, setSearch] = useState('');
  const [stateFilter, setStateFilter] = useState<'all' | PoolAddressState>('all');
  const [sort, setSort] = useState<Sort>({ column: 'usage', direction: 'descending' });
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(25);

  const load = useCallback(
    async (silent = false) => {
      if (!silent) setRefreshing(true);
      try {
        setDetails(await api<PoolDetails>('/api/pool'));
        setError('');
      } catch (err) {
        setError(errorMessage(t, err));
      } finally {
        setRefreshing(false);
      }
    },
    [t],
  );

  useEffect(() => {
    // Initial load; state updates happen after the async response.
    // oxlint-disable-next-line react/set-state-in-effect
    void load();
  }, [load]);

  useEffect(() => {
    if (!autoRefresh) return;
    const interval = window.setInterval(() => {
      if (document.visibilityState === 'visible') void load(true);
    }, 5000);
    const visibility = () => {
      if (document.visibilityState === 'visible') void load(true);
    };
    document.addEventListener('visibilitychange', visibility);
    return () => {
      window.clearInterval(interval);
      document.removeEventListener('visibilitychange', visibility);
    };
  }, [autoRefresh, load]);

  const pool = details?.pool;
  const addresses = useMemo(() => details?.addresses ?? [], [details]);

  const counts = useMemo(() => {
    const next: Record<PoolAddressState, number> = { fresh: 0, active: 0, near_limit: 0, expired: 0 };
    for (const address of addresses) next[address.state] += 1;
    return next;
  }, [addresses]);

  const filtered = useMemo(() => {
    const query = search.trim().toLowerCase();
    return addresses.filter((address) => {
      if (stateFilter !== 'all' && address.state !== stateFilter) return false;
      if (query && !address.address.toLowerCase().includes(query)) return false;
      return true;
    });
  }, [addresses, search, stateFilter]);

  const sorted = useMemo(() => {
    const column = sort.column as SortKey;
    const direction = sort.direction === 'ascending' ? 1 : -1;
    return filtered.toSorted((a, b) => {
      switch (column) {
        case 'address':
          return a.address.localeCompare(b.address) * direction;
        case 'state':
          return (stateOrder[a.state] - stateOrder[b.state]) * direction;
        case 'last_used':
        case 'added_at':
          return (new Date(a[column]).getTime() - new Date(b[column]).getTime()) * direction;
        default:
          return (a[column] - b[column]) * direction;
      }
    });
  }, [filtered, sort]);

  const pageCount = Math.max(1, Math.ceil(sorted.length / pageSize));
  const currentPage = Math.min(page, pageCount);
  const start = (currentPage - 1) * pageSize;
  const rows = sorted.slice(start, start + pageSize);

  const resetPage = () => setPage(1);

  if (!details || !pool) {
    return (
      <div className="space-y-4">
        {error && (
          <Alert status="danger">
            <Alert.Indicator />
            <Alert.Content><Alert.Title>{error}</Alert.Title></Alert.Content>
          </Alert>
        )}
        <Skeleton className="h-40 w-full rounded-3xl" />
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
          {Array.from({ length: 4 }).map((_, index) => (
            <Skeleton key={index} className="h-24 w-full rounded-3xl" />
          ))}
        </div>
        <Skeleton className="h-80 w-full rounded-3xl" />
        <div className="flex justify-end">
          <Button variant="secondary" onPress={() => void load()} isDisabled={refreshing}>
            {refreshing ? <Spinner size="sm" color="current" /> : <RefreshCw size={15} aria-hidden="true" />}
            {refreshing ? t('common.loading') : t('common.refresh')}
          </Button>
        </div>
      </div>
    );
  }

  if (!pool.enabled) {
    return (
      <Card className="relative overflow-hidden p-6 sm:p-8">
        <div className="aurora pointer-events-none absolute inset-0 opacity-60" aria-hidden="true" />
        <div className="relative mx-auto flex max-w-md flex-col items-center py-8 text-center">
          <span className="flex h-14 w-14 items-center justify-center rounded-2xl bg-accent-soft text-accent-soft-foreground">
            <Network size={26} aria-hidden="true" />
          </span>
          <h2 className="mt-4 text-xl font-semibold tracking-tight text-foreground">
            {t('pool.disabled_title')}
          </h2>
          <p className="mt-2 text-pretty text-sm leading-relaxed text-muted">{t('pool.disabled_body')}</p>
          <Button className="mt-5" onPress={() => router.push('/config')}>
            {t('pool.open_config')}
          </Button>
        </div>
      </Card>
    );
  }

  const utilization = Math.round(Math.max(0, Math.min(1, pool.utilization)) * 100);
  const utilizationColor = utilization >= 90 ? 'danger' : utilization >= 70 ? 'warning' : 'accent';
  const from = sorted.length === 0 ? 0 : start + 1;
  const to = Math.min(start + pageSize, sorted.length);

  return (
    <div className="space-y-5">
      {error && (
        <Alert status="danger">
          <Alert.Indicator />
          <Alert.Content><Alert.Title>{error}</Alert.Title></Alert.Content>
        </Alert>
      )}

      <Card className="relative overflow-hidden p-6 sm:p-7">
        <div className="aurora pointer-events-none absolute inset-0 opacity-70" aria-hidden="true" />
        <div className="relative flex flex-col gap-5 lg:flex-row lg:items-start lg:justify-between">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <Chip size="sm" variant="soft" color="accent">
                <span className="h-1.5 w-1.5 rounded-full bg-current" aria-hidden="true" />
                <Chip.Label>{t('pool.rotation_enabled')}</Chip.Label>
              </Chip>
              <span className="text-[11px] text-muted" aria-live="polite">
                {t('pool.updated_at', { time: formatTime(details.generated_at, lang) })}
              </span>
            </div>
            <p className="mt-3 break-all font-mono text-[13px] leading-relaxed text-foreground">
              {pool.prefix || '—'}
            </p>
            <div className="mt-4 flex flex-wrap items-center gap-x-4 gap-y-2 text-xs text-muted">
              <span>
                {t('pool.max_usage')}: <span className="tabular font-medium text-foreground">{pool.max_usage}</span>
              </span>
              <span>
                {t('pool.max_age')}:{' '}
                <span className="tabular font-medium text-foreground">{pool.max_age_minutes}m</span>
              </span>
              <span>
                {t('pool.check_interval')}:{' '}
                <span className="tabular font-medium text-foreground">
                  {formatDuration(pool.check_interval_seconds)}
                </span>
              </span>
              <span>
                {t('pool.last_rotated')}:{' '}
                <span className="font-medium text-foreground">
                  {pool.last_rotated_at ? formatDate(pool.last_rotated_at, lang) : t('pool.never')}
                </span>
              </span>
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <Switch size="sm" isSelected={autoRefresh} onChange={setAutoRefresh}>
              <Switch.Content>
                <Switch.Control>
                  <Switch.Thumb />
                </Switch.Control>
                <Label>{t('pool.auto_refresh')}</Label>
              </Switch.Content>
            </Switch>
            <Button
              variant="secondary"
              size="sm"
              onPress={() => void load()}
              isDisabled={refreshing}
            >
              {refreshing ? (
                <Spinner size="sm" color="current" />
              ) : (
                <RefreshCw size={15} aria-hidden="true" />
              )}
              {refreshing ? t('pool.refreshing') : t('common.refresh')}
            </Button>
          </div>
        </div>

        <div className="relative mt-6">
          <div className="mb-1.5 flex items-center justify-between text-xs">
            <span className="font-medium text-muted">{t('pool.utilization')}</span>
            <span className="tabular font-semibold text-foreground">{utilization}%</span>
          </div>
          <ProgressBar
            aria-label={t('pool.utilization')}
            value={utilization}
            color={utilizationColor}
            size="md"
          >
            <ProgressBar.Track>
              <ProgressBar.Fill />
            </ProgressBar.Track>
          </ProgressBar>
        </div>
      </Card>

      <section aria-label={t('pool.title')} className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <Metric icon={Network} label={t('pool.total_addresses')} value={formatNumber(pool.size, lang)} />
        <Metric
          icon={CircleCheck}
          label={t('pool.used_addresses')}
          value={formatNumber(pool.used_addresses, lang)}
          hint={`${t('pool.fresh_addresses')}: ${formatNumber(pool.fresh_addresses, lang)}`}
        />
        <Metric
          icon={RefreshCw}
          label={t('pool.total_rotations')}
          value={formatNumber(pool.rotations, lang)}
          hint={`${t('pool.total_usage')}: ${formatNumber(pool.total_usage, lang)}`}
        />
        <Metric
          icon={Gauge}
          label={t('pool.average_usage')}
          value={pool.average_usage.toFixed(2)}
          hint={`${t('pool.next_check')}: ${
            pool.next_check_at ? formatTime(pool.next_check_at, lang) : '—'
          }`}
        />
      </section>

      <Card>
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <Card.Title>{t('pool.distribution')}</Card.Title>
            <Card.Description>{t('pool.distribution_hint')}</Card.Description>
          </div>
          <Chip size="sm" variant="soft">
            <Chip.Label>{t('pool.count', { count: formatNumber(pool.size, lang) })}</Chip.Label>
          </Chip>
        </div>
        <div className="mt-2 grid gap-4 sm:grid-cols-2">
          {STATES.map((state) => {
            const share = pool.size > 0 ? Math.round((counts[state] / pool.size) * 100) : 0;
            return (
              <div key={state}>
                <div className="mb-1.5 flex items-center justify-between gap-3 text-xs">
                  <Chip size="sm" variant="soft" color={stateColor[state]}>
                    <span className="h-1.5 w-1.5 rounded-full bg-current" aria-hidden="true" />
                    <Chip.Label>{t(`pool.state.${state}` as 'pool.state.fresh')}</Chip.Label>
                  </Chip>
                  <span className="tabular text-muted">
                    {formatNumber(counts[state], lang)} · {share}%
                  </span>
                </div>
                <ProgressBar
                  aria-label={t(`pool.state.${state}` as 'pool.state.fresh')}
                  value={share}
                  color={stateColor[state]}
                  size="sm"
                >
                  <ProgressBar.Track>
                    <ProgressBar.Fill />
                  </ProgressBar.Track>
                </ProgressBar>
              </div>
            );
          })}
        </div>
      </Card>

      <Card className="p-0">
        <div className="flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between sm:p-5">
          <div className="min-w-0">
            <Card.Title>{t('pool.addresses')}</Card.Title>
            <Card.Description>{t('pool.count', { count: formatNumber(sorted.length, lang) })}</Card.Description>
          </div>
          <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
            <SearchField
              value={search}
              onChange={(value) => {
                setSearch(value);
                resetPage();
              }}
              aria-label={t('pool.search')}
              className="w-full sm:w-64"
            >
              <SearchField.Group>
                <SearchField.SearchIcon />
                <SearchField.Input placeholder={t('pool.search')} />
                <SearchField.ClearButton aria-label={t('common.clear')} />
              </SearchField.Group>
            </SearchField>
            <Select
              selectedKey={stateFilter}
              onSelectionChange={(key) => {
                const next = String(key) as 'all' | PoolAddressState;
                setStateFilter(next);
                resetPage();
              }}
              aria-label={t('pool.filter_state')}
              className="w-full sm:w-44"
            >
              <Select.Trigger>
                <Select.Value />
                <Select.Indicator />
              </Select.Trigger>
              <Select.Popover>
                <ListBox>
                  <ListBox.Item id="all" textValue={t('pool.all_states')}>
                    {t('pool.all_states')}
                    <ListBox.ItemIndicator />
                  </ListBox.Item>
                  {STATES.map((state) => (
                    <ListBox.Item
                      key={state}
                      id={state}
                      textValue={t(`pool.state.${state}` as 'pool.state.fresh')}
                    >
                      {t(`pool.state.${state}` as 'pool.state.fresh')}
                      <ListBox.ItemIndicator />
                    </ListBox.Item>
                  ))}
                </ListBox>
              </Select.Popover>
            </Select>
          </div>
        </div>

        <Table>
          <Table.ScrollContainer>
            <Table.Content
              aria-label={t('pool.addresses')}
              className="min-w-[680px]"
              sortDescriptor={sort}
              onSortChange={(descriptor) => {
                setSort({ column: String(descriptor.column), direction: descriptor.direction });
                resetPage();
              }}
            >
              <Table.Header>
                <Table.Column id="address" allowsSorting isRowHeader>
                  {({ sortDirection }) => (
                    <Table.SortableColumnHeader sortDirection={sortDirection}>
                      {t('pool.address')}
                    </Table.SortableColumnHeader>
                  )}
                </Table.Column>
                <Table.Column id="usage" allowsSorting>
                  {({ sortDirection }) => (
                    <Table.SortableColumnHeader sortDirection={sortDirection}>
                      {t('pool.usage')}
                    </Table.SortableColumnHeader>
                  )}
                </Table.Column>
                <Table.Column id="remaining_uses" allowsSorting>
                  {({ sortDirection }) => (
                    <Table.SortableColumnHeader sortDirection={sortDirection}>
                      {t('pool.remaining_uses')}
                    </Table.SortableColumnHeader>
                  )}
                </Table.Column>
                <Table.Column id="remaining_age_seconds" allowsSorting>
                  {({ sortDirection }) => (
                    <Table.SortableColumnHeader sortDirection={sortDirection}>
                      {t('pool.remaining_age')}
                    </Table.SortableColumnHeader>
                  )}
                </Table.Column>
                <Table.Column id="last_used" allowsSorting>
                  {({ sortDirection }) => (
                    <Table.SortableColumnHeader sortDirection={sortDirection}>
                      {t('pool.last_used')}
                    </Table.SortableColumnHeader>
                  )}
                </Table.Column>
                <Table.Column id="state" allowsSorting>
                  {({ sortDirection }) => (
                    <Table.SortableColumnHeader sortDirection={sortDirection}>
                      {t('pool.state')}
                    </Table.SortableColumnHeader>
                  )}
                </Table.Column>
              </Table.Header>
              <Table.Body
                items={rows}
                renderEmptyState={() => (
                  <div className="flex flex-col items-center gap-2 py-12 text-center text-muted">
                    <CircleDashed size={22} aria-hidden="true" />
                    <p className="text-[13px]">{search || stateFilter !== 'all' ? t('pool.no_results') : t('pool.empty')}</p>
                  </div>
                )}
              >
                {(item: PoolAddress) => (
                  <Table.Row id={item.address}>
                    <Table.Cell>
                      <span className="break-all font-mono text-[12px] text-foreground">{item.address}</span>
                    </Table.Cell>
                    <Table.Cell>
                      <span className="tabular text-foreground">
                        <span className="font-semibold">{item.usage}</span>
                        <span className="text-muted"> / {pool.max_usage}</span>
                      </span>
                    </Table.Cell>
                    <Table.Cell>
                      <span className="tabular text-foreground">{item.remaining_uses}</span>
                    </Table.Cell>
                    <Table.Cell>
                      <span className="tabular text-foreground">
                        {item.remaining_age_seconds > 0
                          ? formatDuration(item.remaining_age_seconds)
                          : '0s'}
                      </span>
                    </Table.Cell>
                    <Table.Cell>
                      <span className="whitespace-nowrap text-[12px] text-muted">
                        {isUnset(item.last_used) ? t('pool.never') : formatCompactDate(item.last_used, lang)}
                      </span>
                    </Table.Cell>
                    <Table.Cell>
                      <Chip size="sm" variant="soft" color={stateColor[item.state]}>
                        <span className="h-1.5 w-1.5 rounded-full bg-current" aria-hidden="true" />
                        <Chip.Label>{t(`pool.state.${item.state}` as 'pool.state.fresh')}</Chip.Label>
                      </Chip>
                    </Table.Cell>
                  </Table.Row>
                )}
              </Table.Body>
            </Table.Content>
          </Table.ScrollContainer>

          <div className="flex flex-col gap-3 border-t border-border p-4 sm:flex-row sm:items-center sm:justify-between sm:p-5">
            <p className="tabular text-xs text-muted">
              {t('pool.page_range', {
                from: formatNumber(from, lang),
                to: formatNumber(to, lang),
                total: formatNumber(sorted.length, lang),
              })}
            </p>
            <div className="flex items-center gap-2">
              <Select
                selectedKey={String(pageSize)}
                onSelectionChange={(key) => {
                  setPageSize(Number(key));
                  resetPage();
                }}
                aria-label={t('pool.per_page')}
                className="w-32"
              >
                <Select.Trigger>
                  <Select.Value />
                  <Select.Indicator />
                </Select.Trigger>
                <Select.Popover>
                  <ListBox>
                    {[10, 25, 50, 100].map((size) => (
                      <ListBox.Item key={size} id={String(size)} textValue={`${size}`}>
                        {size}
                        <ListBox.ItemIndicator />
                      </ListBox.Item>
                    ))}
                  </ListBox>
                </Select.Popover>
              </Select>
              <Button
                variant="secondary"
                size="sm"
                isDisabled={currentPage <= 1}
                onPress={() => setPage((value) => Math.max(1, value - 1))}
              >
                {t('pool.previous')}
              </Button>
              <span className="tabular px-1 text-xs text-muted">
                {currentPage} / {pageCount}
              </span>
              <Button
                variant="secondary"
                size="sm"
                isDisabled={currentPage >= pageCount}
                onPress={() => setPage((value) => Math.min(pageCount, value + 1))}
              >
                {t('pool.next')}
              </Button>
            </div>
          </div>
        </Table>
      </Card>
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
    <Card className="p-4">
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
