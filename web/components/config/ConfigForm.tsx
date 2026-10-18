'use client';

import {
  Activity,
  Globe2,
  Network,
  RefreshCw,
  ScrollText,
  Server,
} from 'lucide-react';
import type { LucideIcon } from 'lucide-react';
import type { ReactNode } from 'react';
import {
  Card,
  Chip,
  Description,
  Disclosure,
  FieldError,
  Input,
  Label,
  ListBox,
  NumberField,
  Select,
  Switch,
  TextField,
} from '@heroui/react';
import { useI18n } from '@/lib/i18n';
import type { Config, InterfaceInfo } from '@/lib/types';

interface Props {
  value: Config;
  locked: Record<string, string>;
  errors: Record<string, string>;
  interfaces: InterfaceInfo[];
  passwordsSet: Record<string, boolean>;
  onChange: (next: Config) => void;
}

function LockChip({
  locked,
  label,
}: {
  locked?: 'env' | 'restart_required';
  label: string;
}) {
  if (!locked) return null;
  return (
    <Chip size="sm" variant="soft" color={locked === 'env' ? 'warning' : 'default'}>
      <Chip.Label>{label}</Chip.Label>
    </Chip>
  );
}

function FieldLabel({
  label,
  locked,
  lockLabel,
}: {
  label: string;
  locked?: 'env' | 'restart_required';
  lockLabel: string;
}) {
  return (
    <div className="flex min-h-5 flex-wrap items-center justify-between gap-x-2 gap-y-1">
      <Label>{label}</Label>
      <LockChip locked={locked} label={lockLabel} />
    </div>
  );
}

function FieldMessages({ hint, error }: { hint?: string; error?: string }) {
  return (
    <>
      {hint && !error && <Description>{hint}</Description>}
      <FieldError>{error}</FieldError>
    </>
  );
}

interface CommonFieldProps {
  label: string;
  hint?: string;
  error?: string;
  locked?: 'env' | 'restart_required';
  lockLabel: string;
  disabled?: boolean;
}

function TextInputField({
  label,
  hint,
  error,
  locked,
  lockLabel,
  disabled,
  value,
  onChange,
  name,
  type = 'text',
  placeholder,
  autoComplete,
  mono,
}: CommonFieldProps & {
  value: string;
  onChange: (value: string) => void;
  name: string;
  type?: string;
  placeholder?: string;
  autoComplete?: string;
  mono?: boolean;
}) {
  return (
    <TextField
      name={name}
      type={type}
      value={value}
      onChange={onChange}
      isDisabled={disabled}
      isInvalid={Boolean(error)}
      fullWidth
    >
      <FieldLabel label={label} locked={locked} lockLabel={lockLabel} />
      <Input
        placeholder={placeholder}
        autoComplete={autoComplete}
        spellCheck={false}
        className={mono ? 'font-mono text-[13px]' : undefined}
        fullWidth
      />
      <FieldMessages hint={hint} error={error} />
    </TextField>
  );
}

function NumberInputField({
  label,
  hint,
  error,
  locked,
  lockLabel,
  disabled,
  value,
  onChange,
  name,
  min,
  max,
}: CommonFieldProps & {
  value: number;
  onChange: (value: number) => void;
  name: string;
  min?: number;
  max?: number;
}) {
  return (
    <NumberField
      name={name}
      value={value}
      onChange={(next) => onChange(next ?? min ?? 0)}
      minValue={min}
      maxValue={max}
      isDisabled={disabled}
      isInvalid={Boolean(error)}
      fullWidth
    >
      <FieldLabel label={label} locked={locked} lockLabel={lockLabel} />
      <NumberField.Group>
        <NumberField.DecrementButton />
        <NumberField.Input />
        <NumberField.IncrementButton />
      </NumberField.Group>
      <FieldMessages hint={hint} error={error} />
    </NumberField>
  );
}

function SelectField({
  label,
  hint,
  error,
  locked,
  lockLabel,
  disabled,
  selectedKey,
  onChange,
  name,
  options,
}: CommonFieldProps & {
  selectedKey: string;
  onChange: (value: string) => void;
  name: string;
  options: { id: string; label: string }[];
}) {
  return (
    <Select
      selectedKey={selectedKey}
      onSelectionChange={(key) => onChange(String(key))}
      isDisabled={disabled}
      isInvalid={Boolean(error)}
      name={name}
      fullWidth
    >
      <FieldLabel label={label} locked={locked} lockLabel={lockLabel} />
      <Select.Trigger>
        <Select.Value />
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover>
        <ListBox>
          {options.map((option) => (
            <ListBox.Item key={option.id} id={option.id} textValue={option.label}>
              {option.label}
              <ListBox.ItemIndicator />
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
      <FieldMessages hint={hint} error={error} />
    </Select>
  );
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
    <div className="flex items-start justify-between gap-4 rounded-2xl border border-border bg-surface-secondary/40 px-3.5 py-3">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <p className="text-[13px] font-medium text-foreground">{label}</p>
          {badge}
        </div>
        {description && <p className="mt-0.5 text-xs leading-relaxed text-muted">{description}</p>}
      </div>
      <Switch isSelected={checked} onChange={onChange} isDisabled={disabled} aria-label={label}>
        <Switch.Content>
          <Switch.Control>
            <Switch.Thumb />
          </Switch.Control>
        </Switch.Content>
      </Switch>
    </div>
  );
}

function Section({
  id,
  icon: Icon,
  title,
  description,
  children,
}: {
  id: string;
  icon: LucideIcon;
  title: string;
  description: string;
  children: ReactNode;
}) {
  return (
    <Card id={id} className="scroll-mt-32">
      <Card.Header>
        <div className="flex items-start gap-3">
          <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-accent-soft text-accent-soft-foreground">
            <Icon size={17} aria-hidden="true" />
          </span>
          <div className="min-w-0">
            <Card.Title>{title}</Card.Title>
            <Card.Description>{description}</Card.Description>
          </div>
        </div>
      </Card.Header>
      <Card.Content className="mt-1">{children}</Card.Content>
    </Card>
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
      <Chip size="sm" variant="soft" color={lock(field) === 'env' ? 'warning' : 'default'}>
        <Chip.Label>{lockLabel(field)}</Chip.Label>
      </Chip>
    ) : undefined;

  const interfaceOptions = [
    { id: '__auto__', label: t('config.interface_auto') },
    ...interfaces.map((iface) => ({
      id: iface.name,
      label: `${iface.name}${iface.suggested_prefix ? ` — ${iface.suggested_prefix}` : ''}`,
    })),
  ];
  if (
    value.network.interface_name &&
    !interfaces.some((iface) => iface.name === value.network.interface_name)
  ) {
    interfaceOptions.push({ id: value.network.interface_name, label: value.network.interface_name });
  }

  return (
    <div className="space-y-4">
      <Section id="section-proxy" icon={Server} title={t('config.proxy')} description={t('config.proxy_desc')}>
        <div className="grid gap-4 sm:grid-cols-2">
          <SelectField
            name="proxy.type"
            label={t('config.field.proxy.type')}
            selectedKey={value.proxy.type}
            onChange={(next) =>
              onChange({ ...value, proxy: { ...value.proxy, type: next as 'socks5' | 'https' } })
            }
            options={[
              { id: 'socks5', label: 'SOCKS5' },
              { id: 'https', label: 'HTTPS' },
            ]}
            lockLabel={lockLabel('proxy.type')}
          />
          <NumberInputField
            name="proxy.listen_port"
            label={t('config.field.proxy.listen_port')}
            value={value.proxy.listen_port}
            min={1}
            max={65535}
            onChange={(next) => onChange({ ...value, proxy: { ...value.proxy, listen_port: next } })}
            error={errors['proxy.listen_port']}
            lockLabel={lockLabel('proxy.listen_port')}
          />
          <TextInputField
            name="proxy.listen_address"
            label={t('config.field.proxy.listen_address')}
            value={value.proxy.listen_address}
            onChange={(next) =>
              onChange({ ...value, proxy: { ...value.proxy, listen_address: next } })
            }
            error={errors['proxy.listen_address']}
            lockLabel={lockLabel('proxy.listen_address')}
            mono
          />
          <TextInputField
            name="proxy.username"
            label={t('config.field.proxy.username')}
            value={value.proxy.username}
            onChange={(next) => onChange({ ...value, proxy: { ...value.proxy, username: next } })}
            error={errors['proxy.username']}
            lockLabel={lockLabel('proxy.username')}
            autoComplete="off"
          />
          <TextInputField
            name="proxy.password"
            label={t('config.field.proxy.password')}
            value={value.proxy.password}
            type="password"
            autoComplete="new-password"
            placeholder={passwordsSet['proxy.password'] ? t('config.password_keep') : undefined}
            onChange={(next) => onChange({ ...value, proxy: { ...value.proxy, password: next } })}
            error={errors['proxy.password']}
            lockLabel={lockLabel('proxy.password')}
          />
        </div>
      </Section>

      <Section id="section-network" icon={Network} title={t('config.network')} description={t('config.network_desc')}>
        <div className="grid gap-4 sm:grid-cols-2">
          <SelectField
            name="network.interface_name"
            label={t('config.field.network.interface_name')}
            selectedKey={value.network.interface_name || '__auto__'}
            onChange={(next) =>
              onChange({
                ...value,
                network: { ...value.network, interface_name: next === '__auto__' ? '' : next },
              })
            }
            options={interfaceOptions}
            locked={lock('network.interface_name')}
            lockLabel={lockLabel('network.interface_name')}
            disabled={isDisabled('network.interface_name')}
            error={errors['network.interface_name']}
          />
          <TextInputField
            name="network.ipv6_subnet"
            label={t('config.field.network.ipv6_subnet')}
            value={value.network.ipv6_subnet}
            onChange={(next) =>
              onChange({ ...value, network: { ...value.network, ipv6_subnet: next } })
            }
            placeholder="2001:db8::/64"
            hint={t('config.hint.ipv6_subnet')}
            error={errors['network.ipv6_subnet']}
            locked={lock('network.ipv6_subnet')}
            lockLabel={lockLabel('network.ipv6_subnet')}
            disabled={isDisabled('network.ipv6_subnet')}
            mono
          />
          <TextInputField
            name="network.dns_source_address"
            label={t('config.field.network.dns_source_address')}
            value={value.network.dns_source_address}
            onChange={(next) =>
              onChange({ ...value, network: { ...value.network, dns_source_address: next } })
            }
            hint={t('config.hint.dns_source')}
            error={errors['network.dns_source_address']}
            lockLabel={lockLabel('network.dns_source_address')}
            mono
          />
          <div className="hidden sm:block" aria-hidden="true" />
          <div className="grid gap-3 sm:col-span-2 sm:grid-cols-2">
            <ToggleRow
              label={t('config.field.network.ipv4_enabled')}
              checked={value.network.ipv4_enabled}
              onChange={(checked) =>
                onChange({ ...value, network: { ...value.network, ipv4_enabled: checked } })
              }
            />
            <ToggleRow
              label={t('config.field.network.ipv6_enabled')}
              checked={value.network.ipv6_enabled}
              onChange={(checked) =>
                onChange({ ...value, network: { ...value.network, ipv6_enabled: checked } })
              }
            />
          </div>
          {errors['network.ipv4_enabled'] && (
            <p role="alert" className="text-xs font-medium leading-relaxed text-danger sm:col-span-2">
              {errors['network.ipv4_enabled']}
            </p>
          )}
        </div>
      </Section>

      <Section id="section-rotation" icon={RefreshCw} title={t('config.rotation')} description={t('config.rotation_desc')}>
        <div className="space-y-4">
          <ToggleRow
            label={t('config.field.network.rotate_ipv6')}
            checked={value.network.rotate_ipv6}
            onChange={(checked) =>
              onChange({ ...value, network: { ...value.network, rotate_ipv6: checked } })
            }
          />
          <div className="grid gap-4 sm:grid-cols-3">
            <NumberInputField
              name="network.ipv6_pool_size"
              label={t('config.field.network.ipv6_pool_size')}
              value={value.network.ipv6_pool_size}
              min={1}
              disabled={!value.network.rotate_ipv6}
              onChange={(next) =>
                onChange({ ...value, network: { ...value.network, ipv6_pool_size: next } })
              }
              hint={t('config.hint.pool_size')}
              error={errors['network.ipv6_pool_size']}
              lockLabel={lockLabel('network.ipv6_pool_size')}
            />
            <NumberInputField
              name="network.ipv6_max_usage"
              label={t('config.field.network.ipv6_max_usage')}
              value={value.network.ipv6_max_usage}
              min={1}
              disabled={!value.network.rotate_ipv6}
              onChange={(next) =>
                onChange({ ...value, network: { ...value.network, ipv6_max_usage: next } })
              }
              error={errors['network.ipv6_max_usage']}
              lockLabel={lockLabel('network.ipv6_max_usage')}
            />
            <NumberInputField
              name="network.ipv6_max_age"
              label={t('config.field.network.ipv6_max_age')}
              value={value.network.ipv6_max_age}
              min={1}
              disabled={!value.network.rotate_ipv6}
              onChange={(next) =>
                onChange({ ...value, network: { ...value.network, ipv6_max_age: next } })
              }
              error={errors['network.ipv6_max_age']}
              lockLabel={lockLabel('network.ipv6_max_age')}
            />
          </div>
        </div>
      </Section>

      <Section id="section-monitoring" icon={Activity} title={t('config.monitoring')} description={t('config.monitoring_desc')}>
        <div className="space-y-4">
          <ToggleRow
            label={t('config.field.monitoring.enabled')}
            checked={value.monitoring.enabled}
            onChange={(checked) =>
              onChange({ ...value, monitoring: { ...value.monitoring, enabled: checked } })
            }
          />
          <div className="grid items-start gap-4 sm:grid-cols-2">
            <NumberInputField
              name="monitoring.port"
              label={t('config.field.monitoring.port')}
              value={value.monitoring.port}
              min={1}
              max={65535}
              disabled={!value.monitoring.enabled}
              onChange={(next) =>
                onChange({ ...value, monitoring: { ...value.monitoring, port: next } })
              }
              error={errors['monitoring.port']}
              lockLabel={lockLabel('monitoring.port')}
            />
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
      </Section>

      <Section id="section-logging" icon={ScrollText} title={t('config.logging')} description={t('config.logging_desc')}>
        <div className="max-w-xs">
          <SelectField
            name="logging.debug_level"
            label={t('config.field.logging.debug_level')}
            selectedKey={String(value.logging.debug_level)}
            onChange={(next) =>
              onChange({ ...value, logging: { debug_level: Number(next) } })
            }
            options={[
              { id: '0', label: t('config.debug.off') },
              { id: '1', label: t('config.debug.basic') },
              { id: '2', label: t('config.debug.detailed') },
            ]}
            error={errors['logging.debug_level']}
            lockLabel={lockLabel('logging.debug_level')}
          />
        </div>
      </Section>

      <Section id="section-web" icon={Globe2} title={t('config.web')} description={t('config.web_desc')}>
        <ToggleRow
          label={t('config.field.web.enabled')}
          checked={value.web.enabled}
          disabled={isDisabled('web.enabled')}
          badge={lockBadge('web.enabled')}
          onChange={(checked) => onChange({ ...value, web: { ...value.web, enabled: checked } })}
        />

        <div className="mt-4 grid gap-4 sm:grid-cols-2">
          <NumberInputField
            name="web.port"
            label={t('config.field.web.port')}
            value={value.web.port}
            min={1}
            max={65535}
            disabled={isDisabled('web.port')}
            onChange={(next) => onChange({ ...value, web: { ...value.web, port: next } })}
            error={errors['web.port']}
            locked={lock('web.port')}
            lockLabel={lockLabel('web.port')}
          />
          <TextInputField
            name="web.listen_address"
            label={t('config.field.web.listen_address')}
            value={value.web.listen_address}
            disabled={isDisabled('web.listen_address')}
            onChange={(next) =>
              onChange({ ...value, web: { ...value.web, listen_address: next } })
            }
            error={errors['web.listen_address']}
            locked={lock('web.listen_address')}
            lockLabel={lockLabel('web.listen_address')}
            mono
          />
          <TextInputField
            name="web.username"
            label={t('config.field.web.username')}
            value={value.web.username}
            autoComplete="off"
            onChange={(next) => onChange({ ...value, web: { ...value.web, username: next } })}
            error={errors['web.username']}
            lockLabel={lockLabel('web.username')}
          />
          <TextInputField
            name="web.password"
            label={t('config.field.web.password')}
            value={value.web.password}
            type="password"
            autoComplete="new-password"
            placeholder={passwordsSet['web.password'] ? t('config.password_keep') : undefined}
            onChange={(next) => onChange({ ...value, web: { ...value.web, password: next } })}
            error={errors['web.password']}
            lockLabel={lockLabel('web.password')}
          />
        </div>

        <Disclosure className="mt-5 overflow-hidden rounded-2xl border border-border bg-surface-secondary/30">
          <Disclosure.Trigger className="flex w-full items-center justify-between gap-3 px-4 py-3 text-[13px] font-medium text-foreground transition-colors hover:bg-default">
            {t('config.advanced')}
            <Disclosure.Indicator />
          </Disclosure.Trigger>
          <Disclosure.Content>
            <Disclosure.Body className="grid gap-4 border-t border-border p-4 sm:grid-cols-2">
              <TextInputField
                name="web.tls_cert_file"
                label={t('config.field.web.tls_cert_file')}
                value={value.web.tls_cert_file}
                disabled={isDisabled('web.tls_cert_file')}
                onChange={(next) =>
                  onChange({ ...value, web: { ...value.web, tls_cert_file: next } })
                }
                hint={t('config.hint.tls')}
                error={errors['web.tls_cert_file']}
                locked={lock('web.tls_cert_file')}
                lockLabel={lockLabel('web.tls_cert_file')}
                mono
              />
              <TextInputField
                name="web.tls_key_file"
                label={t('config.field.web.tls_key_file')}
                value={value.web.tls_key_file}
                disabled={isDisabled('web.tls_key_file')}
                onChange={(next) =>
                  onChange({ ...value, web: { ...value.web, tls_key_file: next } })
                }
                error={errors['web.tls_key_file']}
                locked={lock('web.tls_key_file')}
                lockLabel={lockLabel('web.tls_key_file')}
                mono
              />
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
                onChange={(checked) =>
                  onChange({ ...value, web: { ...value.web, cookie_secure: checked } })
                }
              />
            </Disclosure.Body>
          </Disclosure.Content>
        </Disclosure>
      </Section>
    </div>
  );
}
