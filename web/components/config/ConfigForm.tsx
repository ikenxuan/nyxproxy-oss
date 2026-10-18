'use client';

import { ChevronDown } from 'lucide-react';
import type { ReactNode } from 'react';
import { Badge } from '@/components/ui/Badge';
import { Card } from '@/components/ui/Card';
import { Input } from '@/components/ui/Input';
import { Select } from '@/components/ui/Select';
import { Switch } from '@/components/ui/Switch';
import { cn } from '@/lib/cn';
import { useI18n } from '@/lib/i18n';
import type { Config, InterfaceInfo } from '@/lib/types';
import { Field } from './Field';

interface Props {
  value: Config;
  locked: Record<string, string>;
  errors: Record<string, string>;
  interfaces: InterfaceInfo[];
  passwordsSet: Record<string, boolean>;
  onChange: (next: Config) => void;
}

function ToggleRow({
  label,
  description,
  checked,
  onChange,
  disabled,
  badge,
}: {
  label: string;
  description?: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
  disabled?: boolean;
  badge?: ReactNode;
}) {
  return (
    <div
      className={cn(
        'flex items-center justify-between gap-4 rounded-field border border-line bg-surface-2/40 px-3.5 py-3',
        disabled && 'opacity-70',
      )}
    >
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <p className="text-[13px] font-medium text-ink">{label}</p>
          {badge}
        </div>
        {description && <p className="mt-0.5 text-xs leading-relaxed text-muted">{description}</p>}
      </div>
      <Switch label={label} checked={checked} onChange={onChange} disabled={disabled} />
    </div>
  );
}

export function ConfigForm({ value, locked, errors, interfaces, passwordsSet, onChange }: Props) {
  const { t } = useI18n();

  const lock = (field: string) => locked[field] as 'env' | 'restart_required' | undefined;
  const lockLabel = (field: string) =>
    lock(field) === 'env' ? t('config.locked_env') : t('config.locked_restart');
  const isDisabled = (field: string) => Boolean(lock(field));
  const lockBadge = (field: string) =>
    lock(field) ? (
      <Badge tone={lock(field) === 'env' ? 'warning' : 'neutral'}>{lockLabel(field)}</Badge>
    ) : undefined;

  return (
    <div className="space-y-4">
      <Card
        id="section-proxy"
        className="scroll-mt-32"
        title={t('config.proxy')}
        description={t('config.proxy_desc')}
      >
        <div className="grid gap-4 sm:grid-cols-2">
          <Field id="proxy.type" label={t('config.field.proxy.type')} error={errors['proxy.type']}>
            <Select
              id="proxy.type"
              value={value.proxy.type}
              onChange={(event) =>
                onChange({ ...value, proxy: { ...value.proxy, type: event.target.value as 'socks5' | 'https' } })
              }
            >
              <option value="socks5">SOCKS5</option>
              <option value="https">HTTPS</option>
            </Select>
          </Field>
          <Field
            id="proxy.listen_port"
            label={t('config.field.proxy.listen_port')}
            error={errors['proxy.listen_port']}
          >
            <Input
              id="proxy.listen_port"
              type="number"
              inputMode="numeric"
              min={1}
              max={65535}
              value={value.proxy.listen_port}
              onChange={(event) =>
                onChange({ ...value, proxy: { ...value.proxy, listen_port: Number(event.target.value) } })
              }
            />
          </Field>
          <Field
            id="proxy.listen_address"
            label={t('config.field.proxy.listen_address')}
            error={errors['proxy.listen_address']}
          >
            <Input
              id="proxy.listen_address"
              value={value.proxy.listen_address}
              spellCheck={false}
              onChange={(event) =>
                onChange({ ...value, proxy: { ...value.proxy, listen_address: event.target.value } })
              }
            />
          </Field>
          <Field
            id="proxy.username"
            label={t('config.field.proxy.username')}
            error={errors['proxy.username']}
          >
            <Input
              id="proxy.username"
              value={value.proxy.username}
              autoComplete="off"
              spellCheck={false}
              onChange={(event) =>
                onChange({ ...value, proxy: { ...value.proxy, username: event.target.value } })
              }
            />
          </Field>
          <Field
            id="proxy.password"
            label={t('config.field.proxy.password')}
            error={errors['proxy.password']}
          >
            <Input
              id="proxy.password"
              type="password"
              autoComplete="new-password"
              placeholder={passwordsSet['proxy.password'] ? t('config.password_keep') : undefined}
              value={value.proxy.password}
              onChange={(event) =>
                onChange({ ...value, proxy: { ...value.proxy, password: event.target.value } })
              }
            />
          </Field>
        </div>
      </Card>

      <Card
        id="section-network"
        className="scroll-mt-32"
        title={t('config.network')}
        description={t('config.network_desc')}
      >
        <div className="grid gap-4 sm:grid-cols-2">
          <Field
            id="network.interface_name"
            label={t('config.field.network.interface_name')}
            error={errors['network.interface_name']}
            locked={lock('network.interface_name')}
            lockLabel={lockLabel('network.interface_name')}
          >
            <Select
              id="network.interface_name"
              disabled={isDisabled('network.interface_name')}
              value={value.network.interface_name}
              onChange={(event) =>
                onChange({ ...value, network: { ...value.network, interface_name: event.target.value } })
              }
            >
              <option value="">{t('config.interface_auto')}</option>
              {interfaces.map((iface) => (
                <option key={iface.name} value={iface.name}>
                  {iface.name}
                  {iface.suggested_prefix ? ` — ${iface.suggested_prefix}` : ''}
                </option>
              ))}
              {value.network.interface_name &&
                !interfaces.some((iface) => iface.name === value.network.interface_name) && (
                  <option value={value.network.interface_name}>{value.network.interface_name}</option>
                )}
            </Select>
          </Field>
          <Field
            id="network.ipv6_subnet"
            label={t('config.field.network.ipv6_subnet')}
            hint={t('config.hint.ipv6_subnet')}
            error={errors['network.ipv6_subnet']}
            locked={lock('network.ipv6_subnet')}
            lockLabel={lockLabel('network.ipv6_subnet')}
          >
            <Input
              id="network.ipv6_subnet"
              disabled={isDisabled('network.ipv6_subnet')}
              value={value.network.ipv6_subnet}
              placeholder="2001:db8::/64"
              spellCheck={false}
              className="font-mono text-[13px]"
              onChange={(event) =>
                onChange({ ...value, network: { ...value.network, ipv6_subnet: event.target.value } })
              }
            />
          </Field>
          <Field
            id="network.dns_source_address"
            label={t('config.field.network.dns_source_address')}
            hint={t('config.hint.dns_source')}
            error={errors['network.dns_source_address']}
          >
            <Input
              id="network.dns_source_address"
              value={value.network.dns_source_address}
              spellCheck={false}
              className="font-mono text-[13px]"
              onChange={(event) =>
                onChange({ ...value, network: { ...value.network, dns_source_address: event.target.value } })
              }
            />
          </Field>
          <div className="hidden sm:block" aria-hidden="true" />
          <div className="grid gap-3 sm:col-span-2 sm:grid-cols-2">
            <ToggleRow
              label={t('config.field.network.ipv4_enabled')}
              checked={value.network.ipv4_enabled}
              onChange={(checked) => onChange({ ...value, network: { ...value.network, ipv4_enabled: checked } })}
            />
            <ToggleRow
              label={t('config.field.network.ipv6_enabled')}
              checked={value.network.ipv6_enabled}
              onChange={(checked) => onChange({ ...value, network: { ...value.network, ipv6_enabled: checked } })}
            />
          </div>
          {errors['network.ipv4_enabled'] && (
            <p role="alert" className="text-xs font-medium leading-relaxed text-danger sm:col-span-2">
              {errors['network.ipv4_enabled']}
            </p>
          )}
        </div>
      </Card>

      <Card
        id="section-rotation"
        className="scroll-mt-32"
        title={t('config.rotation')}
        description={t('config.rotation_desc')}
      >
        <div className="space-y-4">
          <ToggleRow
            label={t('config.field.network.rotate_ipv6')}
            checked={value.network.rotate_ipv6}
            onChange={(checked) => onChange({ ...value, network: { ...value.network, rotate_ipv6: checked } })}
          />
          <div className="grid gap-4 sm:grid-cols-3">
            <Field
              id="network.ipv6_pool_size"
              label={t('config.field.network.ipv6_pool_size')}
              hint={t('config.hint.pool_size')}
              error={errors['network.ipv6_pool_size']}
            >
              <Input
                id="network.ipv6_pool_size"
                type="number"
                inputMode="numeric"
                min={1}
                disabled={!value.network.rotate_ipv6}
                value={value.network.ipv6_pool_size}
                onChange={(event) =>
                  onChange({ ...value, network: { ...value.network, ipv6_pool_size: Number(event.target.value) } })
                }
              />
            </Field>
            <Field
              id="network.ipv6_max_usage"
              label={t('config.field.network.ipv6_max_usage')}
              error={errors['network.ipv6_max_usage']}
            >
              <Input
                id="network.ipv6_max_usage"
                type="number"
                inputMode="numeric"
                min={1}
                disabled={!value.network.rotate_ipv6}
                value={value.network.ipv6_max_usage}
                onChange={(event) =>
                  onChange({ ...value, network: { ...value.network, ipv6_max_usage: Number(event.target.value) } })
                }
              />
            </Field>
            <Field
              id="network.ipv6_max_age"
              label={t('config.field.network.ipv6_max_age')}
              error={errors['network.ipv6_max_age']}
            >
              <Input
                id="network.ipv6_max_age"
                type="number"
                inputMode="numeric"
                min={1}
                disabled={!value.network.rotate_ipv6}
                value={value.network.ipv6_max_age}
                onChange={(event) =>
                  onChange({ ...value, network: { ...value.network, ipv6_max_age: Number(event.target.value) } })
                }
              />
            </Field>
          </div>
        </div>
      </Card>

      <Card
        id="section-monitoring"
        className="scroll-mt-32"
        title={t('config.monitoring')}
        description={t('config.monitoring_desc')}
      >
        <div className="space-y-4">
          <ToggleRow
            label={t('config.field.monitoring.enabled')}
            checked={value.monitoring.enabled}
            onChange={(checked) => onChange({ ...value, monitoring: { ...value.monitoring, enabled: checked } })}
          />
          <div className="grid items-start gap-4 sm:grid-cols-2">
            <Field
              id="monitoring.port"
              label={t('config.field.monitoring.port')}
              error={errors['monitoring.port']}
            >
              <Input
                id="monitoring.port"
                type="number"
                inputMode="numeric"
                min={1}
                max={65535}
                disabled={!value.monitoring.enabled}
                value={value.monitoring.port}
                onChange={(event) =>
                  onChange({ ...value, monitoring: { ...value.monitoring, port: Number(event.target.value) } })
                }
              />
            </Field>
            <ToggleRow
              label={t('config.field.monitoring.allow_remote')}
              checked={value.monitoring.allow_remote}
              disabled={!value.monitoring.enabled}
              onChange={(checked) =>
                onChange({ ...value, monitoring: { ...value.monitoring, allow_remote: checked } })
              }
            />
          </div>
        </div>
      </Card>

      <Card
        id="section-logging"
        className="scroll-mt-32"
        title={t('config.logging')}
        description={t('config.logging_desc')}
      >
        <div className="max-w-xs">
          <Field
            id="logging.debug_level"
            label={t('config.field.logging.debug_level')}
            error={errors['logging.debug_level']}
          >
            <Select
              id="logging.debug_level"
              value={String(value.logging.debug_level)}
              onChange={(event) =>
                onChange({ ...value, logging: { debug_level: Number(event.target.value) } })
              }
            >
              <option value="0">{t('config.debug.off')}</option>
              <option value="1">{t('config.debug.basic')}</option>
              <option value="2">{t('config.debug.detailed')}</option>
            </Select>
          </Field>
        </div>
      </Card>

      <Card
        id="section-web"
        className="scroll-mt-32"
        title={t('config.web')}
        description={t('config.web_desc')}
      >
        <ToggleRow
          label={t('config.field.web.enabled')}
          checked={value.web.enabled}
          disabled={isDisabled('web.enabled')}
          badge={lockBadge('web.enabled')}
          onChange={(checked) => onChange({ ...value, web: { ...value.web, enabled: checked } })}
        />

        <div className="mt-4 grid gap-4 sm:grid-cols-2">
          <Field
            id="web.port"
            label={t('config.field.web.port')}
            error={errors['web.port']}
            locked={lock('web.port')}
            lockLabel={lockLabel('web.port')}
          >
            <Input
              id="web.port"
              type="number"
              inputMode="numeric"
              min={1}
              max={65535}
              disabled={isDisabled('web.port')}
              value={value.web.port}
              onChange={(event) => onChange({ ...value, web: { ...value.web, port: Number(event.target.value) } })}
            />
          </Field>
          <Field
            id="web.listen_address"
            label={t('config.field.web.listen_address')}
            error={errors['web.listen_address']}
            locked={lock('web.listen_address')}
            lockLabel={lockLabel('web.listen_address')}
          >
            <Input
              id="web.listen_address"
              disabled={isDisabled('web.listen_address')}
              value={value.web.listen_address}
              spellCheck={false}
              onChange={(event) =>
                onChange({ ...value, web: { ...value.web, listen_address: event.target.value } })
              }
            />
          </Field>
          <Field
            id="web.username"
            label={t('config.field.web.username')}
            error={errors['web.username']}
          >
            <Input
              id="web.username"
              value={value.web.username}
              autoComplete="off"
              spellCheck={false}
              onChange={(event) => onChange({ ...value, web: { ...value.web, username: event.target.value } })}
            />
          </Field>
          <Field
            id="web.password"
            label={t('config.field.web.password')}
            error={errors['web.password']}
          >
            <Input
              id="web.password"
              type="password"
              autoComplete="new-password"
              placeholder={passwordsSet['web.password'] ? t('config.password_keep') : undefined}
              value={value.web.password}
              onChange={(event) => onChange({ ...value, web: { ...value.web, password: event.target.value } })}
            />
          </Field>
        </div>

        <details className="group mt-5 rounded-field border border-line bg-surface-2/30">
          <summary className="flex cursor-pointer list-none items-center justify-between gap-3 px-4 py-3 text-[13px] font-medium text-ink transition-colors hover:text-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-accent/50 [&::-webkit-details-marker]:hidden">
            {t('config.advanced')}
            <ChevronDown
              size={16}
              aria-hidden="true"
              className="text-faint transition-transform duration-200 group-open:rotate-180 motion-reduce:transition-none"
            />
          </summary>
          <div className="grid gap-4 border-t border-line p-4 sm:grid-cols-2">
            <Field
              id="web.tls_cert_file"
              label={t('config.field.web.tls_cert_file')}
              hint={t('config.hint.tls')}
              error={errors['web.tls_cert_file']}
              locked={lock('web.tls_cert_file')}
              lockLabel={lockLabel('web.tls_cert_file')}
            >
              <Input
                id="web.tls_cert_file"
                disabled={isDisabled('web.tls_cert_file')}
                value={value.web.tls_cert_file}
                spellCheck={false}
                className="font-mono text-[13px]"
                onChange={(event) =>
                  onChange({ ...value, web: { ...value.web, tls_cert_file: event.target.value } })
                }
              />
            </Field>
            <Field
              id="web.tls_key_file"
              label={t('config.field.web.tls_key_file')}
              error={errors['web.tls_key_file']}
              locked={lock('web.tls_key_file')}
              lockLabel={lockLabel('web.tls_key_file')}
            >
              <Input
                id="web.tls_key_file"
                disabled={isDisabled('web.tls_key_file')}
                value={value.web.tls_key_file}
                spellCheck={false}
                className="font-mono text-[13px]"
                onChange={(event) =>
                  onChange({ ...value, web: { ...value.web, tls_key_file: event.target.value } })
                }
              />
            </Field>
            <ToggleRow
              label={t('config.field.web.trust_proxy_headers')}
              description={t('config.hint.trust_proxy')}
              checked={value.web.trust_proxy_headers}
              disabled={isDisabled('web.trust_proxy_headers')}
              badge={lockBadge('web.trust_proxy_headers')}
              onChange={(checked) =>
                onChange({ ...value, web: { ...value.web, trust_proxy_headers: checked } })
              }
            />
            <ToggleRow
              label={t('config.field.web.cookie_secure')}
              description={t('config.hint.cookie_secure')}
              checked={value.web.cookie_secure}
              disabled={isDisabled('web.cookie_secure')}
              badge={lockBadge('web.cookie_secure')}
              onChange={(checked) => onChange({ ...value, web: { ...value.web, cookie_secure: checked } })}
            />
          </div>
        </details>
      </Card>
    </div>
  );
}
