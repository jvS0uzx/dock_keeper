import { useState, useEffect, useMemo, useCallback } from 'react';
import { Search, Thermometer, Cpu, MemoryStick, User, RefreshCw } from 'lucide-react';
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
  if (pct >= USAGE_CRITICAL) return 'text-crit';
  if (pct >= USAGE_WARN) return 'text-warn';
  return 'text-text-hi';
};

const tempColor = (celsius: number) => {
  if (celsius >= TEMP_CRITICAL) return 'text-crit';
  if (celsius >= TEMP_WARN) return 'text-warn';
  return 'text-ok';
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
  <div className="stat-card">
    <div className={`stat-value ${accent}`}>{value}</div>
    <div className="eyebrow mt-1.5">{label}</div>
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
    <div className="p-4 md:p-8 h-full flex flex-col overflow-hidden anim-rise">
      <div className="page-header">
        <div>
          <h1 className="page-title">Estações</h1>
          <p className="page-desc">
            Máquinas com agente instalado. Cada uma se registra sozinha no primeiro envio.
          </p>
        </div>
      </div>

      <div className="grid grid-cols-2 md:grid-cols-4 gap-3 mb-6 stagger">
        <StatCard label="Estações" value={inScope.length} accent="" />
        <StatCard label="Online" value={online} accent="text-ok" />
        <StatCard label="CPU ou RAM alta" value={pressured} accent={pressured > 0 ? 'text-warn' : 'text-text-faint'} />
        <StatCard label="Temperatura alta" value={hot} accent={hot > 0 ? 'text-crit' : 'text-text-faint'} />
      </div>

      <div className="panel flex flex-col flex-1 min-h-0 overflow-hidden">
        <div className="p-4 border-b border-line flex flex-col md:flex-row gap-4 justify-between items-center">
          <div className="relative w-full md:w-96">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 text-text-faint" size={16} strokeWidth={1.75} />
            <input
              type="text"
              placeholder="Buscar máquina, IP ou usuário..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="input-base w-full pl-10"
            />
          </div>
        </div>

        <div className="flex-1 overflow-auto custom-scrollbar">
          <table className="table-base whitespace-nowrap">
            <thead className="sticky top-0 bg-ink-900 z-10">
              <tr>
                <th>Estado</th>
                <th>Máquina</th>
                <th>Unidade</th>
                <th>IP</th>
                <th>Usuário</th>
                <th className="text-right">CPU</th>
                <th className="text-right">Memória</th>
                <th className="text-right">Temp.</th>
                <th className="text-right">Ligada há</th>
                <th>Sistema</th>
              </tr>
            </thead>
            <tbody>
              {loading && (
                <tr><td colSpan={10} className="py-8 text-center text-text-faint">Carregando...</td></tr>
              )}
              {!loading && filtered.length === 0 && (
                <tr>
                  <td colSpan={10} className="py-8 text-center text-text-faint">
                    Nenhuma estação com agente. Instale o <code className="mono-data text-warn">cmd/agent</code> nas máquinas.
                  </td>
                </tr>
              )}
              {filtered.map(s => {
                const memPct = s.mem_total > 0 ? (s.mem_used / s.mem_total) * 100 : 0;
                return (
                  <tr
                    key={s.id}
                    onClick={() => openMachine(s.id)}
                    className="cursor-pointer"
                  >
                    <td>
                      <span className={`badge ${s.online ? 'badge-ok' : 'badge-muted'}`}>
                        <span className={`w-1.5 h-1.5 rounded-full ${s.online ? 'bg-ok animate-pulse' : 'bg-text-faint'}`} />
                        {s.online ? 'Online' : 'Offline'}
                      </span>
                    </td>
                    <td className="text-text-hi font-medium">{s.name}</td>
                    <td className="text-text-mut">{siteName(s.site_id)}</td>
                    <td className="mono-data text-xs text-text-mut selectable">{s.host_ip}</td>
                    <td className="text-text">
                      {s.last_user ? (
                        <span className="inline-flex items-center gap-1.5">
                          <User size={12} strokeWidth={1.75} className="text-text-faint" />
                          {s.last_user}
                        </span>
                      ) : <span className="text-text-faint">—</span>}
                    </td>
                    <td className={`text-right mono-data font-medium ${s.online ? usageColor(s.cpu) : 'text-text-faint'}`}>
                      {s.online ? `${s.cpu.toFixed(0)}%` : '—'}
                    </td>
                    <td className="text-right">
                      {s.online && s.mem_total > 0 ? (
                        <div className="flex flex-col items-end">
                          <span className={`mono-data font-medium ${usageColor(memPct)}`}>{memPct.toFixed(0)}%</span>
                          <span className="text-[10px] text-text-faint mono-data">
                            {formatGB(s.mem_used)}/{formatGB(s.mem_total)} GB
                          </span>
                        </div>
                      ) : <span className="text-text-faint">—</span>}
                    </td>
                    <td className="text-right">
                      {s.temperature_c !== null ? (
                        <span className={`inline-flex items-center gap-1 mono-data font-medium ${tempColor(s.temperature_c)}`}>
                          <Thermometer size={12} strokeWidth={1.75} />
                          {formatTemperature(s.temperature_c)}
                        </span>
                      ) : (
                        <span className="text-text-faint text-xs" title={NO_TEMPERATURE_HINT}>
                          {NO_TEMPERATURE}
                        </span>
                      )}
                    </td>
                    <td className="text-right text-text-mut text-xs mono-data">
                      {s.online ? formatUptime(s.uptime) : '—'}
                    </td>
                    <td className="text-text-mut text-xs">
                      {s.platform || s.os || '—'}
                      {s.agent_version && <span className="block text-[10px] text-text-faint">agente {s.agent_version}</span>}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>

        <div className="px-4 py-2 border-t border-line text-[11px] text-text-faint flex items-center gap-4">
          <span className="flex items-center gap-1.5"><RefreshCw size={11} strokeWidth={1.75} /> atualiza a cada {POLL_MS / 1000}s</span>
          <span className="flex items-center gap-1.5"><Cpu size={11} strokeWidth={1.75} /> alerta em {USAGE_WARN}%</span>
          <span className="flex items-center gap-1.5"><MemoryStick size={11} strokeWidth={1.75} /> crítico em {USAGE_CRITICAL}%</span>
          <span className="flex items-center gap-1.5"><Thermometer size={11} strokeWidth={1.75} /> quente acima de {TEMP_WARN}°C</span>
        </div>
      </div>
    </div>
  );
};

export default StationsView;
