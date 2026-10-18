import { describe, expect, it } from 'vitest';
import { en } from './i18n/en';
import { zh } from './i18n/zh';

describe('i18n dictionaries', () => {
  it('have exactly the same keys', () => {
    expect(Object.keys(en).toSorted()).toEqual(Object.keys(zh).toSorted());
  });

  it('have no empty messages', () => {
    for (const value of Object.values(zh)) {
      expect(value).not.toBe('');
    }
    for (const value of Object.values(en)) {
      expect(value).not.toBe('');
    }
  });
});
