export type ProxyType = 'socks5' | 'https';

export interface ProxySettings {
  type: ProxyType;
  listen_address: string;
  listen_port: number;
  username: string;
  password: string;
}

export interface NetworkSettings {
  interface_name: string;
  ipv4_enabled: boolean;
  ipv6_enabled: boolean;
  ipv6_subnet: string;
  dns_source_address: string;
  rotate_ipv6: boolean;
  ipv6_pool_size: number;
  ipv6_max_usage: number;
  ipv6_max_age: number;
}

export interface MonitoringSettings {
  enabled: boolean;
  port: number;
  allow_remote: boolean;
}

export interface LoggingSettings {
  debug_level: number;
}

export interface WebSettings {
  enabled: boolean;
  listen_address: string;
  port: number;
  username: string;
  password: string;
  tls_cert_file: string;
  tls_key_file: string;
  trust_proxy_headers: boolean;
  cookie_secure: boolean;
}

export interface Config {
  proxy: ProxySettings;
  network: NetworkSettings;
  monitoring: MonitoringSettings;
  logging: LoggingSettings;
  web: WebSettings;
}

export interface Meta {
  path: string;
  generation: number;
  locked: Record<string, 'env' | 'restart_required'>;
  passwords_set: Record<string, boolean>;
}

export interface ConfigResponse {
  config: Config;
  meta: Meta;
}

export interface ApplyResult {
  changed: boolean;
  generation: number;
  restarted: { proxy: boolean; monitoring: boolean };
  reauth_required: boolean;
}

export interface SessionResponse {
  username: string;
  csrf_token: string;
  expires_at: string;
}

export interface ProxyStatus {
  state: 'starting' | 'running' | 'error' | 'stopped';
  type: string;
  listen: string;
  protocol: number;
  started_at?: string;
  error: string;
}

export interface Status {
  version: string;
  uptime_seconds: number;
  proxy: ProxyStatus;
  monitoring: { enabled: boolean; state: 'stopped' | 'running' | 'error'; error: string };
  pool: {
    enabled: boolean;
    size: number;
    prefix: string;
    rotations: number;
    max_usage: number;
    max_age_minutes: number;
  };
  metrics: {
    active_connections: number;
    total_connections: number;
    total_requests: number;
    bytes_sent: number;
    bytes_received: number;
  };
  config: {
    path: string;
    generation: number;
    last_applied_at?: string;
    last_apply: { ok: boolean; error: string; at: string; generation: number };
  };
  platform: { os: string; ip_command: boolean; warnings: string[] };
  apply: { in_progress: boolean; step: string };
}

export interface InterfaceInfo {
  name: string;
  addresses: string[];
  suggested_prefix: string;
}

export interface LogLine {
  seq: number;
  ts: string;
  line: string;
}
