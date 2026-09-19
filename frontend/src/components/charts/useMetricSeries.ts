import { useCallback, useEffect, useState } from 'react';
import { api, apiErrorMessage, type Annotation, type HistoryMetric, type HistoryPoint, type HistoryWindow } from '../../lib/api';
import { windowBounds } from '../../lib/metrics';

const REFRESH_MS = 15000;

const windowKey = (janela: HistoryWindow | null): string =>
  janela === null ? '' : typeof janela === 'string' ? janela : `${janela.from}|${janela.to ?? ''}`;

const parseKey = (key: string): HistoryWindow | null => {
  if (!key) return null;
  if (!key.includes('|')) return key as HistoryWindow;
  const [from, to] = key.split('|');
  return to ? { from, to } : { from };
};

export const useMetricSeries = (serverId: string, metric: HistoryMetric, janela: HistoryWindow | null) => {
  const [points, setPoints] = useState<HistoryPoint[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const key = windowKey(janela);

  const load = useCallback(async (signal?: AbortSignal) => {
    const current = parseKey(key);
    if (!serverId || current === null) return;
    setLoading(true);
    try {
      setPoints(await api.history(serverId, metric, current, signal));
      setError(null);
    } catch (err) {
      if (signal?.aborted) return;
      setPoints([]);
      setError(apiErrorMessage(err, 'Falha ao carregar o histórico.'));
    } finally {
      if (!signal?.aborted) setLoading(false);
    }
  }, [serverId, metric, key]);

  useEffect(() => {
    const controller = new AbortController();
    load(controller.signal);
    if (!key.includes('|')) {
      const interval = setInterval(() => load(controller.signal), REFRESH_MS);
      return () => {
        clearInterval(interval);
        controller.abort();
      };
    }
    return () => controller.abort();
  }, [load, key]);

  return { points, loading, error, reload: () => load() };
};

export const useAnnotations = (serverId: string, janela: HistoryWindow | null) => {
  const [annotations, setAnnotations] = useState<Annotation[]>([]);
  const [error, setError] = useState<string | null>(null);
  const key = windowKey(janela);

  const load = useCallback(async (signal?: AbortSignal) => {
    const current = parseKey(key);
    if (!serverId || current === null) return;
    try {
      setAnnotations(await api.annotations({ server_id: serverId, ...windowBounds(current) }, signal));
      setError(null);
    } catch (err) {
      if (signal?.aborted) return;
      setAnnotations([]);
      setError(apiErrorMessage(err, 'Falha ao carregar as anotações.'));
    }
  }, [serverId, key]);

  useEffect(() => {
    const controller = new AbortController();
    load(controller.signal);
    return () => controller.abort();
  }, [load]);

  return { annotations, error, reload: () => load() };
};
