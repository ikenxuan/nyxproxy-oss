'use client';

import { Languages, Monitor, Moon, Sun } from 'lucide-react';
import { Button, Dropdown, Label } from '@heroui/react';
import { useI18n } from '@/lib/i18n';
import { useTheme, type Theme } from '@/lib/theme';

export function LanguageMenu() {
  const { t, lang, setLang } = useI18n();
  return (
    <Dropdown>
      <Button
        variant="ghost"
        size="sm"
        isIconOnly
        aria-label={t('top.language')}
        className="text-muted"
      >
        <Languages size={17} aria-hidden="true" />
      </Button>
      <Dropdown.Popover placement="bottom end">
        <Dropdown.Menu
          selectionMode="single"
          selectedKeys={[lang]}
          onSelectionChange={(keys) => {
            const next = Array.from(keys)[0];
            if (next === 'zh' || next === 'en') setLang(next);
          }}
        >
          <Dropdown.Item id="zh" textValue="中文">
            <Label>中文</Label>
            <Dropdown.ItemIndicator />
          </Dropdown.Item>
          <Dropdown.Item id="en" textValue="English">
            <Label>English</Label>
            <Dropdown.ItemIndicator />
          </Dropdown.Item>
        </Dropdown.Menu>
      </Dropdown.Popover>
    </Dropdown>
  );
}

export function ThemeMenu() {
  const { t } = useI18n();
  const { theme, setTheme } = useTheme();
  const options: { id: Theme; label: string; icon: typeof Sun }[] = [
    { id: 'light', label: t('theme.light'), icon: Sun },
    { id: 'dark', label: t('theme.dark'), icon: Moon },
    { id: 'system', label: t('theme.system'), icon: Monitor },
  ];
  const Current = options.find((option) => option.id === theme)?.icon ?? Monitor;
  return (
    <Dropdown>
      <Button
        variant="ghost"
        size="sm"
        isIconOnly
        aria-label={t('top.theme')}
        className="text-muted"
      >
        <Current size={17} aria-hidden="true" />
      </Button>
      <Dropdown.Popover placement="bottom end">
        <Dropdown.Menu
          selectionMode="single"
          selectedKeys={[theme]}
          onSelectionChange={(keys) => {
            const next = Array.from(keys)[0];
            if (next === 'light' || next === 'dark' || next === 'system') setTheme(next);
          }}
        >
          {options.map(({ id, label, icon: Icon }) => (
            <Dropdown.Item key={id} id={id} textValue={label}>
              <Icon size={15} aria-hidden="true" className="text-muted" />
              <Label>{label}</Label>
              <Dropdown.ItemIndicator />
            </Dropdown.Item>
          ))}
        </Dropdown.Menu>
      </Dropdown.Popover>
    </Dropdown>
  );
}
