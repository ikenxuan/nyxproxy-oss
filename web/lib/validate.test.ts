import { describe, expect, it } from 'vitest';
import type { Config } from './types';
import { validateConfig } from './validate';

function valid(): Config {
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

const passwordsSet = { 'proxy.password': true, 'web.password': true };

describe('validateConfig', () => {
  it('accepts empty passwords when a password is already stored', () => {
    expect(validateConfig(valid(), passwordsSet)).toEqual({});
  });

  it('requires credentials when none are stored', () => {
    const errors = validateConfig(valid(), {});
    expect(errors['proxy.password']).toBeTruthy();
    expect(errors['web.password']).toBeTruthy();
  });

  it('checks ports and subnet format', () => {
    const cfg = valid();
    cfg.proxy.listen_port = 0;
    cfg.monitoring.enabled = true;
    cfg.monitoring.port = 70000;
    cfg.network.ipv6_subnet = 'not-a-prefix';
    const errors = validateConfig(cfg, passwordsSet);
    expect(errors['proxy.listen_port']).toBeTruthy();
    expect(errors['monitoring.port']).toBeTruthy();
    expect(errors['network.ipv6_subnet']).toBeTruthy();
  });

  it('rejects overlapping listeners', () => {
    const cfg = valid();
    cfg.monitoring.enabled = true;
    cfg.monitoring.port = 1080;
    cfg.web.port = 1080;
    const errors = validateConfig(cfg, passwordsSet);
    expect(errors['monitoring.port']).toBeTruthy();
    expect(errors['web.port']).toBeTruthy();
  });

  it('requires at least one enabled protocol', () => {
    const cfg = valid();
    cfg.network.ipv4_enabled = false;
    cfg.network.ipv6_enabled = false;
    expect(validateConfig(cfg, passwordsSet)['network.ipv4_enabled']).toBeTruthy();
  });

  it('does not validate the web port while the console is disabled', () => {
    const cfg = valid();
    cfg.web.enabled = false;
    cfg.web.port = 0;
    expect(validateConfig(cfg, passwordsSet)).toEqual({});

    cfg.web.port = 70000;
    expect(validateConfig(cfg, passwordsSet)).toEqual({});
  });
});
