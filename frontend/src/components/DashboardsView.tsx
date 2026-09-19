import { useCallback, useEffect, useRef, useState } from 'react';
import {
  AlertTriangle, ArrowDown, ArrowUp, LayoutGrid, Pencil, Plus, Save, Trash2, X,
} from 'lucide-react';
import {
  api,
  apiErrorMessage,
  type Dashboard,
  type DashboardInput,
  type DashboardMetric,
  type DashboardPanel,
  type HistoryRange,
} from '../lib/api';
import { DASHBOARD_METRICS, HISTORY_RANGES, metricLabel } from '../lib/metrics';
import MetricChart from './charts/MetricChart';
import { useAnnotations, useMetricSeries } from './charts/useMetricSeries';
import { useDialog } from './ui/dialog-context';
import Select from './ui/Select';

const MAX_PANELS = 12;
const MAX_DASHBOARDS = 20;

interface ServerOption {
  id: string;
  name: string;
}

interface DraftPanel {
  key: number;
  title: string;
  server_id: string;
  metric: DashboardMetric;
  range: HistoryRange;
  width: 1 | 2;
}

interface Draft {
  dashboardId: number | null;
  name: string;
  panels: DraftPanel[];
}

const METRIC_OPTIONS = DASHBOARD_METRICS.map((m) => ({ value: m.key, label: m.label }));
const RANGE_OPTIONS = HISTORY_RANGES.map((r) => ({ value: r, label: r }));

const PanelChart = ({ panel }: { panel: DashboardPanel }) => {
  const series = useMetricSeries(panel.server_id, panel.metric, panel.range);
  const notes = useAnnotations(panel.server_id, panel.range);
  return (
    <div className="h-full flex flex-col gap-1">
      <div className="flex-1 min-h-0">
        <MetricChart
          points={series.points}
          metric={panel.metric}
          label={metricLabel(panel.metric)}
          annotations={notes.annotations}
          loading={series.loading}
          error={series.error}
        />
      </div>
      {notes.error && (
        <p role="alert" className="text-[11px] text-warn">Anotações indisponíveis: {notes.error}</p>
      )}
    </div>
  );
};

const DashboardsView = () => {
  const dialog = useDialog();
  const [dashboards, setDashboards] = useState<Dashboard[]>([]);
  const [servers, setServers] = useState<ServerOption[]>([]);
  const [serversError, setServersError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [saving, setSaving] = useState(false);
  const nextKey = useRef(1);

  const load = useCallback(async (select?: number) => {
    try {
      const list = await api.dashboards();
      setDashboards(list);
      setLoadError(null);
      setSelectedId((prev) => {
        const wanted = select ?? prev;
        return list.some((d) => d.id === wanted) ? (wanted as number) : (list[0]?.id ?? null);
      });
    } catch (err) {
      setLoadError(apiErrorMessage(err, 'Falha ao carregar os painéis.'));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
    api.liveMetrics()
      .then((data) => {
        setServers(data.servers.map(({ id, name }) => ({ id, name })));
        setServersError(null);
      })
      .catch((err) => setServersError(apiErrorMessage(err, 'Falha ao listar os servidores.')));
  }, [load]);

  const selected = dashboards.find((d) => d.id === selectedId) ?? null;
  const serverName = (id: string) => servers.find((s) => s.id === id)?.name ?? id;

  const toInput = (name: string, panels: { title: string; server_id: string; metric: DashboardMetric; range: HistoryRange; width: 1 | 2 }[]): DashboardInput => ({
    name,
    panels: panels.map(({ title, server_id, metric, range, width }) => ({
      title: title.trim() || `${metricLabel(metric)} de ${serverName(server_id)}`,
      server_id,
      metric,
      range,
      width,
    })),
  });

  const newDraftPanel = (): DraftPanel => ({
    key: nextKey.current++,
    title: '',
    server_id: servers[0]?.id ?? '',
    metric: 'cpu',
    range: '24h',
    width: 1,
  });

  const handleNew = async () => {
    const name = await dialog.prompt({
      title: 'Novo painel',
      message: 'Um nome que diga o que ele acompanha.',
      placeholder: 'Ex.: Loja em produção',
      confirmLabel: 'Criar',
    });
    if (!name) return;
    setDraft({ dashboardId: null, name, panels: [] });
  };

  const handleEdit = () => {
    if (!selected) return;
    setDraft({
      dashboardId: selected.id,
      name: selected.name,
      panels: selected.panels.map((p) => ({ ...p, key: nextKey.current++ })),
    });
  };

  const handleRename = async () => {
    if (!selected) return;
    const name = await dialog.prompt({
      title: 'Renomear painel',
      initialValue: selected.name,
      confirmLabel: 'Renomear',
    });
    if (!name || name === selected.name) return;
    try {
      await api.updateDashboard(selected.id, toInput(name, selected.panels));
      dialog.notify(`Painel renomeado para "${name}".`, 'success');
      load(selected.id);
    } catch (err) {
      dialog.notify(apiErrorMessage(err, 'Falha ao renomear o painel.'), 'error');
    }
  };

  const handleDelete = async () => {
    if (!selected) return;
    const confirmed = await dialog.confirm({
      title: `Apagar "${selected.name}"?`,
      message: 'O painel e a arrumação dos gráficos somem. As métricas e as anotações continuam guardadas.',
      confirmLabel: 'Apagar',
      danger: true,
    });
    if (!confirmed) return;
    try {
      await api.deleteDashboard(selected.id);
      dialog.notify(`Painel "${selected.name}" apagado.`, 'success');
      setSelectedId(null);
      load();
    } catch (err) {
      dialog.notify(apiErrorMessage(err, 'Falha ao apagar o painel.'), 'error');
    }
  };

  const updatePanel = (key: number, patch: Partial<DraftPanel>) =>
    setDraft((d) => (d ? { ...d, panels: d.panels.map((p) => (p.key === key ? { ...p, ...patch } : p)) } : d));

  const movePanel = (index: number, delta: number) =>
    setDraft((d) => {
      if (!d) return d;
      const target = index + delta;
      if (target < 0 || target >= d.panels.length) return d;
      const panels = [...d.panels];
      [panels[index], panels[target]] = [panels[target], panels[index]];
      return { ...d, panels };
    });

  const handleSave = async () => {
    if (!draft) return;
    setSaving(true);
    try {
      const body = toInput(draft.name, draft.panels);
      const saved = draft.dashboardId === null
        ? await api.createDashboard(body)
        : await api.updateDashboard(draft.dashboardId, body);
      dialog.notify(`Painel "${draft.name}" salvo.`, 'success');
      setDraft(null);
      load(saved?.id ?? draft.dashboardId ?? undefined);
    } catch (err) {
      dialog.notify(apiErrorMessage(err, 'Falha ao salvar o painel.'), 'error');
    } finally {
      setSaving(false);
    }
  };

  const serverOptions = servers.map((s) => ({ value: s.id, label: s.name }));
  const atDashboardLimit = dashboards.length >= MAX_DASHBOARDS;

  return (
    <div className="p-4 md:p-8 anim-rise">
      <div className="page-header">
        <div>
          <h1 className="page-title">Painéis</h1>
          <p className="page-desc">
            Grades de gráficos com as métricas que você acompanha todo dia, lado a lado. As anotações
            do histórico aparecem como marcas azuis em cada gráfico.
          </p>
        </div>
        {!loadError && !draft && (
          <button
            type="button"
            onClick={handleNew}
            className="btn btn-primary"
            disabled={atDashboardLimit}
            title={atDashboardLimit ? `Limite de ${MAX_DASHBOARDS} painéis atingido` : undefined}
          >
            <Plus size={16} strokeWidth={1.75} />
            Novo painel
          </button>
        )}
      </div>

      {loadError ? (
        <div className="panel p-6 flex items-center gap-3 text-sm text-text-mut">
          <AlertTriangle size={18} strokeWidth={1.75} className="text-warn shrink-0" />
          <span>{loadError}</span>
        </div>
      ) : loading ? (
        <p className="text-sm text-text-mut">Carregando...</p>
      ) : draft ? (
        <div className="panel p-5 flex flex-col gap-4">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <h2 className="text-base font-semibold tracking-tight text-text-hi">
              {draft.dashboardId === null ? 'Novo painel' : 'Editando'}: {draft.name}
            </h2>
            <span className="text-xs text-text-faint">
              {draft.panels.length} de {MAX_PANELS} gráficos
            </span>
          </div>

          {draft.panels.length === 0 && (
            <p className="text-sm text-text-mut">
              Adicione o primeiro gráfico: escolha o servidor, a métrica e a janela de tempo.
            </p>
          )}

          <ol className="flex flex-col gap-3">
            {draft.panels.map((p, index) => {
              const n = index + 1;
              return (
                <li
                  key={p.key}
                  data-testid={`editor-grafico-${n}`}
                  className="rounded-ctrl border border-line bg-ink-850 p-3 grid grid-cols-1 md:grid-cols-12 gap-3 items-end"
                >
                  <div className="md:col-span-3">
                    <label htmlFor={`painel-titulo-${p.key}`} className="eyebrow block mb-1.5">Título</label>
                    <input
                      id={`painel-titulo-${p.key}`}
                      aria-label={`Título do gráfico ${n}`}
                      type="text"
                      value={p.title}
                      onChange={(e) => updatePanel(p.key, { title: e.target.value })}
                      className="input-base w-full"
                      placeholder={metricLabel(p.metric)}
                    />
                  </div>
                  <div className="md:col-span-3">
                    <span className="eyebrow block mb-1.5">Servidor</span>
                    <Select
                      ariaLabel={`Servidor do gráfico ${n}`}
                      value={p.server_id}
                      onChange={(v) => updatePanel(p.key, { server_id: v })}
                      options={serverOptions}
                      placeholder="Nenhum servidor"
                    />
                  </div>
                  <div className="md:col-span-2">
                    <span className="eyebrow block mb-1.5">Métrica</span>
                    <Select
                      ariaLabel={`Métrica do gráfico ${n}`}
                      value={p.metric}
                      onChange={(v) => updatePanel(p.key, { metric: v as DashboardMetric })}
                      options={METRIC_OPTIONS}
                    />
                  </div>
                  <div className="md:col-span-1">
                    <span className="eyebrow block mb-1.5">Janela</span>
                    <Select
                      ariaLabel={`Janela do gráfico ${n}`}
                      value={p.range}
                      onChange={(v) => updatePanel(p.key, { range: v as HistoryRange })}
                      options={RANGE_OPTIONS}
                    />
                  </div>
                  <div className="md:col-span-2 flex gap-1">
                    {([1, 2] as const).map((w) => (
                      <button
                        key={w}
                        type="button"
                        aria-pressed={p.width === w}
                        onClick={() => updatePanel(p.key, { width: w })}
                        className={`btn btn-sm text-xs flex-1 ${
                          p.width === w ? 'bg-accent/10 border border-accent/40 text-accent' : 'btn-ghost'
                        }`}
                      >
                        {w === 1 ? '1 coluna' : '2 colunas'}
                      </button>
                    ))}
                  </div>
                  <div className="md:col-span-1 flex justify-end gap-0.5">
                    <button
                      type="button"
                      aria-label={`Subir gráfico ${n}`}
                      disabled={index === 0}
                      onClick={() => movePanel(index, -1)}
                      className="btn btn-ghost btn-sm px-1.5 disabled:opacity-30"
                    >
                      <ArrowUp size={14} strokeWidth={1.75} />
                    </button>
                    <button
                      type="button"
                      aria-label={`Descer gráfico ${n}`}
                      disabled={index === draft.panels.length - 1}
                      onClick={() => movePanel(index, 1)}
                      className="btn btn-ghost btn-sm px-1.5 disabled:opacity-30"
                    >
                      <ArrowDown size={14} strokeWidth={1.75} />
                    </button>
                    <button
                      type="button"
                      aria-label={`Remover gráfico ${n}`}
                      onClick={() => setDraft((d) => (d ? { ...d, panels: d.panels.filter((x) => x.key !== p.key) } : d))}
                      className="btn btn-ghost btn-sm px-1.5 hover:text-crit"
                    >
                      <X size={14} strokeWidth={1.75} />
                    </button>
                  </div>
                </li>
              );
            })}
          </ol>

          <div className="flex flex-wrap items-center gap-2">
            <button
              type="button"
              onClick={() => setDraft((d) => (d ? { ...d, panels: [...d.panels, newDraftPanel()] } : d))}
              disabled={draft.panels.length >= MAX_PANELS || servers.length === 0}
              className="btn btn-ghost"
            >
              <Plus size={16} strokeWidth={1.75} />
              Adicionar gráfico
            </button>
            {serversError ? (
              <span role="alert" className="text-xs text-crit">Servidores indisponíveis: {serversError}</span>
            ) : servers.length === 0 && (
              <span className="text-xs text-text-faint">Cadastre um servidor antes de montar gráficos.</span>
            )}
            <div className="ml-auto flex gap-2">
              <button type="button" onClick={() => setDraft(null)} className="btn btn-ghost">
                Cancelar
              </button>
              <button
                type="button"
                onClick={handleSave}
                disabled={saving || draft.panels.length === 0}
                className="btn btn-primary"
              >
                <Save size={16} strokeWidth={1.75} />
                Salvar painel
              </button>
            </div>
          </div>
        </div>
      ) : dashboards.length === 0 ? (
        <div className="panel p-8 flex flex-col items-center gap-3 text-center text-sm text-text-mut">
          <LayoutGrid size={28} strokeWidth={1.75} className="text-text-faint" />
          <p>Nenhum painel ainda. Crie o primeiro com Novo painel e escolha os gráficos que ele mostra.</p>
        </div>
      ) : (
        <div className="flex flex-col gap-5">
          <div className="flex flex-wrap items-center gap-2">
            <div className="flex flex-wrap gap-1 rounded-ctrl border border-line bg-ink-850 p-1">
              {dashboards.map((d) => (
                <button
                  key={d.id}
                  type="button"
                  aria-pressed={d.id === selectedId}
                  onClick={() => setSelectedId(d.id)}
                  className={`rounded-[0.4rem] px-3 py-1.5 text-xs font-semibold transition-colors ${
                    d.id === selectedId ? 'bg-ink-750 text-text-hi' : 'text-text-mut hover:text-text-hi'
                  }`}
                >
                  {d.name}
                </button>
              ))}
            </div>
            {selected && (
              <div className="ml-auto flex gap-1">
                <button type="button" onClick={handleEdit} className="btn btn-ghost btn-sm">
                  <Pencil size={14} strokeWidth={1.75} />
                  Editar gráficos
                </button>
                <button type="button" onClick={handleRename} className="btn btn-ghost btn-sm">
                  Renomear
                </button>
                <button type="button" onClick={handleDelete} className="btn btn-ghost btn-sm hover:text-crit">
                  <Trash2 size={14} strokeWidth={1.75} />
                  Apagar
                </button>
              </div>
            )}
          </div>

          {selected && selected.panels.length === 0 ? (
            <div className="panel p-8 text-center text-sm text-text-mut">
              Este painel ainda não tem gráficos. Use Editar gráficos para adicionar.
            </div>
          ) : (
            <div className="grid grid-cols-1 lg:grid-cols-2 gap-4 stagger">
              {selected?.panels.map((p) => (
                <div
                  key={p.id}
                  data-testid={`grafico-${p.id}`}
                  className={`panel p-4 flex flex-col gap-3 ${p.width === 2 ? 'lg:col-span-2' : ''}`}
                >
                  <div className="flex items-baseline justify-between gap-3">
                    <h3 className="text-sm font-semibold text-text-hi truncate">{p.title}</h3>
                    <span className="text-[11px] text-text-faint whitespace-nowrap">
                      {serverName(p.server_id)}, {metricLabel(p.metric)}, {p.range}
                    </span>
                  </div>
                  <div className="h-[240px] w-full">
                    <PanelChart panel={p} />
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
};

export default DashboardsView;
