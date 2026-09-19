import { useCallback, useState } from 'react';
import { apiErrorMessage } from '../../lib/api';

export interface LoadStatus {
  error: string | null;
  lastOk: Date | null;
}

export const useLoadStatus = () => {
  const [status, setStatus] = useState<LoadStatus>({ error: null, lastOk: null });
  const ok = useCallback(() => setStatus({ error: null, lastOk: new Date() }), []);
  const fail = useCallback(
    (err: unknown, fallback: string) => setStatus((s) => ({ ...s, error: apiErrorMessage(err, fallback) })),
    [],
  );
  return { ...status, ok, fail };
};
