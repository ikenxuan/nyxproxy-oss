import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError, api, setCSRFToken } from './api';

interface MockResponse {
  ok: boolean;
  status: number;
  text: () => Promise<string>;
}

function mockFetch(status: number, body: unknown) {
  return vi
    .fn<(input: RequestInfo | URL, init?: RequestInit) => Promise<MockResponse>>()
    .mockResolvedValue({
      ok: status >= 200 && status < 300,
      status,
      text: async () => JSON.stringify(body),
    });
}

afterEach(() => {
  vi.unstubAllGlobals();
  setCSRFToken('');
});

describe('api client', () => {
  it('adds the CSRF header for mutations', async () => {
    const fetchMock = mockFetch(200, { ok: true });
    vi.stubGlobal('fetch', fetchMock);
    setCSRFToken('token-123');

    await api('/api/config', { method: 'PUT', body: '{}' });

    const init = fetchMock.mock.calls[0][1] as RequestInit;
    expect((init.headers as Headers).get('X-CSRF-Token')).toBe('token-123');
    expect((init.headers as Headers).get('Content-Type')).toBe('application/json');
  });

  it('maps the error envelope to ApiError', async () => {
    vi.stubGlobal('fetch', mockFetch(422, {
      error: { code: 'validation_failed', message: 'invalid', fields: { 'proxy.listen_port': 'bad' } },
    }));

    await expect(api('/api/config', { method: 'PUT', body: '{}' })).rejects.toMatchObject({
      code: 'validation_failed',
      fields: { 'proxy.listen_port': 'bad' },
      status: 422,
    });
  });

  it('throws ApiError instances', async () => {
    vi.stubGlobal('fetch', mockFetch(500, { error: { code: 'internal_error', message: 'boom' } }));
    await expect(api('/api/status')).rejects.toBeInstanceOf(ApiError);
  });

  it('suppresses the unauthenticated event only when requested', async () => {
    const events: Event[] = [];
    const listener = (event: Event) => events.push(event);
    window.addEventListener('nyxproxy:unauthenticated', listener);

    try {
      const unauthorized = { error: { code: 'unauthenticated', message: 'expired' } };

      vi.stubGlobal('fetch', mockFetch(401, unauthorized));
      await expect(
        api('/api/auth/session', {}, { suppressUnauthenticatedEvent: true }),
      ).rejects.toMatchObject({ code: 'unauthenticated' });
      expect(events).toHaveLength(0);

      vi.stubGlobal('fetch', mockFetch(401, unauthorized));
      await expect(api('/api/status')).rejects.toMatchObject({ code: 'unauthenticated' });
      expect(events).toHaveLength(1);
    } finally {
      window.removeEventListener('nyxproxy:unauthenticated', listener);
    }
  });
});
