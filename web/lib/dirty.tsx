'use client';

import { createContext, useContext, useMemo, useState, type ReactNode } from 'react';

interface DirtyValue {
  dirty: boolean;
  setDirty: (dirty: boolean) => void;
}

const DirtyContext = createContext<DirtyValue | null>(null);

export function DirtyProvider({ children }: { children: ReactNode }) {
  const [dirty, setDirty] = useState(false);
  const value = useMemo(() => ({ dirty, setDirty }), [dirty]);
  return <DirtyContext.Provider value={value}>{children}</DirtyContext.Provider>;
}

export function useDirty(): DirtyValue {
  const value = useContext(DirtyContext);
  if (!value) throw new Error('useDirty must be used inside DirtyProvider');
  return value;
}
