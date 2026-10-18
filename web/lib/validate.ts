import type { Config } from './types';

function isIPv6Prefix(value: string): boolean {
  const [addr, bits, extra] = value.split('/');
  if (extra !== undefined || bits !== '64' || !addr || !addr.includes(':')) return false;
  if (!/^[0-9a-fA-F:]+$/.test(addr)) return false;
  if ((addr.match(/::/g) ?? []).length > 1) return false;
  const groups = addr.split(':').filter(Boolean);
  return groups.length <= 8 && groups.every((group) => group.length <= 4);
}

function portError(port: number): string | undefined {
  if (!Number.isInteger(port) || port < 1 || port > 65535) return 'must be between 1 and 65535';
  return undefined;
}

function listenersOverlap(aAddr: string, aPort: number, bAddr: string, bPort: number): boolean {
  if (aPort !== bPort) return false;
  const a = aAddr || '0.0.0.0';
  const b = bAddr || '0.0.0.0';
  if (a === b) return true;
  return a === '0.0.0.0' || a === '::' || b === '0.0.0.0' || b === '::';
}

export function validateConfig(
  cfg: Config,
  passwordsSet: Record<string, boolean>,
): Record<string, string> {
  const errors: Record<string, string> = {};
  const set = (field: string, message: string) => {
    if (!errors[field]) errors[field] = message;
  };

  if (cfg.proxy.type !== 'socks5' && cfg.proxy.type !== 'https') {
    set('proxy.type', "must be 'https' or 'socks5'");
  }
  if (!cfg.proxy.username) set('proxy.username', 'must be set');
  if (!cfg.proxy.password && !passwordsSet['proxy.password']) {
    set('proxy.password', 'must be set');
  }
  const proxyPortError = portError(cfg.proxy.listen_port);
  if (proxyPortError) set('proxy.listen_port', proxyPortError);

  if (!cfg.network.ipv4_enabled && !cfg.network.ipv6_enabled) {
    set('network.ipv4_enabled', 'at least one of ipv4_enabled or ipv6_enabled must be true');
  }
  if (cfg.network.ipv6_subnet && !isIPv6Prefix(cfg.network.ipv6_subnet)) {
    set('network.ipv6_subnet', 'must be a valid IPv6 /64');
  }
  if (cfg.network.rotate_ipv6) {
    if (cfg.network.ipv6_pool_size <= 0) set('network.ipv6_pool_size', 'must be greater than 0');
    if (cfg.network.ipv6_max_usage <= 0) set('network.ipv6_max_usage', 'must be greater than 0');
    if (cfg.network.ipv6_max_age <= 0) set('network.ipv6_max_age', 'must be greater than 0');
  }

  let monitorPortOK = true;
  if (cfg.monitoring.enabled) {
    const err = portError(cfg.monitoring.port);
    if (err) {
      set('monitoring.port', err);
      monitorPortOK = false;
    }
  }

  if (cfg.web.enabled) {
    if (!cfg.web.username) set('web.username', 'must be set');
    if (!cfg.web.password && !passwordsSet['web.password']) set('web.password', 'must be set');
    const err = portError(cfg.web.port);
    if (err) set('web.port', err);
  }

  const webPortOK = !errors['web.port'];
  const monitorAddr = cfg.monitoring.allow_remote ? '0.0.0.0' : '127.0.0.1';
  if (proxyPortError === undefined && monitorPortOK && cfg.monitoring.enabled &&
      listenersOverlap(cfg.proxy.listen_address, cfg.proxy.listen_port, monitorAddr, cfg.monitoring.port)) {
    set('monitoring.port', 'conflicts with proxy.listen_port');
  }
  if (proxyPortError === undefined && webPortOK && cfg.web.enabled &&
      listenersOverlap(cfg.proxy.listen_address, cfg.proxy.listen_port, cfg.web.listen_address, cfg.web.port)) {
    set('web.port', 'conflicts with proxy.listen_port');
  }
  if (monitorPortOK && webPortOK && cfg.monitoring.enabled && cfg.web.enabled &&
      listenersOverlap(monitorAddr, cfg.monitoring.port, cfg.web.listen_address, cfg.web.port)) {
    set('web.port', 'conflicts with monitoring.port');
  }

  return errors;
}
