import { useCallback, useEffect, useState } from 'react';
import { api, apiErrorMessage, type Annotation, type HistoryPoint, type HistoryWindow } from '../../lib/api';
import { windowBounds } from '../../lib/metrics';
import { POLL } from '../../lib/polling';


const windowKey = (janela: HistoryWindow | null): string =>
  janela === null ? '' : typeof janela === 'string' ? janela : `${janela.from}|${janela.to ?? ''}`;

const parseKey = (key: string): HistoryWindow | null => {
  if (!key) return null;
  if (!key.includes('|')) return key as HistoryWindow;
  const [from, to] = key.split('|');
  return to ? { from, to } : { from };
};

export const useMetricSeries = (serverId: string, metric: string, janela: HistoryWindow | null) => {
  const [points, setPoints] = useState<HistoryPoint[]>([]);
  const [concluidoPara, setConcluidoPara] = useState<string | null>(null);
  const [recarregando, setRecarregando] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const key = windowKey(janela);
  const ativo = Boolean(serverId && metric && key);
  const alvo = `${serverId}|${metric}|${key}`;
  const loading = (ativo && concluidoPara !== alvo) || recarregando;

  const load = useCallback((signal?: AbortSignal) => {
    const current = parseKey(key);
    if (!serverId || !metric || current === null) return;
    api.history(serverId, metric, current, signal)
      .then((lista) => {
        setPoints(lista);
        setError(null);
      })
      .catch((err: unknown) => {
        if (signal?.aborted) return;
        setPoints([]);
        setError(apiErrorMessage(err, 'Falha ao carregar o histórico.'));
      })
      .finally(() => {
        if (signal?.aborted) return;
        setConcluidoPara(`${serverId}|${metric}|${key}`);
        setRecarregando(false);
      });
  }, [serverId, metric, key]);

  const recarregar = useCallback((signal?: AbortSignal) => {
    if (!ativo) return;
    setRecarregando(true);
    load(signal);
  }, [ativo, load]);

  useEffect(() => {
    const controller = new AbortController();
    load(controller.signal);
    if (!key.includes('|')) {
      const interval = setInterval(() => recarregar(controller.signal), POLL.serieDoGrafico);
      return () => {
        clearInterval(interval);
        controller.abort();
      };
    }
    return () => controller.abort();
  }, [load, recarregar, key]);

  return { points, loading, error, reload: () => recarregar() };
};

export const useAnnotations = (serverId: string, janela: HistoryWindow | null) => {
  const [annotations, setAnnotations] = useState<Annotation[]>([]);
  const [error, setError] = useState<string | null>(null);
  const key = windowKey(janela);

  const load = useCallback((signal?: AbortSignal) => {
    const current = parseKey(key);
    if (!serverId || current === null) return;
    api.annotations({ server_id: serverId, ...windowBounds(current) }, signal)
      .then((lista) => {
        setAnnotations(lista);
        setError(null);
      })
      .catch((err: unknown) => {
        if (signal?.aborted) return;
        setAnnotations([]);
        setError(apiErrorMessage(err, 'Falha ao carregar as anotações.'));
      });
  }, [serverId, key]);

  useEffect(() => {
    const controller = new AbortController();
    load(controller.signal);
    return () => controller.abort();
  }, [load]);

  return { annotations, error, reload: () => load() };
};
