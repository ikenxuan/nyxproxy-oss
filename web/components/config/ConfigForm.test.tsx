import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { I18nProvider } from '@/lib/i18n';
import type { Config } from '@/lib/types';
import { ConfigForm } from './ConfigForm';

const value: Config = {
  proxy: { type: 'socks5', listen_address: '0.0.0.0', listen_port: 1080, username: 'admin', password: '' },
  network: {
    interface_name: 'eth0',
    ipv4_enabled: false,
    ipv6_enabled: true,
    ipv6_subnet: '2001:db8::/64',
    dns_source_address: '',
    rotate_ipv6: true,
    ipv6_pool_size: 64,
    ipv6_max_usage: 100,
    ipv6_max_age: 30,
  },
  monitoring: { enabled: false, port: 9090, allow_remote: false },
  logging: { debug_level: 1 },
  web: {
    enabled: true,
    listen_address: '127.0.0.1',
    port: 18081,
    username: 'admin',
    password: '',
    tls_cert_file: '',
    tls_key_file: '',
    trust_proxy_headers: false,
    cookie_secure: false,
  },
};

describe('ConfigForm', () => {
  it('disables locked fields and shows field errors', () => {
    render(
      <I18nProvider>
        <ConfigForm
          value={value}
          locked={{ 'network.ipv6_subnet': 'env', 'web.port': 'restart_required' }}
          errors={{
            'proxy.listen_port': 'must be between 1 and 65535',
            'network.ipv4_enabled': 'at least one of ipv4_enabled or ipv6_enabled must be true',
          }}
          interfaces={[]}
          passwordsSet={{ 'proxy.password': true, 'web.password': true }}
          onChange={vi.fn<() => void>()}
        />
      </I18nProvider>,
    );

    expect(screen.getByRole('textbox', { name: 'IPv6 /64 subnet' })).toBeDisabled();
    expect(screen.getByRole('textbox', { name: 'Console port' })).toBeDisabled();
    expect(screen.getByText('must be between 1 and 65535')).toBeInTheDocument();
    expect(
      screen.getByText('at least one of ipv4_enabled or ipv6_enabled must be true'),
    ).toBeInTheDocument();
    expect(screen.getByText('Managed by environment')).toBeInTheDocument();
    expect(screen.getByText('Restart required')).toBeInTheDocument();
  });
});
