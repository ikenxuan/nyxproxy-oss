'use client';

import { LogIn } from 'lucide-react';
import { useState, type FormEvent } from 'react';
import { Alert, Button, FieldError, Input, Label, Spinner, TextField } from '@heroui/react';
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
    <form onSubmit={submit} className="flex flex-col gap-5" aria-busy={submitting}>
      {error && (
        <Alert status="danger" role="alert">
          <Alert.Indicator />
          <Alert.Content>
            <Alert.Title>{error}</Alert.Title>
          </Alert.Content>
        </Alert>
      )}

      <TextField
        name="username"
        type="text"
        autoComplete="username"
        isRequired
        isDisabled={submitting}
        value={username}
        onChange={setUsername}
      >
        <Label>{t('login.username')}</Label>
        <Input
          autoCapitalize="none"
          spellCheck={false}
          placeholder={t('login.username')}
          fullWidth
        />
        <FieldError>{t('login.error')}</FieldError>
      </TextField>

      <TextField
        name="password"
        type="password"
        autoComplete="current-password"
        isRequired
        isDisabled={submitting}
        value={password}
        onChange={setPassword}
      >
        <Label>{t('login.password')}</Label>
        <Input placeholder="••••••••" fullWidth />
        <FieldError>{t('login.error')}</FieldError>
      </TextField>

      <Button type="submit" size="lg" fullWidth isDisabled={submitting}>
        {submitting ? (
          <>
            <Spinner size="sm" color="current" />
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
