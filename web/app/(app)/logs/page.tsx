'use client';

import { ArrowDownToLine, Pause, Play, ScrollText, Search, Trash2 } from 'lucide-react';
import { useRouter } from 'next/navigation';
import { useEffect, useRef, useState } from 'react';
import { Badge } from '@/components/ui/Badge';
import { Banner } from '@/components/ui/Banner';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
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
      <section className="overflow-hidden rounded-card border border-line bg-surface shadow-card">
        <div className="flex flex-wrap items-center gap-2 border-b border-line px-4 py-3 sm:px-5">
          <Badge tone={connected ? 'success' : 'danger'} dot>
            {connected ? t('logs.connected') : t('logs.disconnected')}
          </Badge>
          <span className="tabular text-[11px] text-faint">
            {t('logs.lines', { count: formatNumber(visible.length, lang) })}
          </span>
          <div className="ml-auto flex flex-wrap items-center gap-2">
            <div className="relative">
              <Search
                size={14}
                aria-hidden="true"
                className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-faint"
              />
              <Input
                className="h-8 w-40 pl-8 text-[13px] sm:w-56"
                name="log-filter"
                autoComplete="off"
                spellCheck={false}
                placeholder={t('logs.filter')}
                aria-label={t('logs.filter')}
                value={filter}
                onChange={(event) => setFilter(event.target.value)}
              />
            </div>
            <Button
              variant={paused ? 'secondary' : 'ghost'}
              size="sm"
              onClick={togglePause}
              aria-pressed={paused}            >
              {paused ? <Play size={14} aria-hidden="true" /> : <Pause size={14} aria-hidden="true" />}
              {paused ? t('logs.resume') : t('logs.pause')}
            </Button>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setState(emptyLogState())}
              aria-label={t('logs.clear')}
            >
              <Trash2 size={14} aria-hidden="true" />
              <span className="hidden sm:inline">{t('logs.clear')}</span>
            </Button>
          </div>
        </div>

        {state.dropped > 0 && (
          <div className="border-b border-line p-3">
            <Banner tone="warning">{t('logs.gap', { count: state.dropped })}</Banner>
          </div>
        )}

        <div className="relative">
          <div className="scrollbar-thin h-[calc(100dvh-19rem)] min-h-[20rem] overflow-auto bg-[#0b0b0d] px-3 py-3 font-mono text-[12px] leading-5 sm:px-4">
            {visible.length === 0 ? (
              <div className="flex h-full flex-col items-center justify-center gap-2 text-center text-zinc-500">
                <ScrollText size={22} aria-hidden="true" />
                <p className="text-[12px]">{t('logs.empty')}</p>
              </div>
            ) : (
              visible.map((line) => (
                <div
                  key={`${line.ts}-${line.seq}`}
                  className="flex gap-3 rounded px-2 py-0.5 transition-colors hover:bg-white/[0.03] [contain-intrinsic-size:auto_1.35rem] [content-visibility:auto]"
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
              <Button
                size="sm"
                className="pointer-events-auto shadow-float"
                onClick={jumpToLatest}
              >
                <ArrowDownToLine size={14} aria-hidden="true" />
                {t('logs.jump_latest')}
                <span className="tabular rounded-full bg-white/20 px-1.5 py-0.5 text-[10px]">
                  {newCount}
                </span>
              </Button>
            </div>
          )}
        </div>
      </section>
    </div>
  );
}
