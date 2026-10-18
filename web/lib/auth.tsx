'use client';

import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { api, setCSRFToken } from '@/lib/api';
import type { SessionResponse } from '@/lib/types';

interface AuthValue {
  username: string | null;
  loading: boolean;
  login: (username: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  refresh: () => Promise<void>;
}

const AuthContext = createContext<AuthValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [username, setUsername] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  // Bumped by session-changing actions so an in-flight bootstrap refresh cannot
  // apply an older result, such as a 401 that resolves after a successful login.
  const generation = useRef(0);

  const refresh = useCallback(async () => {
    const startedAt = generation.current;
    try {
      // A refresh handles its own 401; unlike other API calls it must not
      // broadcast the global unauthenticated event, because the response may
      // belong to a request that login/logout has already superseded.
      const session = await api<SessionResponse>(
        '/api/auth/session',
        {},
        { suppressUnauthenticatedEvent: true },
      );
      if (generation.current !== startedAt) return;
      setCSRFToken(session.csrf_token);
      setUsername(session.username);
    } catch {
      if (generation.current !== startedAt) return;
      setCSRFToken('');
      setUsername(null);
    } finally {
      if (generation.current === startedAt) setLoading(false);
    }
  }, []);

  useEffect(() => {
    // Bootstrap the session once. State is only updated after the async
    // response arrives, so this cannot cascade renders synchronously.
    // oxlint-disable-next-line react/set-state-in-effect
    void refresh();
    const onUnauthenticated = () => {
      setCSRFToken('');
      setUsername(null);
    };
    window.addEventListener('nyxproxy:unauthenticated', onUnauthenticated);
    return () => window.removeEventListener('nyxproxy:unauthenticated', onUnauthenticated);
  }, [refresh]);

  const login = useCallback(async (user: string, password: string) => {
    const requestGeneration = generation.current + 1;
    generation.current = requestGeneration;
    try {
      const session = await api<SessionResponse>('/api/auth/login', {
        method: 'POST',
        body: JSON.stringify({ username: user, password }),
      });
      if (generation.current !== requestGeneration) return;
      setCSRFToken(session.csrf_token);
      setUsername(session.username);
    } finally {
      if (generation.current === requestGeneration) setLoading(false);
    }
  }, []);

  const logout = useCallback(async () => {
    generation.current += 1;
    try {
      await api('/api/auth/logout', { method: 'POST' });
    } finally {
      setCSRFToken('');
      setUsername(null);
      setLoading(false);
    }
  }, []);

  const value = useMemo(
    () => ({ username, loading, login, logout, refresh }),
    [username, loading, login, logout, refresh],
  );
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthValue {
  const value = useContext(AuthContext);
  if (!value) throw new Error('useAuth must be used inside AuthProvider');
  return value;
}
