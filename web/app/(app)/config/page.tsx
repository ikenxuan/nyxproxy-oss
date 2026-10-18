'use client';

import { CheckCircle2, RefreshCw, TriangleAlert } from 'lucide-react';
import { useRouter } from 'next/navigation';
import { useCallback, useEffect, useRef, useState } from 'react';
import {
  Alert,
  AlertDialog,
  Button,
  Chip,
  Fieldset,
  Modal,
  Skeleton,
  Spinner,
  toast,
} from '@heroui/react';
import { ConfigForm } from '@/components/config/ConfigForm';
import { api, ApiError } from '@/lib/api';
import { hasChanges, restartSections } from '@/lib/config-diff';
import { useDirty } from '@/lib/dirty';
import { errorMessage, useI18n, type MessageKey } from '@/lib/i18n';
import type { ApplyResult, Config, ConfigResponse, InterfaceInfo, Meta, Status } from '@/lib/types';
import { validateConfig } from '@/lib/validate';

function redactPasswords(cfg: Config): Config {
  return {
    ...cfg,
    proxy: { ...cfg.proxy, password: '' },
    web: { ...cfg.web, password: '' },
  };
}

export default function ConfigPage() {
  const { t } = useI18n();
  const tRef = useRef(t);
  useEffect(() => {
    tRef.current = t;
  }, [t]);
  const router = useRouter();
  const { setDirty } = useDirty();
  const redirecting = useRef(false);
  const loadSeq = useRef(0);
  const pollRef = useRef<number | null>(null);

  const [form, setForm] = useState<Config | null>(null);
  const [original, setOriginal] = useState<Config | null>(null);
  const [meta, setMeta] = useState<Meta | null>(null);
  const [interfaces, setInterfaces] = useState<InterfaceInfo[]>([]);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [step, setStep] = useState('');
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [reloadOpen, setReloadOpen] = useState(false);
  const [lastApply, setLastApply] = useState<Status['config']['last_apply'] | null>(null);

  const dirty = Boolean(form && original && hasChanges(original, form));

  useEffect(() => {
    setDirty(dirty);
    return () => setDirty(false);
  }, [dirty, setDirty]);

  useEffect(() => {
    const handler = (event: BeforeUnloadEvent) => {
      if (!dirty || redirecting.current) return;
      event.preventDefault();
      event.returnValue = '';
    };
    window.addEventListener('beforeunload', handler);
    return () => window.removeEventListener('beforeunload', handler);
  }, [dirty]);

  useEffect(() => {
    return () => {
      if (pollRef.current !== null) {
        window.clearInterval(pollRef.current);
        pollRef.current = null;
      }
    };
  }, []);

  const load = useCallback(async () => {
    const seq = ++loadSeq.current;
    setLoading(true);
    try {
      const [configResponse, ifaces, status] = await Promise.all([
        api<ConfigResponse>('/api/config'),
        api<InterfaceInfo[]>('/api/network/interfaces'),
        api<Status>('/api/status'),
      ]);
      if (seq !== loadSeq.current) return;
      setForm(configResponse.config);
      setOriginal(configResponse.config);
      setMeta(configResponse.meta);
      setInterfaces(ifaces);
      const applyAt = new Date(status.config.last_apply.at);
      setLastApply(
        Number.isFinite(applyAt.getTime()) && applyAt.getUTCFullYear() > 1970
          ? status.config.last_apply
          : null,
      );
      setErrors({});
      setError('');
    } catch (err) {
      if (seq !== loadSeq.current) return;
      setError(errorMessage(tRef.current, err));
    } finally {
      if (seq === loadSeq.current) setLoading(false);
    }
  }, []);

  useEffect(() => {
    // Initial load; state updates happen after the async response.
    // oxlint-disable-next-line react/set-state-in-effect
    void load();
  }, [load]);

  const apply = async () => {
    if (!form || !original) return;
    const clientErrors = validateConfig(form, meta?.passwords_set ?? {});
    if (Object.keys(clientErrors).length > 0) {
      setConfirmOpen(false);
      setErrors(clientErrors);
      setError(t('config.invalid'));
      return;
    }
    setConfirmOpen(false);
    setSaving(true);
    setErrors({});
    setError('');
    const poll = window.setInterval(async () => {
      try {
        const status = await api<Status>('/api/status');
        setStep(status.apply.step);
      } catch {
        // Ignore transient polling errors while the apply request is running.
      }
    }, 1000);
    pollRef.current = poll;

    try {
      const result = await api<ApplyResult>('/api/config', {
        method: 'PUT',
        body: JSON.stringify(form),
      });
      const saved = redactPasswords(form);
      const nextPasswordsSet = {
        ...meta?.passwords_set,
        'proxy.password':
          form.proxy.password !== '' || Boolean(meta?.passwords_set?.['proxy.password']),
        'web.password': form.web.password !== '' || Boolean(meta?.passwords_set?.['web.password']),
      };
      setForm(saved);
      setOriginal(saved);
      setMeta((current) =>
        current
          ? { ...current, generation: result.generation, passwords_set: nextPasswordsSet }
          : current,
      );
      setLastApply({ ok: true, error: '', at: new Date().toISOString(), generation: result.generation });
      toast.success(t('config.saved'));
      if (result.reauth_required) {
        redirecting.current = true;
        router.replace('/login');
        return;
      }
    } catch (err) {
      if (err instanceof ApiError && err.fields) {
        setErrors(err.fields);
      }
      const message = errorMessage(t, err);
      setError(message);
      setLastApply({
        ok: false,
        error: message,
        at: new Date().toISOString(),
        generation: meta?.generation ?? 0,
      });
      toast.danger(t('config.failed'), { description: message });
    } finally {
      window.clearInterval(poll);
      if (pollRef.current === poll) pollRef.current = null;
      setSaving(false);
      setStep('');
    }
  };

  const save = () => {
    if (!form || !original) return;
    if (restartSections(original, form).length > 0) {
      setConfirmOpen(true);
      return;
    }
    void apply();
  };

  const requestReload = () => {
    if (dirty) {
      setReloadOpen(true);
      return;
    }
    void load();
  };

  if (!form || !meta) {
    return (
      <div className="space-y-4">
        {error && (
          <Alert status="danger">
            <Alert.Indicator />
            <Alert.Content><Alert.Title>{error}</Alert.Title></Alert.Content>
          </Alert>
        )}
        <Skeleton className="h-52 w-full rounded-3xl" />
        <Skeleton className="h-72 w-full rounded-3xl" />
        <div className="flex justify-end">
          <Button variant="secondary" onPress={() => void load()} isDisabled={loading}>
            {loading ? <Spinner size="sm" color="current" /> : <RefreshCw size={15} aria-hidden="true" />}
            {loading ? t('common.loading') : t('common.retry')}
          </Button>
        </div>
      </div>
    );
  }

  const stepLabel = step ? t(`config.step.${step}` as MessageKey) : t('config.applying');
  const sections = [
    { id: 'section-proxy', label: t('config.proxy') },
    { id: 'section-network', label: t('config.network') },
    { id: 'section-rotation', label: t('config.rotation') },
    { id: 'section-monitoring', label: t('config.monitoring') },
    { id: 'section-logging', label: t('config.logging') },
    { id: 'section-web', label: t('config.web') },
  ];

  return (
    <div className="space-y-4">
      <div className="space-y-3">
        {saving && (
          <Alert status="accent">
            <Alert.Indicator><Spinner size="sm" /></Alert.Indicator>
            <Alert.Content><Alert.Title>{stepLabel}</Alert.Title></Alert.Content>
          </Alert>
        )}
        {!saving && lastApply && (
          <Alert status={lastApply.ok ? 'success' : 'danger'}>
            <Alert.Indicator />
            <Alert.Content>
              <Alert.Title>
                {lastApply.ok
                  ? t('config.last_ok', { generation: lastApply.generation })
                  : t('config.last_failed', { error: lastApply.error })}
              </Alert.Title>
            </Alert.Content>
          </Alert>
        )}
        {!saving && error && (
          <Alert status="danger">
            <Alert.Indicator />
            <Alert.Content><Alert.Title>{error}</Alert.Title></Alert.Content>
          </Alert>
        )}
      </div>

      <nav
        aria-label={t('config.section_nav')}
        className="glass sticky top-16 z-20 -mx-4 border-b border-border px-4 py-2 sm:-mx-6 sm:px-6 lg:-mx-8 lg:px-8"
      >
        <div className="flex gap-1 overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
          {sections.map((section) => (
            <a
              key={section.id}
              href={`#${section.id}`}
              className="shrink-0 rounded-full px-3 py-1.5 text-[13px] font-medium text-muted transition-colors hover:bg-default hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/50"
            >
              {section.label}
            </a>
          ))}
        </div>
      </nav>

      <Fieldset disabled={saving} className="min-w-0">
        <ConfigForm
          value={form}
          locked={meta.locked}
          errors={errors}
          interfaces={interfaces}
          passwordsSet={meta.passwords_set}
          onChange={setForm}
        />
      </Fieldset>

      <div className="sticky bottom-3 z-20">
        <div className="glass flex flex-col gap-3 rounded-3xl border border-border px-3 py-3 shadow-overlay sm:flex-row sm:items-center sm:justify-between sm:px-4">
          <div className="flex min-w-0 items-center gap-2 text-[13px] text-muted" aria-live="polite">
            {saving ? (
              <>
                <Spinner size="sm" />
                <span className="truncate">{stepLabel}</span>
              </>
            ) : dirty ? (
              <Chip size="sm" variant="soft" color="warning">
                <span className="h-1.5 w-1.5 rounded-full bg-current" aria-hidden="true" />
                <Chip.Label>{t('config.unsaved')}</Chip.Label>
              </Chip>
            ) : (
              <Chip size="sm" variant="soft" color="success">
                <CheckCircle2 size={12} aria-hidden="true" />
                <Chip.Label>{t('config.synced')}</Chip.Label>
              </Chip>
            )}
          </div>
          <div className="flex items-center justify-end gap-2">
            <Button variant="secondary" onPress={requestReload} isDisabled={saving || loading}>
              {loading ? (
                <Spinner size="sm" color="current" />
              ) : (
                <RefreshCw size={15} aria-hidden="true" />
              )}
              {t('common.reload')}
            </Button>
            <Button onPress={save} isDisabled={saving || !dirty}>
              {saving ? <Spinner size="sm" color="current" /> : null}
              {saving ? t('common.saving') : t('config.save')}
            </Button>
          </div>
        </div>
      </div>

      <Modal isOpen={confirmOpen} onOpenChange={setConfirmOpen}>
        <Modal.Backdrop>
          <Modal.Container>
            <Modal.Dialog className="sm:max-w-[440px]">
              <Modal.CloseTrigger aria-label={t('common.close')} />
              <Modal.Header>
                <Modal.Icon className="bg-warning-soft text-warning-soft-foreground">
                  <TriangleAlert size={18} aria-hidden="true" />
                </Modal.Icon>
                <Modal.Heading>{t('config.confirm_title')}</Modal.Heading>
              </Modal.Header>
              <Modal.Body>
                <p className="text-sm leading-relaxed text-muted">{t('config.confirm_body')}</p>
              </Modal.Body>
              <Modal.Footer>
                <Button slot="close" variant="secondary">
                  {t('common.cancel')}
                </Button>
                <Button onPress={() => void apply()}>{t('config.save')}</Button>
              </Modal.Footer>
            </Modal.Dialog>
          </Modal.Container>
        </Modal.Backdrop>
      </Modal>

      <AlertDialog isOpen={reloadOpen} onOpenChange={setReloadOpen}>
        <AlertDialog.Backdrop>
          <AlertDialog.Container>
            <AlertDialog.Dialog className="sm:max-w-[420px]">
              <AlertDialog.CloseTrigger aria-label={t('common.close')} />
              <AlertDialog.Header>
                <AlertDialog.Icon status="warning" />
                <AlertDialog.Heading>{t('config.reload_title')}</AlertDialog.Heading>
              </AlertDialog.Header>
              <AlertDialog.Body>
                <p className="text-sm leading-relaxed text-muted">{t('config.reload_body')}</p>
              </AlertDialog.Body>
              <AlertDialog.Footer>
                <Button slot="close" variant="secondary">
                  {t('common.cancel')}
                </Button>
                <Button
                  variant="danger"
                  onPress={() => {
                    setReloadOpen(false);
                    void load();
                  }}
                >
                  {t('common.reload')}
                </Button>
              </AlertDialog.Footer>
            </AlertDialog.Dialog>
          </AlertDialog.Container>
        </AlertDialog.Backdrop>
      </AlertDialog>
    </div>
  );
}
