import { useCallback, useEffect, useMemo, useState, type FormEvent } from 'react';
import { RefreshCw, Trash2, MessageSquarePlus, Globe } from 'lucide-react';
import {
  api,
  apiErrorMessage,
  type Annotation,
  type CustomWindow,
  type HistoryRange,
  type HistoryWindow,
} from '../lib/api';
import { formatDateTime } from '../lib/format';
import { AVISO_SEM_TENDENCIA, HISTORY_RANGES, janelaIndisponivel, metricasDoHistorico } from '../lib/metrics';
import { useCatalogo } from './ui/useCatalogo';
import LoadNotice from './ui/LoadNotice';
import MetricChart from './charts/MetricChart';
import { useAnnotations, useMetricSeries } from './charts/useMetricSeries';
import { useDialog } from './ui/dialog-context';
import Select from './ui/Select';

interface ServerOption {
  id: string;
  name: string;
}

const CUSTOM = 'custom';
const ANNOTATION_MAX = 280;

const localInputValue = (date: Date) =>
  new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 16);

const toIso = (local: string) => new Date(local).toISOString();

const MetricsHistoryView = () => {
  const dialog = useDialog();
  const [servers, setServers] = useState<ServerOption[]>([]);
  const [serverId, setServerId] = useState('');
  const [serversError, setServersError] = useState<string | null>(null);
  const catalogo = useCatalogo();
  const opcoesDeMetrica = useMemo(() => metricasDoHistorico(catalogo.metricas), [catalogo.metricas]);
  const [escolhida, setMetric] = useState('');
  const metric = escolhida || (opcoesDeMetrica[0]?.nome ?? '');
  const metrica = catalogo.buscar(metric);
  const [range, setRange] = useState<HistoryRange | typeof CUSTOM>('1h');
  const [customFrom, setCustomFrom] = useState('');
  const [customTo, setCustomTo] = useState('');
  const [custom, setCustom] = useState<CustomWindow | null>(null);
  const [customError, setCustomError] = useState<string | null>(null);

  const [noteText, setNoteText] = useState('');
  const [noteAt, setNoteAt] = useState(() => localInputValue(new Date()));
  const [noteGlobal, setNoteGlobal] = useState(false);
  const [savingNote, setSavingNote] = useState(false);

  const janela: HistoryWindow | null = range === CUSTOM ? custom : range;
  const series = useMetricSeries(serverId, metric, janela);
  const notes = useAnnotations(serverId, janela);

  const fetchServers = useCallback(async () => {
    try {
      const data = await api.liveMetrics();
      const opts = data.servers.map(({ id, name }) => ({ id, name }));
      setServers(opts);
      setServersError(null);
      setServerId((prev) => prev || (opts[0]?.id ?? ''));
    } catch (err) {
      setServersError(apiErrorMessage(err, 'Falha ao listar os servidores.'));
    }
  }, []);

  useEffect(() => {
    fetchServers();
  }, [fetchServers]);

  const applyCustom = (e: FormEvent) => {
    e.preventDefault();
    if (!customFrom) {
      setCustomError('Informe o início do período.');
      return;
    }
    if (customTo && new Date(customTo) <= new Date(customFrom)) {
      setCustomError('O fim precisa ser depois do início.');
      return;
    }
    setCustomError(null);
    setCustom({ from: toIso(customFrom), ...(customTo ? { to: toIso(customTo) } : {}) });
  };

  const handleAnnotate = async (e: FormEvent) => {
    e.preventDefault();
    const text = noteText.trim();
    if (!text || !serverId) return;
    setSavingNote(true);
    try {
      await api.createAnnotation({
        server_id: noteGlobal ? null : serverId,
        at: toIso(noteAt || localInputValue(new Date())),
        text,
      });
      setNoteText('');
      setNoteAt(localInputValue(new Date()));
      notes.reload();
      dialog.notify('Anotação registrada.', 'success');
    } catch (err) {
      dialog.notify(apiErrorMessage(err, 'Falha ao registrar a anotação.'), 'error');
    } finally {
      setSavingNote(false);
    }
  };

  const handleRemoveNote = async (note: Annotation) => {
    const confirmed = await dialog.confirm({
      title: 'Remover anotação?',
      message: `"${note.text}" deixa de aparecer em todos os gráficos.`,
      confirmLabel: 'Remover',
      danger: true,
    });
    if (!confirmed) return;
    try {
      await api.deleteAnnotation(note.id);
      notes.reload();
    } catch (err) {
      dialog.notify(apiErrorMessage(err, 'Falha ao remover a anotação.'), 'error');
    }
  };

  const escolherMetrica = (nova: string) => {
    setMetric(nova);
    if (range !== CUSTOM && janelaIndisponivel(catalogo.buscar(nova), range)) setRange('7d');
  };

  const rangeButton = (value: HistoryRange | typeof CUSTOM, label: string) => (
    <button
      key={value}
      type="button"
      aria-pressed={range === value}
      disabled={value !== CUSTOM && janelaIndisponivel(metrica, value)}
      title={value !== CUSTOM && janelaIndisponivel(metrica, value) ? AVISO_SEM_TENDENCIA : undefined}
      onClick={() => setRange(value)}
      className={`btn text-xs disabled:opacity-40 ${
        range === value ? 'bg-accent/10 border border-accent/40 text-accent' : 'btn-ghost'
      }`}
    >
      {label}
    </button>
  );

  const sortedNotes = [...notes.annotations].sort((a, b) => b.at.localeCompare(a.at));

  return (
    <div className="p-4 md:p-8 anim-rise flex flex-col gap-6">
      <div className="page-header !mb-0">
        <div>
          <h1 className="page-title">Histórico de métricas</h1>
          <p className="page-desc">
            Séries temporais agregadas por servidor. Janelas prontas se atualizam a cada 15 segundos;
            o período personalizado fica fixo.
          </p>
        </div>
      </div>

      <div className="panel p-6">
        <div className="flex flex-wrap items-end gap-4 mb-4">
          <div className="flex flex-col gap-1.5">
            <label htmlFor="history-server" className="eyebrow">Servidor</label>
            <Select
              id="history-server"
              value={serverId}
              onChange={setServerId}
              className="min-w-[220px]"
              placeholder="Nenhum servidor"
              options={servers.map((s) => ({ value: s.id, label: s.name }))}
            />
          </div>

          <div className="flex flex-col gap-1.5">
            <label htmlFor="history-metric" className="eyebrow">Métrica</label>
            <Select
              id="history-metric"
              value={metric}
              onChange={escolherMetrica}
              className="min-w-[160px]"
              options={opcoesDeMetrica.map((m) => ({ value: m.nome, label: m.rotulo }))}
            />
          </div>

          <div className="flex flex-col gap-1.5">
            <span className="eyebrow">Período</span>
            <div className="flex flex-wrap gap-1">
              {HISTORY_RANGES.map((r) => rangeButton(r, r))}
              {rangeButton(CUSTOM, 'Personalizado')}
            </div>
            {metrica !== undefined && !metrica.tem_tendencia && (
              <p className="max-w-prose text-[11px] text-text-faint">{AVISO_SEM_TENDENCIA}</p>
            )}
          </div>

          <button
            type="button"
            onClick={() => series.reload()}
            className="btn btn-ghost ml-auto text-xs"
          >
            <RefreshCw size={14} strokeWidth={1.75} className={series.loading ? 'animate-spin' : ''} />
            Atualizar
          </button>
        </div>

        <LoadNotice error={catalogo.erro} className="mb-4" />

        {serversError && (
          <p role="alert" className="mb-4 text-xs text-crit">{serversError}</p>
        )}

        {range === CUSTOM && (
          <form onSubmit={applyCustom} className="flex flex-wrap items-end gap-3 mb-4">
            <div className="flex flex-col gap-1.5">
              <label htmlFor="history-from" className="eyebrow">De</label>
              <input
                id="history-from"
                type="datetime-local"
                value={customFrom}
                onChange={(e) => setCustomFrom(e.target.value)}
                className="input-base"
              />
            </div>
            <div className="flex flex-col gap-1.5">
              <label htmlFor="history-to" className="eyebrow">Até</label>
              <input
                id="history-to"
                type="datetime-local"
                value={customTo}
                onChange={(e) => setCustomTo(e.target.value)}
                className="input-base"
              />
            </div>
            <button type="submit" className="btn btn-ghost">Aplicar</button>
            <p className="text-[11px] text-text-faint self-center">Sem fim, o período vai até agora.</p>
            {customError && <p role="alert" className="w-full text-xs text-crit">{customError}</p>}
          </form>
        )}

        <div className="h-[420px] w-full">
          {range === CUSTOM && !custom ? (
            <div className="h-full flex items-center justify-center text-sm text-text-faint">
              Escolha o início do período e clique em Aplicar.
            </div>
          ) : (
            <MetricChart
              points={series.points}
              unidade={catalogo.unidade(metric)}
              label={catalogo.rotulo(metric)}
              annotations={notes.annotations}
              loading={series.loading}
              error={series.error}
            />
          )}
        </div>
      </div>

      <div className="panel p-6">
        <h2 className="eyebrow mb-4">Anotações do período</h2>
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          <form onSubmit={handleAnnotate} className="flex flex-col gap-3">
            <div>
              <label htmlFor="note-text" className="eyebrow block mb-1.5">Texto da anotação</label>
              <input
                id="note-text"
                type="text"
                value={noteText}
                maxLength={ANNOTATION_MAX}
                onChange={(e) => setNoteText(e.target.value)}
                className="input-base w-full"
                placeholder="Ex.: deploy da versão 2.3"
              />
            </div>
            <div>
              <label htmlFor="note-at" className="eyebrow block mb-1.5">Horário da anotação</label>
              <input
                id="note-at"
                type="datetime-local"
                value={noteAt}
                onChange={(e) => setNoteAt(e.target.value)}
                className="input-base w-full"
              />
            </div>
            <label className="flex items-center gap-2 text-xs text-text-mut">
              <input
                type="checkbox"
                checked={noteGlobal}
                onChange={(e) => setNoteGlobal(e.target.checked)}
                aria-label="Global"
              />
              Global: aparece no gráfico de todos os servidores
            </label>
            <button
              type="submit"
              className="btn btn-primary"
              disabled={savingNote || !noteText.trim() || !serverId}
            >
              <MessageSquarePlus size={16} strokeWidth={1.75} />
              Anotar
            </button>
          </form>

          <div className="lg:col-span-2">
            {notes.error ? (
              <p role="alert" className="text-sm text-crit">{notes.error}</p>
            ) : sortedNotes.length === 0 ? (
              <p className="text-sm text-text-mut">
                Nenhuma anotação neste período. Anote um deploy ou uma manutenção para explicar um pico no gráfico.
              </p>
            ) : (
              <ul className="flex flex-col gap-2">
                {sortedNotes.map((note) => (
                  <li
                    key={note.id}
                    className="flex items-start justify-between gap-3 rounded-ctrl border border-line bg-ink-850 px-3 py-2"
                  >
                    <div className="min-w-0">
                      <p className="text-sm text-text-hi break-words">{note.text}</p>
                      <p className="mt-0.5 flex flex-wrap items-center gap-2 text-[11px] text-text-faint">
                        <span className="mono-data">{formatDateTime(note.at)}</span>
                        <span>{note.author}</span>
                        {note.server_id === null && (
                          <span className="badge badge-info">
                            <Globe size={11} strokeWidth={1.75} />
                            global
                          </span>
                        )}
                      </p>
                    </div>
                    <button
                      type="button"
                      onClick={() => handleRemoveNote(note)}
                      aria-label={`Remover anotação ${note.text}`}
                      className="btn btn-ghost btn-sm px-1.5 hover:text-crit"
                    >
                      <Trash2 size={14} strokeWidth={1.75} />
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </div>
      </div>
    </div>
  );
};

export default MetricsHistoryView;
