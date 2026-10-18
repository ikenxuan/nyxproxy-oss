let csrfToken = '';

export function setCSRFToken(token: string): void {
  csrfToken = token;
}

interface ErrorEnvelope {
  error?: {
    code?: string;
    message?: string;
    fields?: Record<string, string>;
    rolled_back?: boolean;
  };
}

export interface ApiOptions {
  // Session probes handle their own 401s; suppressing the global event keeps a
  // stale probe from clearing a newer session.
  suppressUnauthenticatedEvent?: boolean;
}

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly fields?: Record<string, string>;
  readonly rolledBack?: boolean;

  constructor(message: string, status: number, code: string, fields?: Record<string, string>, rolledBack?: boolean) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
    this.fields = fields;
    this.rolledBack = rolledBack;
  }
}

export async function api<T>(path: string, init: RequestInit = {}, options: ApiOptions = {}): Promise<T> {
  const method = (init.method ?? 'GET').toUpperCase();
  const headers = new Headers(init.headers);
  if (init.body !== undefined && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json');
  }
  if (method !== 'GET' && method !== 'HEAD' && csrfToken) {
    headers.set('X-CSRF-Token', csrfToken);
  }

  const response = await fetch(path, { ...init, headers, credentials: 'same-origin' });
  const text = await response.text();
  let payload: unknown = null;
  if (text) {
    try {
      payload = JSON.parse(text);
    } catch {
      payload = null;
    }
  }

  if (!response.ok) {
    const envelope = (payload ?? {}) as ErrorEnvelope;
    const error = new ApiError(
      envelope.error?.message ?? `HTTP ${response.status}`,
      response.status,
      envelope.error?.code ?? 'internal_error',
      envelope.error?.fields,
      envelope.error?.rolled_back,
    );
    if (response.status === 401 && path !== '/api/auth/login' && !options.suppressUnauthenticatedEvent) {
      window.dispatchEvent(new Event('nyxproxy:unauthenticated'));
    }
    throw error;
  }

  return payload as T;
}
