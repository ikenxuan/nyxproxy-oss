import { fireEvent, render } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { Dialog } from './Dialog';

function renderDialog({ busy, onClose }: { busy?: boolean; onClose: () => void }) {
  render(
    <Dialog open title="Confirm" onClose={onClose} confirmLabel="Confirm" cancelLabel="Cancel" busy={busy}>
      Body
    </Dialog>,
  );
}

describe('Dialog', () => {
  it('closes on Escape when not busy', () => {
    const onClose = vi.fn<() => void>();
    renderDialog({ onClose });

    fireEvent.keyDown(document, { key: 'Escape' });

    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('does not close on Escape while busy', () => {
    const onClose = vi.fn<() => void>();
    renderDialog({ busy: true, onClose });

    fireEvent.keyDown(document, { key: 'Escape' });

    expect(onClose).not.toHaveBeenCalled();
  });
});
