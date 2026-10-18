export function formatBytes(value: number): string {
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let size = value;
  let unit = 0;
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024;
    unit++;
  }
  return `${size.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`;
}

export function formatDuration(seconds: number): string {
  const total = Math.max(0, Math.floor(seconds));
  const days = Math.floor(total / 86400);
  const hours = Math.floor((total % 86400) / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const secs = total % 60;
  const parts: string[] = [];
  if (days) parts.push(`${days}d`);
  if (hours) parts.push(`${hours}h`);
  if (minutes) parts.push(`${minutes}m`);
  parts.push(`${secs}s`);
  return parts.join(' ');
}

function locale(lang: string): string {
  return lang === 'zh' ? 'zh-CN' : 'en-US';
}

const numberFormatters = new Map<string, Intl.NumberFormat>();

export function formatNumber(value: number, lang: string): string {
  const key = locale(lang);
  let formatter = numberFormatters.get(key);
  if (!formatter) {
    formatter = new Intl.NumberFormat(key);
    numberFormatters.set(key, formatter);
  }
  return formatter.format(value);
}

export function formatDate(value: string | undefined, lang: string): string {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat(locale(lang), {
    dateStyle: 'medium',
    timeStyle: 'medium',
  }).format(date);
}

const timeFormatters = new Map<string, Intl.DateTimeFormat>();

export function formatTime(value: string, lang: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  const key = locale(lang);
  let formatter = timeFormatters.get(key);
  if (!formatter) {
    formatter = new Intl.DateTimeFormat(key, {
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hour12: false,
    });
    timeFormatters.set(key, formatter);
  }
  return formatter.format(date);
}

const compactDateFormatters = new Map<string, Intl.DateTimeFormat>();

export function formatCompactDate(value: string | undefined, lang: string): string {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  const key = locale(lang);
  let formatter = compactDateFormatters.get(key);
  if (!formatter) {
    formatter = new Intl.DateTimeFormat(key, {
      dateStyle: 'short',
      timeStyle: 'short',
    });
    compactDateFormatters.set(key, formatter);
  }
  return formatter.format(date);
}
