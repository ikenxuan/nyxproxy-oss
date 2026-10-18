import type { LogLine } from './types';

export interface LogState {
  lines: LogLine[];
  dropped: number;
}

export const MAX_LINES = 2000;

export function emptyLogState(): LogState {
  return { lines: [], dropped: 0 };
}

export function appendLog(state: LogState, line: LogLine, max = MAX_LINES): LogState {
  const lines = [...state.lines, line];
  return {
    lines: lines.length > max ? lines.slice(lines.length - max) : lines,
    dropped: state.dropped,
  };
}

export function recordGap(state: LogState, dropped: number): LogState {
  return { ...state, dropped: state.dropped + dropped };
}
