import LoadNotice from './ui/LoadNotice';
import { useLoadStatus } from './ui/load-status';
import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  ArrowLeft, MonitorSmartphone, Network, Thermometer, ShieldAlert,
  MapPin, ChevronRight, Printer, HardDrive, Router, Monitor, ShieldQuestion,
} from 'lucide-react';
import {
  api,
  type AlertRuleRecord,
  type NetworkHostView,
  type ServerLiveStat,
  type Site,
} from '../lib/api';
import { formatPercent, relativeTime } from '../lib/format';
import { NO_TEMPERATURE_HINT, formatTemperature, isAbove } from '../lib/metrics';
import { useNavigation } from './ui/navigation-context';
import { POLL } from '../lib/polling';


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
  <div className="stat-card">
    <div className="flex items-center justify-between mb-1.5">
      <span className={`stat-value ${accent}`}>{value}</span>
      <Icon size={16} strokeWidth={1.75} className="text-text-faint" />
    </div>
    <div className="eyebrow">{label}</div>
  </div>
);

const SiteDetailView = ({ siteId }: SiteDetailViewProps) => {
  const { openMachine, goBack } = useNavigation();

  const [site, setSite] = useState<Site | null>(null);
  const [stations, setStations] = useState<ServerLiveStat[]>([]);
  const [hosts, setHosts] = useState<NetworkHostView[]>([]);
  const [rules, setRules] = useState<AlertRuleRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const carga = useLoadStatus();
  const { ok: cargaOk, fail: cargaFail } = carga;
  const cadastro = useLoadStatus();
  const { ok: cadastroOk, fail: cadastroFail } = cadastro;
  const regras = useLoadStatus();
  const { ok: regrasOk, fail: regrasFail } = regras;

  const load = useCallback(async (signal?: AbortSignal) => {
    try {
      const [live, inventory] = await Promise.all([
        api.liveMetrics(signal),
        api.networkHosts(signal),
      ]);
      setStations(live.servers.filter((s) => s.site_id === siteId));
      setHosts(inventory.hosts.filter((h) => h.site_id === siteId));
      cargaOk();
    } catch (err) {
      if (!signal?.aborted) cargaFail(err, 'Falha ao ler as máquinas da unidade.');
    } finally {
      setLoading(false);
    }
  }, [siteId, cargaOk, cargaFail]);

  useEffect(() => {
    let active = true;
    api.sites()
      .then((list) => {
        if (!active) return;
        setSite(list.find((s) => s.id === siteId) ?? null);
        cadastroOk();
      })
      .catch((err) => { if (active) cadastroFail(err, 'Falha ao ler o cadastro da unidade.'); });
    api.alertRules()
      .then((list) => {
        if (!active) return;
        setRules(list);
        regrasOk();
      })
      .catch((err) => { if (active) regrasFail(err, 'Falha ao ler as regras de alerta.'); });
    return () => {
      active = false;
    };
  }, [siteId, cadastroOk, cadastroFail, regrasOk, regrasFail]);

  useEffect(() => {
    const controller = new AbortController();
    load(controller.signal);
    const interval = setInterval(() => load(controller.signal), POLL.unidade);
    return () => {
      clearInterval(interval);
      controller.abort();
    };
  }, [load]);

  const online = stations.filter((s) => s.online).length;
  const hot = stations.filter((s) => isAbove(s.temperature_c, TEMP_WARN)).length;
  const withoutAgent = hosts.filter((h) => !h.monitored).length;

  const siteRules = useMemo(() => {
    const ids = new Set(stations.map((s) => s.id));
    return rules.filter(
      (r) => r.target === '*' || r.target_site_id === siteId || ids.has(r.target),
    );
  }, [rules, stations, siteId]);

  if (loading && !site) {
    return <div className="p-8 text-sm text-text-mut">Carregando unidade...</div>;
  }

  return (
    <div className="p-4 md:p-8 anim-rise">
      <button onClick={goBack} className="btn btn-ghost min-h-0 h-8 px-2.5 mb-4 text-text-mut">
        <ArrowLeft size={14} strokeWidth={1.75} />
        Voltar
      </button>

      <LoadNotice error={carga.error} lastOk={carga.lastOk} className="mb-4" />
      <LoadNotice error={cadastro.error} lastOk={cadastro.lastOk} className="mb-4" />
      <div className="page-header flex-col md:flex-row items-start md:items-end">
        <div>
          <h1 className="page-title">{site ? site.name : 'Unidade'}</h1>
          <p className="page-desc flex items-center gap-2">
            {site && (
              <code className="mono-data text-accent bg-ink-850 border border-line rounded px-1.5 py-0.5 text-[11px]">
                {site.code}
              </code>
            )}
            {site?.address && (
              <span className="inline-flex items-center gap-1">
                <MapPin size={12} strokeWidth={1.75} /> {site.address}
              </span>
            )}
          </p>
        </div>
      </div>

      <div className="grid grid-cols-2 md:grid-cols-4 gap-3 mb-6 stagger">
        <Stat label="Máquinas com agente" value={stations.length} accent="text-text-hi" Icon={MonitorSmartphone} />
        <Stat label="Online agora" value={online} accent="text-ok" Icon={MonitorSmartphone} />
        <Stat
          label="Vistas sem agente"
          value={withoutAgent}
          accent={withoutAgent > 0 ? 'text-warn' : 'text-text-faint'}
          Icon={Network}
        />
        <Stat
          label="Temperatura alta"
          value={hot}
          accent={hot > 0 ? 'text-crit' : 'text-text-faint'}
          Icon={Thermometer}
        />
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
        <div className="lg:col-span-2 panel overflow-hidden">
          <div className="p-4 border-b border-line bg-ink-850">
            <h2 className="eyebrow">Máquinas monitoradas</h2>
          </div>
          <div className="overflow-x-auto custom-scrollbar">
            <table className="table-base whitespace-nowrap">
              <thead>
                <tr>
                  <th>Estado</th>
                  <th>Máquina</th>
                  <th>Usuário</th>
                  <th className="text-right">CPU</th>
                  <th className="text-right">Memória</th>
                  <th className="text-right">Temp.</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {stations.length === 0 && (
                  <tr>
                    <td colSpan={7} className="py-8 text-center text-text-mut">
                      {carga.error && !carga.lastOk ? carga.error : 'Nenhuma máquina com agente nesta unidade.'}
                    </td>
                  </tr>
                )}
                {stations.map((s) => {
                  const memPct = s.mem_total > 0 ? (s.mem_used / s.mem_total) * 100 : 0;
                  return (
                    <tr
                      key={s.id}
                      onClick={() => openMachine(s.id)}
                      className="cursor-pointer"
                    >
                      <td>
                        <span
                          className={`w-2 h-2 rounded-full inline-block ${s.online ? 'bg-ok animate-pulse' : 'bg-text-faint'}`}
                          title={s.online ? 'Online' : 'Offline'}
                        />
                      </td>
                      <td className="text-text-hi font-medium">{s.name}</td>
                      <td className="text-text-mut">{s.last_user || '—'}</td>
                      <td className={`text-right mono-data ${s.cpu !== null && s.cpu >= USAGE_WARN ? 'text-warn' : 'text-text-hi'}`}>
                        {s.online ? formatPercent(s.cpu) : '—'}
                      </td>
                      <td className={`text-right mono-data ${memPct >= USAGE_WARN ? 'text-warn' : 'text-text-hi'}`}>
                        {s.online && s.mem_total > 0 ? `${memPct.toFixed(0)}%` : '—'}
                      </td>
                      <td
                        className={`text-right mono-data ${
                          s.temperature_c === null
                            ? 'text-text-faint text-xs'
                            : isAbove(s.temperature_c, TEMP_WARN)
                              ? 'text-crit'
                              : 'text-ok'
                        }`}
                        title={s.temperature_c === null ? NO_TEMPERATURE_HINT : undefined}
                      >
                        {formatTemperature(s.temperature_c)}
                      </td>
                      <td className="text-right">
                        <ChevronRight size={14} strokeWidth={1.75} className="text-text-faint inline" />
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>

        <div className="flex flex-col gap-4">
          <div className="panel overflow-hidden">
            <div className="p-4 border-b border-line bg-ink-850">
              <h2 className="eyebrow">Vistos na rede sem agente</h2>
            </div>
            <div className="p-2 max-h-64 overflow-y-auto custom-scrollbar">
              {withoutAgent === 0 ? (
                <p className="text-xs text-text-mut p-2">
                  Todo equipamento inventariado desta unidade já reporta métricas.
                </p>
              ) : (
                hosts
                  .filter((h) => !h.monitored)
                  .map((h) => {
                    const Icon = DEVICE_ICONS[h.device_type] ?? ShieldQuestion;
                    return (
                      <div key={h.ip} className="flex items-center gap-2 px-2 py-1.5 text-xs">
                        <Icon size={13} strokeWidth={1.75} className="text-text-faint shrink-0" />
                        <span className="mono-data text-text selectable">{h.ip}</span>
                        <span className="text-text-faint truncate">{h.hostname}</span>
                        <span className="ml-auto text-[11px] text-text-faint shrink-0">
                          {relativeTime(h.last_seen)}
                        </span>
                      </div>
                    );
                  })
              )}
            </div>
          </div>

          <div className="panel overflow-hidden">
            <div className="p-4 border-b border-line bg-ink-850">
              <h2 className="eyebrow flex items-center gap-2">
                <ShieldAlert size={12} strokeWidth={1.75} />
                Regras que cobrem a unidade
              </h2>
            </div>
            <div className="p-2 max-h-64 overflow-y-auto custom-scrollbar">
              {siteRules.length === 0 ? (
                regras.error ? <LoadNotice error={regras.error} className="p-2" /> : <p className="text-xs text-text-mut p-2">Nenhuma regra de alerta cobre esta unidade.</p>
              ) : (
                siteRules.map((r) => (
                  <div key={r.id} className="flex items-center gap-2 px-2 py-1.5 text-xs">
                    <span
                      className={`w-1.5 h-1.5 rounded-full shrink-0 ${r.enabled ? 'bg-ok' : 'bg-text-faint'}`}
                      title={r.enabled ? 'Regra ativa' : 'Regra desativada'}
                    />
                    <span className="text-text truncate">{r.name}</span>
                    <span className="ml-auto text-[11px] text-text-faint mono-data shrink-0">
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
