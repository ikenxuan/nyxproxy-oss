import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { StrictMode } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { api, setCSRFToken } from './api';
import { AuthProvider, useAuth } from './auth';

interface MockResponse {
  ok: boolean;
  status: number;
  text: () => Promise<string>;
}

interface PendingRequest {
  url: string;
  resolve: (response: MockResponse) => void;
}

function deferredFetch() {
  const pending: PendingRequest[] = [];
  const fetchMock = vi.fn<(input: RequestInfo | URL, init?: RequestInit) => Promise<MockResponse>>(
    (input, init) => {
      void init;
      return new Promise<MockResponse>((resolve) => {
        pending.push({ url: String(input), resolve });
      });
    },
  );
  return { fetchMock, pending };
}

function jsonResponse(status: number, body: unknown): MockResponse {
  return {
    ok: status >= 200 && status < 300,
    status,
    text: async () => JSON.stringify(body),
  };
}

function Probe() {
  const { username, loading, login } = useAuth();
  return (
    <>
      <span data-testid="username">{username ?? 'anonymous'}</span>
      <span data-testid="loading">{loading ? 'yes' : 'no'}</span>
      <button type="button" onClick={() => void login('admin', 'secret')}>
        login
      </button>
    </>
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
  setCSRFToken('');
});

describe('AuthProvider race handling', () => {
  it('keeps a successful login when multiple bootstrap session 401s resolve later', async () => {
    const { fetchMock, pending } = deferredFetch();
    vi.stubGlobal('fetch', fetchMock);
    const user = userEvent.setup();

    render(
      <StrictMode>
        <AuthProvider>
          <Probe />
        </AuthProvider>
      </StrictMode>,
    );

    const sessionRequests = () => pending.filter((request) => request.url === '/api/auth/session');
    const loginRequests = () => pending.filter((request) => request.url === '/api/auth/login');

    // StrictMode runs the mount effect twice, so both bootstrap refreshes are
    // in flight with the same generation.
    await waitFor(() => expect(sessionRequests()).toHaveLength(2));

    await user.click(screen.getByRole('button', { name: 'login' }));
    await waitFor(() => expect(loginRequests()).toHaveLength(1));

    await act(async () => {
      loginRequests()[0].resolve(
        jsonResponse(200, {
          username: 'admin',
          csrf_token: 'csrf-1',
          expires_at: '2026-10-19T00:00:00Z',
        }),
      );
    });
    await waitFor(() => expect(screen.getByTestId('username')).toHaveTextContent('admin'));

    const [firstSession, secondSession] = sessionRequests();
    await act(async () => {
      firstSession.resolve(
        jsonResponse(401, { error: { code: 'unauthenticated', message: 'expired' } }),
      );
    });
    await act(async () => {
      secondSession.resolve(
        jsonResponse(401, { error: { code: 'unauthenticated', message: 'expired' } }),
      );
    });

    expect(screen.getByTestId('username')).toHaveTextContent('admin');
    expect(screen.getByTestId('loading')).toHaveTextContent('no');

    // The CSRF token must survive the stale 401s: a mutation issued afterwards
    // must still carry the login token.
    void api('/api/config', { method: 'PUT', body: '{}' });
    const init = fetchMock.mock.calls.at(-1)?.[1] as RequestInit;
    expect(new Headers(init.headers).get('X-CSRF-Token')).toBe('csrf-1');
  });
});
