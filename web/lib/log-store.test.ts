import { describe, expect, it } from 'vitest';
import { appendLog, emptyLogState, recordGap } from './log-store';

describe('log store', () => {
  it('keeps at most 2000 lines', () => {
    let state = emptyLogState();
    for (let i = 0; i < 2005; i++) {
      state = appendLog(state, { seq: i + 1, ts: '2026-10-18T00:00:00Z', line: `line-${i}` });
    }
    expect(state.lines).toHaveLength(2000);
    expect(state.lines[0].line).toBe('line-5');
  });

  it('accumulates dropped counts from gap events', () => {
    let state = emptyLogState();
    state = recordGap(state, 3);
    state = recordGap(state, 4);
    expect(state.dropped).toBe(7);
  });

  it('starts with empty lines and no dropped records', () => {
    expect(emptyLogState()).toEqual({ lines: [], dropped: 0 });
  });
});
