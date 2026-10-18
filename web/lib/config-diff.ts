import type { Config } from './types';

export type RestartSection = 'proxy' | 'network' | 'logging';

export function restartSections(before: Config, after: Config): RestartSection[] {
  const changed: RestartSection[] = [];
  if (!same(before.proxy, after.proxy)) changed.push('proxy');
  if (!same(before.network, after.network)) changed.push('network');
  if (!same(before.logging, after.logging)) changed.push('logging');
  return changed;
}

export function hasChanges(before: Config, after: Config): boolean {
  return JSON.stringify(before) !== JSON.stringify(after);
}

function same(a: unknown, b: unknown): boolean {
  return JSON.stringify(a) === JSON.stringify(b);
}
