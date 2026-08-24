import { useState, useEffect, useCallback, type FormEvent } from 'react';
import { Trash2, Plus } from 'lucide-react';
import { api, type AlertRuleRecord as AlertRule } from '../lib/api';
import { relativeTime } from '../lib/format';
import Select, { type SelectOption } from './ui/Select';
import { useDialog } from './ui/dialog-context';
import { useRole } from './ui/session-context';
import { useSiteScope } from './ui/site-scope-context';

interface ServerOption {
  id: string;
  name: string;
}

const METRIC_LABELS: Record<string, string> = {
  cpu: 'CPU (%)',
  mem: 'Memória (%)',
  disk: 'Disco (%)',
  load: 'Load',
};

// Rótulos legíveis para os operadores de comparação da regra.
const OPERATORS: SelectOption[] = [
  { value: '>', label: 'maior que' },
  { value: '<', label: 'menor que' },
];

// Durações oferecidas para a histerese, em segundos.
//
// Lista fechada em vez de campo livre: a coluna guarda segundos, mas o operador
// pensa em minutos, e um campo numérico sem unidade convida a digitar 5 quando
// se quer 5 minutos. Aqui não há unidade para errar.
const DURATIONS: SelectOption[] = [
  { value: '0', label: 'Dispara na hora' },
  { value: '60', label: 'Após 1 minuto' },
  { value: '120', label: 'Após 2 minutos' },
  { value: '300', label: 'Após 5 minutos' },
  { value: '600', label: 'Após 10 minutos' },
  { value: '900', label: 'Após 15 minutos' },
  { value: '1800', label: 'Após 30 minutos' },
];

// Regra criada antes da coluna existir vem sem o campo; zero é o padrão certo,
// porque é exatamente o comportamento que ela tinha.
const durationOf = (rule: AlertRule): number => rule.for_duration_sec ?? 0;

// Rótulo curto para a coluna Condição, no formato que o operador escolheu.
const durationLabel = (seconds: number): string => {
  if (seconds <= 0) return '';
  const option = DURATIONS.find((d) => d.value === String(seconds));
  if (option) return option.label.replace('Após ', ' por ');
  return ` por ${Math.round(seconds / 60)} min`;
};

// O <Select> guarda um valor só; a unidade é distinguida por prefixo e
// traduzida para target_site_id no envio, que é como o backend modela.
const SITE_PREFIX = 'site:';

const emptyForm = {
  name: '',
  target: '*',
  metric: 'cpu',
  operator: '>',
  threshold: 80,
  for_duration_sec: 0,
  enabled: true,
};

const AlertRulesView = () => {
  const dialog = useDialog();
  const { sites, siteName } = useSiteScope();
  const { canOperate } = useRole();
  const [rules, setRules] = useState<AlertRule[]>([]);
  const [servers, setServers] = useState<ServerOption[]>([]);
  const [loading, setLoading] = useState(true);
  const [form, setForm] = useState({ ...emptyForm });

  const fetchRules = useCallback(async () => {
    try {
      setRules(await api.alertRules());
    } catch (err) {
      console.error(err);
    } finally {
      setLoading(false);
    }
  }, []);

  const fetchServers = useCallback(async (signal?: AbortSignal) => {
    try {
      const data = await api.liveMetrics(signal);
      setServers(data.servers.map(({ id, name }) => ({ id, name })));
    } catch (err) {
      if (!signal?.aborted) console.error(err);
    }
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    fetchRules();
    fetchServers(controller.signal);
    return () => controller.abort();
  }, [fetchRules, fetchServers]);

  const handleCreate = async (e: FormEvent) => {
    e.preventDefault();
    if (!form.name) return;
    try {
      const isSite = form.target.startsWith(SITE_PREFIX);
      await api.createAlertRule({
        ...form,
        target: isSite ? '*' : form.target,
        target_site_id: isSite ? Number(form.target.slice(SITE_PREFIX.length)) : null,
        threshold: Number(form.threshold),
        for_duration_sec: Number(form.for_duration_sec),
      });
      setForm({ ...emptyForm });
      fetchRules();
      dialog.notify(`Regra "${form.name}" criada.`, 'success');
    } catch (err) {
      console.error(err);
      dialog.notify('Erro ao criar a regra.', 'error');
    }
  };

  const handleDelete = async (rule: AlertRule) => {
    const confirmed = await dialog.confirm({
      title: `Remover a regra "${rule.name}"?`,
      message: 'As notificações desta condição param de ser disparadas.',
      confirmLabel: 'Remover',
      danger: true,
    });
    if (!confirmed) return;
    try {
      await api.deleteAlertRule(rule.id);
      fetchRules();
    } catch (err) {
      console.error(err);
      dialog.notify('Erro ao remover a regra.', 'error');
    }
  };

  const handleToggle = async (rule: AlertRule) => {
    try {
      await api.toggleAlertRule(rule.id, !rule.enabled);
      fetchRules();
    } catch (err) {
      console.error(err);
      dialog.notify('Erro ao alternar a regra.', 'error');
    }
  };

  // A regra por unidade chega com target "*" e target_site_id preenchido; sem
  // traduzir isso, a tabela mostraria "Todos" para uma regra de uma filial só.
  const targetName = (rule: AlertRule) => {
    if (rule.target_site_id !== null) return `Unidade: ${siteName(rule.target_site_id)}`;
    if (rule.target === '*') return 'Todos';
    return servers.find((s) => s.id === rule.target)?.name ?? rule.target;
  };

  return (
    <div className="p-4 md:p-8 anim-rise">
      <div className="page-header">
        <div>
          <h1 className="page-title">Regras de Alerta</h1>
          <p className="page-desc">
            Limiar por métrica, com histerese. O motor avalia periodicamente e notifica quando a condição persiste.
          </p>
        </div>
      </div>

      <div className={`grid grid-cols-1 gap-6 ${canOperate ? 'lg:grid-cols-3' : ''}`}>
        {/* O formulário só existe para Suporte TI; Visualizador vê a lista. */}
        {canOperate && (
        <div className="panel p-5 col-span-1 h-fit">
          <h2 className="eyebrow mb-5">Nova regra</h2>
          <form onSubmit={handleCreate} className="flex flex-col gap-4">
            <div>
              <label htmlFor="rule-name" className="eyebrow block mb-1.5">Nome</label>
              <input
                id="rule-name"
                type="text"
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
                className="input-base w-full"
                placeholder="Ex: CPU alta produção"
                required
              />
            </div>
            <div>
              <label htmlFor="rule-target" className="eyebrow block mb-1.5">Alvo</label>
              <Select
                id="rule-target"
                value={form.target}
                onChange={(v) => setForm({ ...form, target: v })}
                options={[
                  { value: '*', label: 'Todos' },
                  ...sites.map((s) => ({ value: `${SITE_PREFIX}${s.id}`, label: `Unidade: ${s.name}` })),
                  ...servers.map((s) => ({ value: s.id, label: s.name })),
                ]}
              />
              <p className="text-[10px] text-text-faint mt-1">
                Escolher uma unidade cobre todas as máquinas dela, inclusive as que entrarem depois.
              </p>
            </div>
            <div>
              <label htmlFor="rule-metric" className="eyebrow block mb-1.5">Métrica</label>
              <Select
                id="rule-metric"
                value={form.metric}
                onChange={(v) => setForm({ ...form, metric: v })}
                options={Object.entries(METRIC_LABELS).map(([key, label]) => ({ value: key, label }))}
              />
            </div>
            <div className="flex gap-3">
              <div className="w-28">
                <label htmlFor="rule-operator" className="eyebrow block mb-1.5">Operador</label>
                <Select
                  id="rule-operator"
                  value={form.operator}
                  onChange={(v) => setForm({ ...form, operator: v })}
                  options={OPERATORS}
                />
              </div>
              <div className="flex-1">
                <label htmlFor="rule-threshold" className="eyebrow block mb-1.5">Limiar</label>
                <input
                  id="rule-threshold"
                  type="number"
                  step="any"
                  value={form.threshold}
                  onChange={(e) => setForm({ ...form, threshold: Number(e.target.value) })}
                  className="input-base w-full"
                  required
                />
              </div>
            </div>
            <div>
              <label htmlFor="rule-duration" className="eyebrow block mb-1.5">
                Só alertar se persistir
              </label>
              <Select
                id="rule-duration"
                value={String(form.for_duration_sec)}
                onChange={(v) => setForm({ ...form, for_duration_sec: Number(v) })}
                options={DURATIONS}
              />
              <p className="text-[10px] text-text-faint mt-1">
                A condição precisa se manter sem interrupção. Uma leitura dentro do limite reinicia a contagem.
              </p>
            </div>
            <label className="flex items-center gap-2 text-xs text-text-mut cursor-pointer">
              <input
                type="checkbox"
                checked={form.enabled}
                onChange={(e) => setForm({ ...form, enabled: e.target.checked })}
                className="accent-accent"
              />
              Ativar imediatamente
            </label>
            <button type="submit" className="btn btn-primary mt-2 w-full">
              <Plus size={15} strokeWidth={1.75} />
              Criar regra
            </button>
          </form>
        </div>
        )}

        <div className={`panel p-5 ${canOperate ? 'col-span-2' : ''}`}>
          <h2 className="eyebrow mb-5">Regras configuradas</h2>
          {loading ? (
            <p className="text-sm text-text-faint">Carregando...</p>
          ) : rules.length === 0 ? (
            <p className="text-sm text-text-faint">Nenhuma regra cadastrada.</p>
          ) : (
            <div className="overflow-x-auto custom-scrollbar">
              <table className="table-base">
                <thead>
                  <tr>
                    <th>Status</th>
                    <th>Nome</th>
                    <th>Alvo</th>
                    <th>Condição</th>
                    <th>Último disparo</th>
                    {canOperate && <th className="text-right">Ação</th>}
                  </tr>
                </thead>
                <tbody>
                  {rules.map((rule) => (
                    <tr key={rule.id}>
                      <td>
                        {/* Alternar dispara escrita: Visualizador só vê o estado. */}
                        <button
                          onClick={() => canOperate && handleToggle(rule)}
                          className={canOperate ? '' : 'cursor-default'}
                          title={canOperate ? 'Alternar ativação' : undefined}
                          disabled={!canOperate}
                        >
                          <span className={`badge ${rule.enabled ? 'badge-ok' : 'badge-muted'}`}>
                            {rule.enabled ? 'Ativa' : 'Inativa'}
                          </span>
                        </button>
                      </td>
                      <td className={rule.enabled ? 'font-medium text-text-hi' : 'font-medium text-text-faint'}>{rule.name}</td>
                      <td className="text-text-mut">{targetName(rule)}</td>
                      <td className={`mono-data text-xs ${rule.enabled ? 'text-text-hi' : 'text-text-faint'}`}>
                        {METRIC_LABELS[rule.metric] ?? rule.metric} {rule.operator} {rule.threshold}
                        <span className="text-text-faint">{durationLabel(durationOf(rule))}</span>
                      </td>
                      <td className="text-text-faint text-xs">{relativeTime(rule.last_fired)}</td>
                      {canOperate && (
                        <td className="text-right">
                          <button
                            onClick={() => handleDelete(rule)}
                            className="inline-flex items-center gap-1.5 text-xs text-crit/80 hover:text-crit transition-colors"
                          >
                            <Trash2 size={14} strokeWidth={1.75} />
                            Remover
                          </button>
                        </td>
                      )}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};

export default AlertRulesView;
