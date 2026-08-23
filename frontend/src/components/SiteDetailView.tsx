import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  ArrowLeft, Building2, MonitorSmartphone, Network, Thermometer, ShieldAlert,
  MapPin, ChevronRight, Printer, HardDrive, Router, Monitor, ShieldQuestion,
} from 'lucide-react';
import {
  api,
  type AlertRuleRecord,
  type NetworkHostView,
  type ServerLiveStat,
  type Site,
} from '../lib/api';
import { relativeTime } from '../lib/format';
import { NO_TEMPERATURE_HINT, formatTemperature, isAbove } from '../lib/metrics';
import { useNavigation } from './ui/navigation-context';

const POLL_MS = 15000;

// Mesmos limiares da tela de Estações: acima de 70 °C costuma ser ventilação
// obstruída, e a partir de 75% de uso a máquina já incomoda o usuário.
const TEMP_WARN = 70;
const USAGE_WARN = 75;

const DEVICE_ICONS: Record<string, typeof Monitor> = {
  printer: Printer,
  windows: Monitor,
  nas: HardDrive,
  linux: Router,
};

interface SiteDetailViewProps {
  siteId: number;
}

const Stat = ({
  label,
  value,
  accent,
  Icon,
}: {
  label: string;
  value: number | string;
  accent: string;
  Icon: typeof Monitor;
}) => (
  <div className="glass-panel rounded-xl p-4 border border-white/5 bg-white/[0.02]">
    <div className="flex items-center justify-between mb-1">
      <span className={`text-3xl font-bold ${accent}`}>{value}</span>
      <Icon size={16} className="text-[#737373]" />
    </div>
    <div className="text-xs text-[#737373] uppercase tracking-widest">{label}</div>
  </div>
);

/**
 * Resumo de uma unidade: o nível entre a lista de unidades e a máquina.
 *
 * Reúne numa tela o que o suporte precisa saber ao atender uma filial —
 * quantas máquinas respondem, quais estão sob pressão, o que a varredura
 * encontrou sem agente e quais regras de alerta cobrem o lugar.
 */
const SiteDetailView = ({ siteId }: SiteDetailViewProps) => {
  const { openMachine, goBack } = useNavigation();

  const [site, setSite] = useState<Site | null>(null);
  const [stations, setStations] = useState<ServerLiveStat[]>([]);
  const [hosts, setHosts] = useState<NetworkHostView[]>([]);
  const [rules, setRules] = useState<AlertRuleRecord[]>([]);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async (signal?: AbortSignal) => {
    try {
      const [live, inventory] = await Promise.all([
        api.liveMetrics(signal),
        api.networkHosts(signal),
      ]);
      setStations(live.servers.filter((s) => s.site_id === siteId));
      setHosts(inventory.hosts.filter((h) => h.site_id === siteId));
    } catch (err) {
      if (!signal?.aborted) console.error(err);
    } finally {
      setLoading(false);
    }
  }, [siteId]);

  // Unidade e regras mudam raramente: buscadas uma vez, fora do polling.
  useEffect(() => {
    let active = true;
    api.sites()
      .then((list) => active && setSite(list.find((s) => s.id === siteId) ?? null))
      .catch(() => {});
    api.alertRules()
      .then((list) => active && setRules(list))
      .catch(() => {});
    return () => {
      active = false;
    };
  }, [siteId]);

  useEffect(() => {
    const controller = new AbortController();
    load(controller.signal);
    const interval = setInterval(() => load(controller.signal), POLL_MS);
    return () => {
      clearInterval(interval);
      controller.abort();
    };
  }, [load]);

  const online = stations.filter((s) => s.online).length;
  // Sem sensor não conta como quente nem como fria: é ausência de leitura.
  const hot = stations.filter((s) => isAbove(s.temperature_c, TEMP_WARN)).length;
  const withoutAgent = hosts.filter((h) => !h.monitored).length;

  // Regras que cobrem esta unidade: as globais (target "*") e as que apontam
  // para ela ou para uma máquina dela.
  const siteRules = useMemo(() => {
    const ids = new Set(stations.map((s) => s.id));
    return rules.filter(
      (r) => r.target === '*' || r.target_site_id === siteId || ids.has(r.target),
    );
  }, [rules, stations, siteId]);

  if (loading && !site) {
    return <div className="p-8 text-sm text-[#737373]">Carregando unidade...</div>;
  }

  return (
    <div className="p-4 md:p-8">
      <button
        onClick={goBack}
        className="flex items-center gap-2 text-xs font-bold uppercase tracking-widest text-[#737373] hover:text-white transition-colors mb-4"
      >
        <ArrowLeft size={14} />
        Voltar
      </button>

      <div className="mb-6 flex flex-col md:flex-row md:items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-light text-white mb-2 flex items-center gap-3">
            <Building2 className="text-[#10b981]" />
            {site ? <span className="font-bold">{site.name}</span> : 'Unidade'}
          </h1>
          <p className="text-[#737373] text-sm flex items-center gap-2">
            {site && (
              <code className="text-amber-300 bg-black/30 px-2 py-0.5 rounded border border-white/5">
                {site.code}
              </code>
            )}
            {site?.address && <span className="flex items-center gap-1"><MapPin size={12} /> {site.address}</span>}
          </p>
        </div>
      </div>

      <div className="grid grid-cols-2 md:grid-cols-4 gap-3 mb-6">
        <Stat label="Máquinas com agente" value={stations.length} accent="text-white" Icon={MonitorSmartphone} />
        <Stat label="Online agora" value={online} accent="text-emerald-400" Icon={MonitorSmartphone} />
        <Stat
          label="Vistas sem agente"
          value={withoutAgent}
          accent={withoutAgent > 0 ? 'text-amber-400' : 'text-[#737373]'}
          Icon={Network}
        />
        <Stat
          label="Temperatura alta"
          value={hot}
          accent={hot > 0 ? 'text-rose-400' : 'text-[#737373]'}
          Icon={Thermometer}
        />
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
        <div className="lg:col-span-2 glass-panel rounded-xl border border-white/5 bg-white/[0.02] overflow-hidden">
          <div className="p-4 border-b border-white/5 bg-black/20">
            <h2 className="text-xs font-bold tracking-widest text-[#737373] uppercase">
              Máquinas monitoradas
            </h2>
          </div>
          <div className="overflow-x-auto custom-scrollbar">
            <table className="w-full text-left text-sm whitespace-nowrap">
              <thead className="bg-[#0c0c0e]/80">
                <tr>
                  <th className="py-3 px-4 font-medium text-gray-500">Estado</th>
                  <th className="py-3 px-4 font-medium text-gray-500">Máquina</th>
                  <th className="py-3 px-4 font-medium text-gray-500">Usuário</th>
                  <th className="py-3 px-4 font-medium text-gray-500 text-right">CPU</th>
                  <th className="py-3 px-4 font-medium text-gray-500 text-right">Memória</th>
                  <th className="py-3 px-4 font-medium text-gray-500 text-right">Temp.</th>
                  <th className="py-3 px-4"></th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/5">
                {stations.length === 0 && (
                  <tr>
                    <td colSpan={7} className="py-8 text-center text-gray-500">
                      Nenhuma máquina com agente nesta unidade.
                    </td>
                  </tr>
                )}
                {stations.map((s) => {
                  const memPct = s.mem_total > 0 ? (s.mem_used / s.mem_total) * 100 : 0;
                  return (
                    <tr
                      key={s.id}
                      onClick={() => openMachine(s.id)}
                      className="hover:bg-white/[0.04] transition-colors cursor-pointer"
                    >
                      <td className="py-3 px-4">
                        <span className={`w-2 h-2 rounded-full inline-block ${s.online ? 'bg-[#10b981] animate-pulse' : 'bg-[#737373]'}`} />
                      </td>
                      <td className="py-3 px-4 text-gray-200 font-medium">{s.name}</td>
                      <td className="py-3 px-4 text-gray-400">{s.last_user || '—'}</td>
                      <td className={`py-3 px-4 text-right ${s.cpu >= USAGE_WARN ? 'text-amber-400' : 'text-white/90'}`}>
                        {s.online ? `${s.cpu.toFixed(0)}%` : '—'}
                      </td>
                      <td className={`py-3 px-4 text-right ${memPct >= USAGE_WARN ? 'text-amber-400' : 'text-white/90'}`}>
                        {s.online && s.mem_total > 0 ? `${memPct.toFixed(0)}%` : '—'}
                      </td>
                      <td
                        className={`py-3 px-4 text-right ${
                          s.temperature_c === null
                            ? 'text-gray-600 text-xs'
                            : isAbove(s.temperature_c, TEMP_WARN)
                              ? 'text-rose-400'
                              : 'text-emerald-400'
                        }`}
                        title={s.temperature_c === null ? NO_TEMPERATURE_HINT : undefined}
                      >
                        {formatTemperature(s.temperature_c)}
                      </td>
                      <td className="py-3 px-4 text-right">
                        <ChevronRight size={14} className="text-[#737373] inline" />
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>

        <div className="flex flex-col gap-4">
          <div className="glass-panel rounded-xl border border-white/5 bg-white/[0.02] overflow-hidden">
            <div className="p-4 border-b border-white/5 bg-black/20">
              <h2 className="text-xs font-bold tracking-widest text-[#737373] uppercase">
                Vistos na rede sem agente
              </h2>
            </div>
            <div className="p-2 max-h-64 overflow-y-auto custom-scrollbar">
              {withoutAgent === 0 ? (
                <p className="text-xs text-[#737373] p-2">
                  Todo equipamento inventariado desta unidade já reporta métricas.
                </p>
              ) : (
                hosts
                  .filter((h) => !h.monitored)
                  .map((h) => {
                    const Icon = DEVICE_ICONS[h.device_type] ?? ShieldQuestion;
                    return (
                      <div key={h.ip} className="flex items-center gap-2 px-2 py-1.5 text-xs">
                        <Icon size={13} className="text-[#737373] shrink-0" />
                        <span className="font-mono text-gray-300 selectable">{h.ip}</span>
                        <span className="text-[#737373] truncate">{h.hostname}</span>
                        <span className="ml-auto text-[10px] text-[#737373] shrink-0">
                          {relativeTime(h.last_seen)}
                        </span>
                      </div>
                    );
                  })
              )}
            </div>
          </div>

          <div className="glass-panel rounded-xl border border-white/5 bg-white/[0.02] overflow-hidden">
            <div className="p-4 border-b border-white/5 bg-black/20">
              <h2 className="text-xs font-bold tracking-widest text-[#737373] uppercase flex items-center gap-2">
                <ShieldAlert size={12} />
                Regras que cobrem a unidade
              </h2>
            </div>
            <div className="p-2 max-h-64 overflow-y-auto custom-scrollbar">
              {siteRules.length === 0 ? (
                <p className="text-xs text-[#737373] p-2">Nenhuma regra de alerta cobre esta unidade.</p>
              ) : (
                siteRules.map((r) => (
                  <div key={r.id} className="flex items-center gap-2 px-2 py-1.5 text-xs">
                    <span
                      className={`w-1.5 h-1.5 rounded-full shrink-0 ${r.enabled ? 'bg-[#10b981]' : 'bg-[#737373]'}`}
                    />
                    <span className="text-gray-300 truncate">{r.name}</span>
                    <span className="ml-auto text-[10px] text-[#737373] font-mono shrink-0">
                      {r.metric} {r.operator} {r.threshold}
                    </span>
                  </div>
                ))
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};

export default SiteDetailView;
