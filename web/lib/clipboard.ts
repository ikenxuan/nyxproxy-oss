/**
 * Copies text to the clipboard.
 *
 * `navigator.clipboard` is only available in secure contexts (HTTPS or
 * localhost). The console is often reached over plain HTTP on a LAN address,
 * so fall back to a temporary textarea when needed.
 */
export async function copyText(value: string): Promise<void> {
  if (window.isSecureContext && navigator.clipboard) {
    try {
      await navigator.clipboard.writeText(value);
      return;
    } catch {
      // Permission or policy failure; try the legacy path below.
    }
  }

  const textarea = document.createElement('textarea');
  textarea.value = value;
  textarea.setAttribute('readonly', '');
  textarea.style.position = 'fixed';
  textarea.style.top = '0';
  textarea.style.left = '0';
  textarea.style.opacity = '0';
  document.body.appendChild(textarea);
  textarea.focus();
  textarea.select();
  textarea.setSelectionRange(0, value.length);

  let copied = false;
  try {
    copied = document.execCommand('copy');
  } finally {
    document.body.removeChild(textarea);
  }

  if (!copied) throw new Error('copy failed');
}
