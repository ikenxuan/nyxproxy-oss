'use client';

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useSyncExternalStore,
  type ReactNode,
} from 'react';
import { ApiError } from '@/lib/api';
import { en } from './en';
import { zh } from './zh';

export type Lang = 'zh' | 'en';
export type MessageKey = keyof typeof zh;

type Vars = Record<string, string | number>;
type Dictionary = Record<MessageKey, string>;

const dictionaries: Record<Lang, Dictionary> = { zh, en };
const STORAGE_KEY = 'nyxproxy-lang';
const CHANGE_EVENT = 'nyxproxy:lang-change';

interface I18nValue {
  lang: Lang;
  setLang: (lang: Lang) => void;
  t: (key: MessageKey, vars?: Vars) => string;
}

const I18nContext = createContext<I18nValue | null>(null);

function detectLang(): Lang {
  const saved = localStorage.getItem(STORAGE_KEY);
  if (saved === 'zh' || saved === 'en') return saved;
  const languages = navigator.languages?.length ? navigator.languages : [navigator.language];
  return languages.some((value) => value.toLowerCase().startsWith('zh')) ? 'zh' : 'en';
}

function getServerLang(): Lang {
  return 'en';
}

function subscribe(onChange: () => void): () => void {
  window.addEventListener('storage', onChange);
  window.addEventListener(CHANGE_EVENT, onChange);
  return () => {
    window.removeEventListener('storage', onChange);
    window.removeEventListener(CHANGE_EVENT, onChange);
  };
}

function storeLang(lang: Lang): void {
  localStorage.setItem(STORAGE_KEY, lang);
  window.dispatchEvent(new Event(CHANGE_EVENT));
}

function interpolate(template: string, vars?: Vars): string {
  if (!vars) return template;
  return template.replace(/\{(\w+)\}/g, (_, key: string) => String(vars[key] ?? `{${key}}`));
}

export function I18nProvider({ children }: { children: ReactNode }) {
  // useSyncExternalStore keeps the stored preference in sync across tabs and
  // avoids a setState-in-effect cascade during hydration.
  const lang = useSyncExternalStore(subscribe, detectLang, getServerLang);

  useEffect(() => {
    document.documentElement.lang = lang === 'zh' ? 'zh-CN' : 'en';
  }, [lang]);

  const setLang = useCallback((next: Lang) => storeLang(next), []);
  const t = useCallback(
    (key: MessageKey, vars?: Vars) => interpolate(dictionaries[lang][key] ?? zh[key] ?? key, vars),
    [lang],
  );

  const value = useMemo(() => ({ lang, setLang, t }), [lang, setLang, t]);
  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n(): I18nValue {
  const value = useContext(I18nContext);
  if (!value) throw new Error('useI18n must be used inside I18nProvider');
  return value;
}

export function errorMessage(t: I18nValue['t'], error: unknown): string {
  if (error instanceof ApiError) {
    const key = `errors.${error.code}` as MessageKey;
    if (key in zh) return t(key);
    return error.message;
  }
  if (error instanceof Error) return error.message;
  return t('errors.network');
}
