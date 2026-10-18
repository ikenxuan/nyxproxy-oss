'use client';

import { LogIn } from 'lucide-react';
import { useState, type FormEvent } from 'react';
import { Banner } from '@/components/ui/Banner';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { useI18n } from '@/lib/i18n';

interface Props {
  onSubmit: (username: string, password: string) => Promise<void>;
  error?: string;
}

export function LoginForm({ onSubmit, error }: Props) {
  const { t } = useI18n();
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [submitting, setSubmitting] = useState(false);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setSubmitting(true);
    try {
      await onSubmit(username, password);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <form onSubmit={submit} className="space-y-5">
      {error && <Banner tone="danger">{error}</Banner>}
      <div className="space-y-1.5">
        <label htmlFor="login-username" className="block text-[13px] font-medium text-ink">
          {t('login.username')}
        </label>
        <Input
          id="login-username"
          name="username"
          autoComplete="username"
          spellCheck={false}
          autoCapitalize="none"
          value={username}
          onChange={(event) => setUsername(event.target.value)}
          required
        />
      </div>
      <div className="space-y-1.5">
        <label htmlFor="login-password" className="block text-[13px] font-medium text-ink">
          {t('login.password')}
        </label>
        <Input
          id="login-password"
          name="password"
          type="password"
          autoComplete="current-password"
          value={password}
          onChange={(event) => setPassword(event.target.value)}
          required
        />
      </div>
      <Button type="submit" size="lg" className="w-full" disabled={submitting}>
        {submitting ? (
          <>
            <span
              aria-hidden="true"
              className="h-3.5 w-3.5 animate-spin rounded-full border-2 border-current/30 border-t-current motion-reduce:animate-none"
            />
            {t('login.submitting')}
          </>
        ) : (
          <>
            <LogIn size={16} aria-hidden="true" />
            {t('login.submit')}
          </>
        )}
      </Button>
    </form>
  );
}
