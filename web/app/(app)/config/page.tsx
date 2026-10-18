'use client';

import { CheckCircle2, RefreshCw } from 'lucide-react';
import { useRouter } from 'next/navigation';
import { useCallback, useEffect, useRef, useState } from 'react';
import { ConfigForm } from '@/components/config/ConfigForm';
import { Banner } from '@/components/ui/Banner';
import { Button } from '@/components/ui/Button';
import { Dialog } from '@/components/ui/Dialog';
import { Skeleton } from '@/components/ui/Skeleton';
import { useToast } from '@/components/ui/Toast';
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

function Spinner() {
  return (
    <span
      aria-hidden="true"
      className="h-3.5 w-3.5 shrink-0 animate-spin rounded-full border-2 border-accent/30 border-t-accent motion-reduce:animate-none"
    />
  );
}

export default function ConfigPage() {
  const { t } = useI18n();
  const tRef = useRef(t);
  useEffect(() => {
    tRef.current = t;
  }, [t]);
  const router = useRouter();
  const { toast } = useToast();
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
        'web.password':
          form.web.password !== '' || Boolean(meta?.passwords_set?.['web.password']),
      };
      setForm(saved);
      setOriginal(saved);
      setMeta((current) =>
        current
          ? { ...current, generation: result.generation, passwords_set: nextPasswordsSet }
          : current,
      );
      setLastApply({ ok: true, error: '', at: new Date().toISOString(), generation: result.generation });
      toast(t('config.saved'), 'success');
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
      toast(t('config.failed'), 'error');
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

  if (!form || !meta) {
    return (
      <div className="space-y-4">
        {error && <Banner tone="danger">{error}</Banner>}
        <Skeleton className="h-48 w-full" />
        <Skeleton className="h-64 w-full" />
        <div className="flex justify-end">
          <Button variant="secondary" onClick={() => void load()} disabled={loading}>
            <RefreshCw
              size={15}
              aria-hidden="true"
              className={loading ? 'animate-spin motion-reduce:animate-none' : undefined}
            />
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
        {saving && <Banner tone="info">{stepLabel}</Banner>}
        {!saving && lastApply && (
          <Banner tone={lastApply.ok ? 'success' : 'danger'}>
            {lastApply.ok
              ? t('config.last_ok', { generation: lastApply.generation })
              : t('config.last_failed', { error: lastApply.error })}
          </Banner>
        )}
        {!saving && error && <Banner tone="danger">{error}</Banner>}
      </div>

      <nav
        aria-label={t('config.section_nav')}
        className="glass sticky top-16 z-20 -mx-4 border-b border-line px-4 py-2 sm:-mx-6 sm:px-6 lg:-mx-8 lg:px-8"
      >
        <div className="flex gap-1 overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
          {sections.map((section) => (
            <a
              key={section.id}
              href={`#${section.id}`}
              className="shrink-0 rounded-full px-3 py-1.5 text-[13px] font-medium text-muted transition-colors hover:bg-surface-2 hover:text-ink focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/50"
            >
              {section.label}
            </a>
          ))}
        </div>
      </nav>

      <fieldset disabled={saving} className="min-w-0">
        <ConfigForm
          value={form}
          locked={meta.locked}
          errors={errors}
          interfaces={interfaces}
          passwordsSet={meta.passwords_set}
          onChange={setForm}
        />
      </fieldset>

      <div className="sticky bottom-3 z-20">
        <div className="glass flex flex-col gap-3 rounded-card border border-line px-3 py-3 shadow-float sm:flex-row sm:items-center sm:justify-between sm:px-4">
          <div className="flex min-w-0 items-center gap-2 text-[13px] text-muted" aria-live="polite">
            {saving ? (
              <>
                <Spinner />
                <span className="truncate">{stepLabel}</span>
              </>
            ) : dirty ? (
              <>
                <span className="h-2 w-2 shrink-0 rounded-full bg-warning" aria-hidden="true" />
                <span>{t('config.unsaved')}</span>
              </>
            ) : (
              <>
                <CheckCircle2 size={15} aria-hidden="true" className="shrink-0 text-success" />
                <span>{t('config.synced')}</span>
              </>
            )}
          </div>
          <div className="flex items-center justify-end gap-2">
            <Button
              variant="secondary"
              onClick={() => {
                if (dirty && !window.confirm(t('config.unsaved_confirm'))) return;
                void load();
              }}
              disabled={saving || loading}
            >
              <RefreshCw
                size={15}
                aria-hidden="true"
                className={loading ? 'animate-spin motion-reduce:animate-none' : undefined}
              />
              {t('common.reload')}
            </Button>
            <Button onClick={save} disabled={saving || !dirty}>
              {saving ? t('common.saving') : t('config.save')}
            </Button>
          </div>
        </div>
      </div>

      <Dialog
        open={confirmOpen}
        title={t('config.confirm_title')}
        confirmLabel={t('config.save')}
        cancelLabel={t('common.cancel')}
        onClose={() => setConfirmOpen(false)}
        onConfirm={() => void apply()}
        busy={saving}
      >
        {t('config.confirm_body')}
      </Dialog>
    </div>
  );
}
