import { useEffect, useState, useMemo } from 'react';
import { Globe, Server, Network } from 'lucide-react';
import { api, type LbStat } from '../lib/api';

// Agrega as linhas do LB por upstream para desenhar o diagrama de fluxo.
interface UpstreamAgg {
  addr: string;
  reqs: number;
  severity: 'ok' | 'warn' | 'error';
}

const ERROR_STATUSES = ['500', '502', '503', '504'];
const WARN_STATUSES = ['400', '404', '429'];

const aggregateUpstreams = (rows: LbStat[]): UpstreamAgg[] => {
  const map: Record<string, UpstreamAgg> = {};
  for (const r of rows) {
    const key = r.upstream_addr || 'Local (Nginx/Cache)';
    if (!map[key]) map[key] = { addr: key, reqs: 0, severity: 'ok' };
    map[key].reqs += r.requests_count;
    if (ERROR_STATUSES.includes(r.status)) map[key].severity = 'error';
    else if (map[key].severity !== 'error' && WARN_STATUSES.includes(r.status)) map[key].severity = 'warn';
  }
  return Object.values(map).sort((a, b) => b.reqs - a.reqs).slice(0, 6);
};

const sevColor = (s: UpstreamAgg['severity']) =>
  s === 'error' ? '#f43f5e' : s === 'warn' ? '#f59e0b' : '#10b981';

// Diagrama SVG: Load Balancer à esquerda, upstreams à direita, com "pacotes"
// animados cuja quantidade e velocidade refletem as reqs/5s de cada upstream.
const TrafficFlow = ({ upstreams }: { upstreams: UpstreamAgg[] }) => {
  const W = 640, H = 260;
  const lbX = 80, lbY = H / 2;
  const upX = W - 120;
  const n = Math.max(upstreams.length, 1);
  const totalReqs = upstreams.reduce((s, u) => s + u.reqs, 0);

  return (
    <svg viewBox={`0 0 ${W} ${H}`} className="w-full h-full" preserveAspectRatio="xMidYMid meet">
      {/* nó do Load Balancer */}
      <g>
        <circle cx={lbX} cy={lbY} r="34" fill="#10b981" fillOpacity="0.08" stroke="#10b981" strokeOpacity="0.4" />
        <circle cx={lbX} cy={lbY} r="34" fill="none" stroke="#10b981" strokeOpacity="0.25">
          <animate attributeName="r" values="34;46;34" dur="2.5s" repeatCount="indefinite" />
          <animate attributeName="stroke-opacity" values="0.35;0;0.35" dur="2.5s" repeatCount="indefinite" />
        </circle>
        <text x={lbX} y={lbY - 44} textAnchor="middle" fill="#10b981" fontSize="11" fontWeight="bold" letterSpacing="1">LOAD BALANCER</text>
        <text x={lbX} y={lbY + 5} textAnchor="middle" fill="#e5e7eb" fontSize="10">{totalReqs} req/5s</text>
      </g>

      {upstreams.map((u, i) => {
        const y = n === 1 ? lbY : 40 + (i * (H - 80)) / (n - 1);
        const color = sevColor(u.severity);
        const pathId = `flow-${i}`;
        const d = `M ${lbX + 34} ${lbY} C ${(lbX + upX) / 2} ${lbY}, ${(lbX + upX) / 2} ${y}, ${upX - 12} ${y}`;
        // mais reqs = mais pacotes e mais rápidos
        const dots = Math.min(1 + Math.floor(u.reqs / 3), 6);
        const dur = Math.max(0.7, 2.6 - u.reqs * 0.04);
        return (
          <g key={u.addr}>
            <path id={pathId} d={d} fill="none" stroke={color} strokeOpacity="0.18" strokeWidth="2" />
            {Array.from({ length: dots }).map((_, k) => (
              <circle key={k} r="3.5" fill={color}>
                <animateMotion dur={`${dur}s`} begin={`${(k * dur) / dots}s`} repeatCount="indefinite">
                  <mpath href={`#${pathId}`} />
                </animateMotion>
              </circle>
            ))}
            {/* nó do upstream */}
            <circle cx={upX} cy={y} r="9" fill={color} fillOpacity="0.15" stroke={color} strokeOpacity="0.6" />
            <circle cx={upX} cy={y} r="3.5" fill={color} />
            <text x={upX + 16} y={y - 4} fill="#e5e7eb" fontSize="10" fontFamily="monospace">{u.addr}</text>
            <text x={upX + 16} y={y + 9} fill="#737373" fontSize="9">{u.reqs} reqs · {u.severity === 'ok' ? '200' : u.severity}</text>
          </g>
        );
      })}
    </svg>
  );
};

const NginxView = () => {
  const [loadBalancing, setLoadBalancing] = useState<LbStat[]>([]);
  const upstreams = useMemo(() => aggregateUpstreams(loadBalancing), [loadBalancing]);

  useEffect(() => {
    const controller = new AbortController();
    const fetchMetrics = () => {
      api.liveMetrics(controller.signal)
        .then(data => setLoadBalancing(data.load_balancing))
        .catch(err => {
          if (!controller.signal.aborted) console.error('Erro API Nginx:', err);
        });
    };

    fetchMetrics();
    const interval = setInterval(fetchMetrics, 2000);
    return () => {
      clearInterval(interval);
      controller.abort();
    };
  }, []);

  return (
    <div className="p-4 md:p-8 h-full flex flex-col overflow-hidden">
      <div className="mb-6 flex flex-col md:flex-row md:items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-light text-white mb-2 flex items-center gap-3">
            <Globe className="text-[#10b981]" /> Nginx & <span className="font-bold">Tráfego (Ao Vivo)</span>
          </h1>
          <p className="text-[#737373] text-sm">Monitoramento de tráfego de rede e roteamento reverso.</p>
        </div>
      </div>

      {/* Fluxo de requisições em tempo real: LB para os upstreams */}
      <div className="glass-panel rounded-xl border border-white/5 bg-white/[0.02] mb-6 p-4">
        <div className="flex items-center gap-2 mb-2">
          <Server className="text-[#10b981]" size={16} />
          <h2 className="text-white font-medium text-sm">Fluxo de Roteamento (Ao Vivo)</h2>
        </div>
        <div className="h-[260px]">
          {upstreams.length === 0 ? (
            <div className="h-full flex items-center justify-center text-gray-500 text-sm">
              Aguardando tráfego no Load Balancer...
            </div>
          ) : (
            <TrafficFlow upstreams={upstreams} />
          )}
        </div>
      </div>

      <div className="glass-panel rounded-xl border border-white/5 bg-white/[0.02] flex flex-col flex-1 min-h-0 overflow-hidden">
        {/* Toolbar */}
        <div className="p-4 border-b border-white/5 bg-black/20 flex items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <Network className="text-[#10b981]" size={18} />
            <h2 className="text-white font-medium">Virtual Hosts (Nginx)</h2>
          </div>
          <div className="flex items-center gap-2">
            <span className="relative flex h-3 w-3">
              <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
              <span className="relative inline-flex rounded-full h-3 w-3 bg-emerald-500"></span>
            </span>
            <span className="text-xs text-emerald-400 uppercase tracking-wider font-medium">Conectado (Live Sync)</span>
          </div>
        </div>

        {/* Table */}
        <div className="flex-1 overflow-auto custom-scrollbar p-4">
          <table className="w-full text-left text-sm whitespace-nowrap">
            <thead className="text-gray-500">
              <tr>
                <th className="py-2 px-4 font-medium">Domínio / Host</th>
                <th className="py-2 px-4 font-medium">Upstream (Proxy Pass)</th>
                <th className="py-2 px-4 font-medium">Requisições (5s)</th>
                <th className="py-2 px-4 font-medium">Status Health</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-white/5">
              {loadBalancing.length === 0 && (
                <tr>
                  <td colSpan={4} className="py-8 text-center text-gray-500">
                    Aguardando tráfego ou nenhum log do Nginx encontrado.
                  </td>
                </tr>
              )}
              {loadBalancing.map((host) => {
                const isError = ERROR_STATUSES.includes(host.status);
                const isWarn = WARN_STATUSES.includes(host.status);

                return (
                  <tr
                    key={`${host.server_name}-${host.upstream_addr}-${host.status}`}
                    className="hover:bg-white/[0.02] transition-colors"
                  >
                    <td className="py-3 px-4">
                      <div className="flex items-center gap-2">
                        <Globe size={14} className={isError ? 'text-rose-500' : 'text-[#10b981]'} />
                        <span className="text-gray-200 font-medium">{host.server_name || "Desconhecido"}</span>
                      </div>
                    </td>
                    <td className="py-3 px-4 font-mono text-gray-400">{host.upstream_addr}</td>
                    <td className="py-3 px-4">
                      <span className="text-gray-300 bg-white/5 px-2 py-1 rounded text-xs">{host.requests_count} reqs</span>
                    </td>
                    <td className="py-3 px-4">
                      {isError ? (
                        <span className="text-rose-400 text-xs bg-rose-400/10 px-2 py-1 rounded border border-rose-400/20">Erro {host.status}</span>
                      ) : isWarn ? (
                        <span className="text-amber-400 text-xs bg-amber-400/10 px-2 py-1 rounded border border-amber-400/20">Aviso {host.status}</span>
                      ) : (
                        <span className="text-emerald-400 text-xs bg-emerald-400/10 px-2 py-1 rounded border border-emerald-400/20">Saudável 200</span>
                      )}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
};

export default NginxView;
