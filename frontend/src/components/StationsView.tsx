import { useState, useEffect, useMemo, useCallback } from 'react';
import { MonitorSmartphone, Search, Thermometer, Cpu, MemoryStick, User, RefreshCw } from 'lucide-react';
import { api, type ServerLiveStat } from '../lib/api';
import { formatGB } from '../lib/format';
import { NO_TEMPERATURE, NO_TEMPERATURE_HINT, formatTemperature, isAbove } from '../lib/metrics';
import { useSiteScope } from './ui/site-scope-context';
import { useNavigation } from './ui/navigation-context';

const POLL_MS = 10000;

// Limiares de temperatura de CPU para estação de escritório. Acima de 85 °C a
// máquina já reduz clock; acima de 70 °C costuma indicar ventilação obstruída.
const TEMP_WARN = 70;
const TEMP_CRITICAL = 85;

// Percentual a partir do qual CPU ou memória merecem atenção do suporte.
const USAGE_WARN = 75;
const USAGE_CRITICAL = 90;

const usageColor = (pct: number) => {
  if (pct >= USAGE_CRITICAL) return 'text-rose-400';
  if (pct >= USAGE_WARN) return 'text-amber-400';
  return 'text-white/90';
};

const tempColor = (celsius: number) => {
  if (celsius >= TEMP_CRITICAL) return 'text-rose-400';
  if (celsius >= TEMP_WARN) return 'text-amber-400';
  return 'text-emerald-400';
};

const formatUptime = (seconds: number) => {
  if (!seconds) return '—';
  const days = Math.floor(seconds / 86400);
  if (days > 0) return `${days}d`;
  const hours = Math.floor(seconds / 3600);
  if (hours > 0) return `${hours}h`;
  return `${Math.floor(seconds / 60)}min`;
};

const StatCard = ({ label, value, accent }: { label: string; value: string | number; accent: string }) => (
  <div className="glass-panel rounded-xl p-4 border border-white/5 bg-white/[0.02]">
    <div className={`text-3xl font-bold ${accent}`}>{value}</div>
    <div className="text-xs text-[#737373] uppercase tracking-widest mt-1">{label}</div>
  </div>
);

const StationsView = () => {
  // A unidade agora é escolhida uma vez na barra lateral e vale para todas as
  // telas; antes cada uma tinha o próprio filtro e o operador reescolhia.
  const { numericSiteId, siteName } = useSiteScope();
  const { openMachine } = useNavigation();

  const [stations, setStations] = useState<ServerLiveStat[]>([]);
  const [search, setSearch] = useState('');
  const [loading, setLoading] = useState(true);

  const fetchData = useCallback(async (signal?: AbortSignal) => {
    try {
      const data = await api.liveMetrics(signal);
      // Estação é o que reporta por agente; VPS coletada por SSH fica no
      // painel de infraestrutura.
      setStations(data.servers.filter(s => s.kind === 'agent'));
    } catch (err) {
      if (!signal?.aborted) console.error(err);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    fetchData(controller.signal);

    const interval = setInterval(() => fetchData(controller.signal), POLL_MS);
    return () => {
      clearInterval(interval);
      controller.abort();
    };
  }, [fetchData]);

  const filtered = useMemo(() => {
    const term = search.toLowerCase();
    return stations.filter(s => {
      if (numericSiteId !== null && s.site_id !== numericSiteId) return false;
      if (!term) return true;
      return (
        s.name.toLowerCase().includes(term) ||
        s.host_ip.includes(term) ||
        s.last_user.toLowerCase().includes(term)
      );
    });
  }, [stations, search, numericSiteId]);

  // Os cartões contam dentro do escopo: no painel de uma filial, o número
  // precisa ser o daquela filial.
  const inScope = useMemo(
    () => stations.filter(s => numericSiteId === null || s.site_id === numericSiteId),
    [stations, numericSiteId],
  );
  const online = inScope.filter(s => s.online).length;
  // Estação sem sensor não entra na conta: ausência de leitura não é máquina
  // fria, e tratá-la como zero mascararia o parque que ninguém está medindo.
  const hot = inScope.filter(s => isAbove(s.temperature_c, TEMP_WARN)).length;
  const pressured = inScope.filter(
    s => s.online && (s.cpu >= USAGE_WARN || (s.mem_total > 0 && (s.mem_used / s.mem_total) * 100 >= USAGE_WARN)),
  ).length;

  return (
    <div className="p-4 md:p-8 h-full flex flex-col overflow-hidden">
      <div className="mb-6">
        <h1 className="text-2xl font-light text-white mb-2 flex items-center gap-3">
          <MonitorSmartphone className="text-[#10b981]" /> Estações <span className="font-bold">Monitoradas</span>
        </h1>
        <p className="text-[#737373] text-sm">
          Máquinas com agente instalado. Cada uma se registra sozinha no primeiro envio.
        </p>
      </div>

      <div className="grid grid-cols-2 md:grid-cols-4 gap-3 mb-6">
        <StatCard label="Estações" value={inScope.length} accent="text-white" />
        <StatCard label="Online" value={online} accent="text-emerald-400" />
        <StatCard label="CPU ou RAM alta" value={pressured} accent={pressured > 0 ? 'text-amber-400' : 'text-[#737373]'} />
        <StatCard label="Temperatura alta" value={hot} accent={hot > 0 ? 'text-rose-400' : 'text-[#737373]'} />
      </div>

      <div className="glass-panel rounded-xl border border-white/5 bg-white/[0.02] flex flex-col flex-1 min-h-0 overflow-hidden">
        <div className="p-4 border-b border-white/5 flex flex-col md:flex-row gap-4 justify-between items-center bg-black/20">
          <div className="relative w-full md:w-96">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500" size={18} />
            <input
              type="text"
              placeholder="Buscar máquina, IP ou usuário..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="w-full bg-[#0c0c0e] border border-white/10 rounded-lg pl-10 pr-4 py-2 text-sm text-gray-300 focus:outline-none focus:border-[#10b981]/50 transition-all placeholder:text-gray-600"
            />
          </div>
        </div>

        <div className="flex-1 overflow-auto custom-scrollbar">
          <table className="w-full text-left text-sm whitespace-nowrap">
            <thead className="bg-[#0c0c0e]/80 sticky top-0 z-10">
              <tr>
                <th className="py-3 px-4 font-medium text-gray-500">Estado</th>
                <th className="py-3 px-4 font-medium text-gray-500">Máquina</th>
                <th className="py-3 px-4 font-medium text-gray-500">Unidade</th>
                <th className="py-3 px-4 font-medium text-gray-500">IP</th>
                <th className="py-3 px-4 font-medium text-gray-500">Usuário</th>
                <th className="py-3 px-4 font-medium text-gray-500 text-right">CPU</th>
                <th className="py-3 px-4 font-medium text-gray-500 text-right">Memória</th>
                <th className="py-3 px-4 font-medium text-gray-500 text-right">Temp.</th>
                <th className="py-3 px-4 font-medium text-gray-500 text-right">Ligada há</th>
                <th className="py-3 px-4 font-medium text-gray-500">Sistema</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-white/5">
              {loading && (
                <tr><td colSpan={10} className="py-8 text-center text-gray-500">Carregando...</td></tr>
              )}
              {!loading && filtered.length === 0 && (
                <tr>
                  <td colSpan={10} className="py-8 text-center text-gray-500">
                    Nenhuma estação com agente. Instale o <code className="text-amber-300">cmd/agent</code> nas máquinas.
                  </td>
                </tr>
              )}
              {filtered.map(s => {
                const memPct = s.mem_total > 0 ? (s.mem_used / s.mem_total) * 100 : 0;
                return (
                  <tr
                    key={s.id}
                    onClick={() => openMachine(s.id)}
                    className="hover:bg-white/[0.04] transition-colors cursor-pointer"
                  >
                    <td className="py-3 px-4">
                      <span className="flex items-center gap-2">
                        <span className={`w-2 h-2 rounded-full ${s.online ? 'bg-[#10b981] animate-pulse' : 'bg-[#737373]'}`} />
                        <span className={`text-[10px] font-bold tracking-widest uppercase ${s.online ? 'text-[#10b981]' : 'text-[#737373]'}`}>
                          {s.online ? 'Online' : 'Offline'}
                        </span>
                      </span>
                    </td>
                    <td className="py-3 px-4 text-gray-200 font-medium">{s.name}</td>
                    <td className="py-3 px-4 text-gray-400">{siteName(s.site_id)}</td>
                    <td className="py-3 px-4 font-mono text-xs text-gray-400 selectable">{s.host_ip}</td>
                    <td className="py-3 px-4 text-gray-300">
                      {s.last_user ? (
                        <span className="inline-flex items-center gap-1.5">
                          <User size={12} className="text-[#737373]" />
                          {s.last_user}
                        </span>
                      ) : <span className="text-gray-600">—</span>}
                    </td>
                    <td className={`py-3 px-4 text-right font-medium ${s.online ? usageColor(s.cpu) : 'text-gray-600'}`}>
                      {s.online ? `${s.cpu.toFixed(0)}%` : '—'}
                    </td>
                    <td className="py-3 px-4 text-right">
                      {s.online && s.mem_total > 0 ? (
                        <div className="flex flex-col items-end">
                          <span className={`font-medium ${usageColor(memPct)}`}>{memPct.toFixed(0)}%</span>
                          <span className="text-[10px] text-[#737373]">
                            {formatGB(s.mem_used)}/{formatGB(s.mem_total)} GB
                          </span>
                        </div>
                      ) : <span className="text-gray-600">—</span>}
                    </td>
                    <td className="py-3 px-4 text-right">
                      {s.temperature_c !== null ? (
                        <span className={`inline-flex items-center gap-1 font-medium ${tempColor(s.temperature_c)}`}>
                          <Thermometer size={12} />
                          {formatTemperature(s.temperature_c)}
                        </span>
                      ) : (
                        <span className="text-gray-600 text-xs" title={NO_TEMPERATURE_HINT}>
                          {NO_TEMPERATURE}
                        </span>
                      )}
                    </td>
                    <td className="py-3 px-4 text-right text-gray-400 text-xs">
                      {s.online ? formatUptime(s.uptime) : '—'}
                    </td>
                    <td className="py-3 px-4 text-gray-500 text-xs">
                      {s.platform || s.os || '—'}
                      {s.agent_version && <span className="block text-[10px] text-gray-600">agente {s.agent_version}</span>}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>

        <div className="px-4 py-2 border-t border-white/5 text-[10px] text-[#737373] uppercase tracking-widest flex items-center gap-4">
          <span className="flex items-center gap-1"><RefreshCw size={10} /> atualiza a cada {POLL_MS / 1000}s</span>
          <span className="flex items-center gap-1"><Cpu size={10} /> alerta em {USAGE_WARN}%</span>
          <span className="flex items-center gap-1"><MemoryStick size={10} /> crítico em {USAGE_CRITICAL}%</span>
          <span className="flex items-center gap-1"><Thermometer size={10} /> quente acima de {TEMP_WARN}°C</span>
        </div>
      </div>
    </div>
  );
};

export default StationsView;
