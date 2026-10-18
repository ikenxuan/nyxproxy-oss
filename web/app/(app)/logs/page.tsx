'use client';

import { ArrowDownToLine, Pause, Play, ScrollText, Trash2 } from 'lucide-react';
import { useRouter } from 'next/navigation';
import { useEffect, useRef, useState } from 'react';
import {
  Alert,
  Button,
  Card,
  Chip,
  SearchField,
  Tooltip,
} from '@heroui/react';
import { api, ApiError } from '@/lib/api';
import { cn } from '@/lib/cn';
import { formatNumber, formatTime } from '@/lib/format';
import { useI18n } from '@/lib/i18n';
import { appendLog, emptyLogState, recordGap, type LogState } from '@/lib/log-store';
import type { LogLine } from '@/lib/types';

function lineTone(line: string): string {
  if (/\b(error|fatal|panic|failed|failure)\b/i.test(line)) return 'text-red-300';
  if (/\b(warn|warning)\b/i.test(line)) return 'text-amber-300';
  if (/\b(debug|trace)\b/i.test(line)) return 'text-zinc-400';
  return 'text-zinc-200';
}

// The Go logger prefixes every line with its own date and time; the console
// already renders a timestamp, so avoid showing it twice.
const LOG_TIMESTAMP = /^\d{4}\/\d{2}\/\d{2} \d{2}:\d{2}:\d{2}\s+/;

function displayLine(line: string): string {
  return line.replace(LOG_TIMESTAMP, '');
}

export default function LogsPage() {
  const { t, lang } = useI18n();
  const router = useRouter();
  const [state, setState] = useState<LogState>(emptyLogState());
  const [paused, setPaused] = useState(false);
  const [connected, setConnected] = useState(false);
  const [filter, setFilter] = useState('');
  const [newCount, setNewCount] = useState(0);
  const pausedRef = useRef(paused);
  const bottomRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    pausedRef.current = paused;
  }, [paused]);

  useEffect(() => {
    const source = new EventSource('/api/logs/stream');
    source.addEventListener('open', () => setConnected(true));
    source.addEventListener('log', (event) => {
      let record: LogLine;
      try {
        record = JSON.parse((event as MessageEvent).data) as LogLine;
      } catch {
        return;
      }
      setState((current) => appendLog(current, record));
      if (pausedRef.current) setNewCount((count) => count + 1);
    });
    source.addEventListener('gap', (event) => {
      let data: { dropped: number };
      try {
        data = JSON.parse((event as MessageEvent).data) as { dropped: number };
      } catch {
        return;
      }
      setState((current) => recordGap(current, data.dropped));
    });
    source.addEventListener('error', () => {
      setConnected(false);
      void api('/api/auth/session', {}, { suppressUnauthenticatedEvent: true }).catch((err) => {
        if (err instanceof ApiError && err.status === 401) {
          source.close();
          router.replace('/login');
        }
      });
    });
    return () => source.close();
  }, [router]);

  const latestSeq = state.lines.at(-1)?.seq;

  useEffect(() => {
    if (paused || filter || latestSeq === undefined) return;
    bottomRef.current?.scrollIntoView({ block: 'end' });
  }, [latestSeq, paused, filter]);

  const togglePause = () => {
    if (paused) setNewCount(0);
    setPaused(!paused);
  };

  const visible = filter
    ? state.lines.filter((line) => line.line.toLowerCase().includes(filter.toLowerCase()))
    : state.lines;

  const jumpToLatest = () => {
    setPaused(false);
    setNewCount(0);
    window.requestAnimationFrame(() => bottomRef.current?.scrollIntoView({ block: 'end' }));
  };

  return (
    <div className="space-y-4">
      <Card className="overflow-hidden p-0">
        <div className="flex flex-wrap items-center gap-2 border-b border-border px-4 py-3 sm:px-5">
          <Chip size="sm" variant="soft" color={connected ? 'success' : 'danger'}>
            <span className="h-1.5 w-1.5 rounded-full bg-current" aria-hidden="true" />
            <Chip.Label>{connected ? t('logs.connected') : t('logs.disconnected')}</Chip.Label>
          </Chip>
          <span className="tabular text-[11px] text-muted">
            {t('logs.lines', { count: formatNumber(visible.length, lang) })}
          </span>
          <div className="ml-auto flex flex-wrap items-center gap-2">
            <SearchField
              value={filter}
              onChange={setFilter}
              aria-label={t('logs.filter')}
              className="w-40 sm:w-56"
            >
              <SearchField.Group>
                <SearchField.SearchIcon />
                <SearchField.Input placeholder={t('logs.filter')} />
                <SearchField.ClearButton aria-label={t('common.clear')} />
              </SearchField.Group>
            </SearchField>
            <Button
              variant={paused ? 'secondary' : 'ghost'}
              size="sm"
              onPress={togglePause}
              aria-pressed={paused}
            >
              {paused ? <Play size={14} aria-hidden="true" /> : <Pause size={14} aria-hidden="true" />}
              {paused ? t('logs.resume') : t('logs.pause')}
            </Button>
            <Tooltip>
              <Button
                variant="ghost"
                size="sm"
                isIconOnly
                aria-label={t('logs.clear')}
                onPress={() => setState(emptyLogState())}
              >
                <Trash2 size={15} aria-hidden="true" />
              </Button>
              <Tooltip.Content>{t('logs.clear')}</Tooltip.Content>
            </Tooltip>
          </div>
        </div>

        {state.dropped > 0 && (
          <div className="border-b border-border p-3">
            <Alert status="warning">
              <Alert.Indicator />
              <Alert.Content>
                <Alert.Title>{t('logs.gap', { count: state.dropped })}</Alert.Title>
              </Alert.Content>
            </Alert>
          </div>
        )}

        <div className="relative">
          <div className="terminal scrollbar-thin h-[calc(100dvh-19rem)] min-h-[20rem] overflow-auto px-3 py-3 font-mono text-[12px] leading-5 sm:px-4">
            {visible.length === 0 ? (
              <div className="flex h-full flex-col items-center justify-center gap-2 text-center text-zinc-500">
                <ScrollText size={22} aria-hidden="true" />
                <p className="text-[12px]">{t('logs.empty')}</p>
              </div>
            ) : (
              visible.map((line) => (
                <div
                  key={`${line.ts}-${line.seq}`}
                  className="flex gap-3 rounded px-2 py-0.5 transition-colors hover:bg-white/[0.04] [contain-intrinsic-size:auto_1.35rem] [content-visibility:auto]"
                >
                  <span className="tabular shrink-0 select-none text-[11px] text-zinc-500">
                    {formatTime(line.ts, lang)}
                  </span>
                  <span className={cn('min-w-0 whitespace-pre-wrap break-words', lineTone(line.line))}>
                    {displayLine(line.line)}
                  </span>
                </div>
              ))
            )}
            <div ref={bottomRef} />
          </div>

          {paused && newCount > 0 && (
            <div className="pointer-events-none absolute inset-x-0 bottom-4 flex justify-center">
              <Button size="sm" className="pointer-events-auto shadow-overlay" onPress={jumpToLatest}>
                <ArrowDownToLine size={14} aria-hidden="true" />
                {t('logs.jump_latest')}
                <span className="tabular rounded-full bg-accent-foreground/20 px-1.5 py-0.5 text-[10px]">
                  {newCount}
                </span>
              </Button>
            </div>
          )}
        </div>
      </Card>
    </div>
  );
}
