import { useState, useEffect, useCallback } from 'react';
import { BellRing, Trash2, Plus } from 'lucide-react';
import { API_URL } from '../config';

interface ServerOption {
  id: string;
  name: string;
}

interface AlertRule {
  id: number;
  name: string;
  target: string;
  metric: string;
  operator: string;
  threshold: number;
  enabled: boolean;
  last_fired: string | null;
}

const METRIC_LABELS: Record<string, string> = {
  cpu: 'CPU (%)',
  mem: 'Memória (%)',
  disk: 'Disco (%)',
  load: 'Load',
};

const emptyForm = {
  name: '',
  target: '*',
  metric: 'cpu',
  operator: '>',
  threshold: 80,
  enabled: true,
};

const AlertRulesView = () => {
  const [rules, setRules] = useState<AlertRule[]>([]);
  const [servers, setServers] = useState<ServerOption[]>([]);
  const [loading, setLoading] = useState(true);
  const [form, setForm] = useState({ ...emptyForm });

  const fetchRules = useCallback(async () => {
    try {
      const res = await fetch(API_URL + '/api/alerts/rules');
      const data = await res.json();
      setRules(data || []);
    } catch (err) {
      console.error(err);
    } finally {
      setLoading(false);
    }
  }, []);

  const fetchServers = useCallback(async () => {
    try {
      const res = await fetch(API_URL + '/api/metrics/live');
      const json = await res.json();
      const opts: ServerOption[] = (json.servers || []).map((s: { id: string; name: string }) => ({
        id: s.id,
        name: s.name,
      }));
      setServers(opts);
    } catch (err) {
      console.error(err);
    }
  }, []);

  useEffect(() => {
    fetchRules();
    fetchServers();
  }, [fetchRules, fetchServers]);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!form.name) return;
    try {
      await fetch(API_URL + '/api/alerts/rules', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ...form, threshold: Number(form.threshold) }),
      });
      setForm({ ...emptyForm });
      fetchRules();
    } catch (err) {
      console.error(err);
    }
  };

  const handleDelete = async (id: number) => {
    if (!confirm('Remover esta regra de alerta?')) return;
    try {
      await fetch(`${API_URL}/api/alerts/rules?id=${id}`, { method: 'DELETE' });
      fetchRules();
    } catch (err) {
      console.error(err);
    }
  };

  const handleToggle = async (rule: AlertRule) => {
    try {
      await fetch(`${API_URL}/api/alerts/rules?id=${rule.id}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ enabled: !rule.enabled }),
      });
      fetchRules();
    } catch (err) {
      console.error(err);
    }
  };

  const targetName = (target: string) => {
    if (target === '*') return 'Todos';
    return servers.find((s) => s.id === target)?.name ?? target;
  };

  const inputClass =
    'w-full bg-black/40 border border-white/10 rounded-lg px-4 py-2 text-sm text-white focus:outline-none focus:border-[#10b981] transition-colors';

  return (
    <div className="p-8">
      <div className="flex items-center gap-3 mb-2">
        <BellRing size={22} className="text-[#10b981]" />
        <h1 className="text-2xl font-light text-white">
          Regras de <span className="font-bold">Alerta</span>
        </h1>
      </div>
      <p className="text-[#737373] text-sm mb-8">
        Defina limiares por métrica. O motor avalia periodicamente e dispara notificações quando violados.
      </p>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <div className="glass-panel p-6 rounded-xl border border-white/5 bg-white/[0.02] col-span-1 h-fit">
          <h2 className="text-sm font-bold tracking-widest text-[#737373] uppercase mb-6">Nova Regra</h2>
          <form onSubmit={handleCreate} className="flex flex-col gap-4">
            <div>
              <label className="text-xs text-[#737373] block mb-1">Nome</label>
              <input
                type="text"
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
                className={inputClass}
                placeholder="Ex: CPU alta produção"
                required
              />
            </div>
            <div>
              <label className="text-xs text-[#737373] block mb-1">Alvo</label>
              <select
                value={form.target}
                onChange={(e) => setForm({ ...form, target: e.target.value })}
                className={inputClass}
              >
                <option value="*">Todos</option>
                {servers.map((s) => (
                  <option key={s.id} value={s.id}>
                    {s.name}
                  </option>
                ))}
              </select>
            </div>
            <div>
              <label className="text-xs text-[#737373] block mb-1">Métrica</label>
              <select
                value={form.metric}
                onChange={(e) => setForm({ ...form, metric: e.target.value })}
                className={inputClass}
              >
                {Object.entries(METRIC_LABELS).map(([key, label]) => (
                  <option key={key} value={key}>
                    {label}
                  </option>
                ))}
              </select>
            </div>
            <div className="flex gap-3">
              <div className="w-24">
                <label className="text-xs text-[#737373] block mb-1">Operador</label>
                <select
                  value={form.operator}
                  onChange={(e) => setForm({ ...form, operator: e.target.value })}
                  className={inputClass}
                >
                  <option value=">">&gt;</option>
                  <option value="<">&lt;</option>
                </select>
              </div>
              <div className="flex-1">
                <label className="text-xs text-[#737373] block mb-1">Limiar</label>
                <input
                  type="number"
                  step="any"
                  value={form.threshold}
                  onChange={(e) => setForm({ ...form, threshold: Number(e.target.value) })}
                  className={inputClass}
                  required
                />
              </div>
            </div>
            <label className="flex items-center gap-2 text-xs text-[#737373] cursor-pointer">
              <input
                type="checkbox"
                checked={form.enabled}
                onChange={(e) => setForm({ ...form, enabled: e.target.checked })}
                className="accent-[#10b981]"
              />
              Ativar imediatamente
            </label>
            <button
              type="submit"
              className="mt-2 flex items-center justify-center gap-2 bg-[#10b981]/20 hover:bg-[#10b981]/30 border border-[#10b981]/50 text-[#10b981] font-bold text-xs uppercase tracking-widest py-3 rounded-lg transition-all"
            >
              <Plus size={14} />
              Criar Regra
            </button>
          </form>
        </div>

        <div className="glass-panel p-6 rounded-xl border border-white/5 bg-white/[0.02] col-span-2">
          <h2 className="text-sm font-bold tracking-widest text-[#737373] uppercase mb-6">Regras Configuradas</h2>
          {loading ? (
            <p className="text-sm text-[#737373]">Carregando...</p>
          ) : rules.length === 0 ? (
            <p className="text-sm text-[#737373]">Nenhuma regra cadastrada.</p>
          ) : (
            <div className="overflow-x-auto custom-scrollbar">
              <table className="w-full text-sm text-left border-collapse">
                <thead className="text-[10px] text-[#737373] uppercase tracking-widest bg-[#0c0c0e]">
                  <tr>
                    <th className="py-3 px-4 rounded-l">Status</th>
                    <th className="py-3 px-4">Nome</th>
                    <th className="py-3 px-4">Alvo</th>
                    <th className="py-3 px-4">Condição</th>
                    <th className="py-3 px-4 text-right rounded-r">Ação</th>
                  </tr>
                </thead>
                <tbody>
                  {rules.map((rule) => (
                    <tr key={rule.id} className="border-b border-white/[0.03] hover:bg-white/[0.05] transition-all">
                      <td className="py-4 px-4">
                        <button
                          onClick={() => handleToggle(rule)}
                          className="flex items-center gap-2"
                          title="Alternar ativação"
                        >
                          <span
                            className={`w-2 h-2 rounded-full ${
                              rule.enabled ? 'bg-[#10b981] animate-pulse' : 'bg-[#737373]'
                            }`}
                          ></span>
                          <span
                            className={`text-[10px] font-bold tracking-widest uppercase ${
                              rule.enabled ? 'text-[#10b981]' : 'text-[#737373]'
                            }`}
                          >
                            {rule.enabled ? 'Ativa' : 'Inativa'}
                          </span>
                        </button>
                      </td>
                      <td className="py-4 px-4 font-medium text-white/90">{rule.name}</td>
                      <td className="py-4 px-4 text-[#737373]">{targetName(rule.target)}</td>
                      <td className="py-4 px-4 text-white/90 font-mono text-xs">
                        {METRIC_LABELS[rule.metric] ?? rule.metric} {rule.operator} {rule.threshold}
                      </td>
                      <td className="py-4 px-4 text-right">
                        <button
                          onClick={() => handleDelete(rule.id)}
                          className="inline-flex items-center gap-1 text-xs text-red-400/80 hover:text-red-400 tracking-wider"
                        >
                          <Trash2 size={14} />
                          Remover
                        </button>
                      </td>
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
