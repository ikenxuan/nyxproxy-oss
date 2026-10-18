import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { LoginForm } from './LoginForm';
import { I18nProvider } from '@/lib/i18n';

describe('LoginForm', () => {
  it('submits the entered credentials', async () => {
    const onSubmit = vi
      .fn<(username: string, password: string) => Promise<void>>()
      .mockResolvedValue(undefined);
    render(
      <I18nProvider>
        <LoginForm onSubmit={onSubmit} />
      </I18nProvider>,
    );

    await userEvent.type(screen.getByLabelText(/username/i), 'admin');
    await userEvent.type(screen.getByLabelText(/password/i), 'secret');
    await userEvent.click(screen.getByRole('button', { name: /sign in|登录/i }));

    expect(onSubmit).toHaveBeenCalledWith('admin', 'secret');
  });

  it('renders a server error', () => {
    render(
      <I18nProvider>
        <LoginForm onSubmit={vi.fn<(username: string, password: string) => Promise<void>>()} error="invalid credentials" />
      </I18nProvider>,
    );
    expect(screen.getByRole('alert')).toHaveTextContent('invalid credentials');
  });
});
