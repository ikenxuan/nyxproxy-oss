import { afterEach, describe, expect, it, vi } from 'vitest';
import { copyText } from './clipboard';

describe('copyText', () => {
  afterEach(() => {
    vi.restoreAllMocks();
    document.querySelectorAll('textarea').forEach((node) => node.remove());
  });

  it('falls back to execCommand when the clipboard API is unavailable', async () => {
    const execCommand = vi.fn<() => boolean>(() => true);
    Object.defineProperty(document, 'execCommand', { value: execCommand, configurable: true });

    await copyText('socks5://admin:secret@example.com:1080');

    expect(execCommand).toHaveBeenCalledWith('copy');
    expect(document.querySelector('textarea')).toBeNull();
  });

  it('throws when the fallback copy is rejected', async () => {
    Object.defineProperty(document, 'execCommand', {
      value: vi.fn<() => boolean>(() => false),
      configurable: true,
    });

    await expect(copyText('value')).rejects.toThrow('copy failed');
    expect(document.querySelector('textarea')).toBeNull();
  });
});
