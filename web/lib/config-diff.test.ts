import { describe, expect, it } from 'vitest';
import type { Config } from './types';
import { restartSections } from './config-diff';

function base(): Config {
  return {
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
}

describe('restartSections', () => {
  it('detects proxy, network, and logging changes', () => {
    const before = base();
    const after = structuredClone(before);
    after.proxy.listen_port = 1081;
    after.network.ipv6_pool_size = 128;
    after.logging.debug_level = 2;
    expect(restartSections(before, after)).toEqual(['proxy', 'network', 'logging']);
  });

  it('ignores monitoring, web credentials, and web settings', () => {
    const before = base();
    const after = structuredClone(before);
    after.monitoring.port = 9091;
    after.web.username = 'other';
    after.web.port = 18082;
    expect(restartSections(before, after)).toEqual([]);
  });
});
