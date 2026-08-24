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

// Severidade referencia os tokens semânticos do tema — o SVG aceita var().
const sevColor = (s: UpstreamAgg['severity']) =>
  s === 'error' ? 'var(--color-crit)' : s === 'warn' ? 'var(--color-warn)' : 'var(--color-ok)';

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
        <circle cx={lbX} cy={lbY} r="34" fill="var(--color-accent)" fillOpacity="0.08" stroke="var(--color-accent)" strokeOpacity="0.4" />
        <circle cx={lbX} cy={lbY} r="34" fill="none" stroke="var(--color-accent)" strokeOpacity="0.25">
          <animate attributeName="r" values="34;46;34" dur="2.5s" repeatCount="indefinite" />
          <animate attributeName="stroke-opacity" values="0.35;0;0.35" dur="2.5s" repeatCount="indefinite" />
        </circle>
        <text x={lbX} y={lbY - 44} textAnchor="middle" fill="var(--color-text-faint)" fontSize="11" fontWeight="600" letterSpacing="1">LOAD BALANCER</text>
        <text x={lbX} y={lbY + 5} textAnchor="middle" fill="var(--color-text-hi)" fontSize="10" fontFamily="var(--font-mono)">{totalReqs} req/5s</text>
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
            <text x={upX + 16} y={y - 4} fill="var(--color-text-hi)" fontSize="10" fontFamily="var(--font-mono)">{u.addr}</text>
            <text x={upX + 16} y={y + 9} fill="var(--color-text-mut)" fontSize="9">{u.reqs} reqs · {u.severity === 'ok' ? '200' : u.severity}</text>
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
    <div className="p-4 md:p-8 h-full flex flex-col overflow-hidden anim-rise">
      <div className="page-header flex-col md:flex-row items-start md:items-end">
        <div>
          <h1 className="page-title">Nginx e tráfego</h1>
          <p className="page-desc">Monitoramento de tráfego de rede e roteamento reverso do Load Balancer.</p>
        </div>
        <span className="badge badge-muted" title="Esta tela não executa ação nenhuma no Nginx">somente leitura</span>
      </div>

      {/* Fluxo de requisições em tempo real: LB para os upstreams */}
      <div className="panel mb-6 p-4">
        <div className="flex items-center gap-2 mb-2">
          <Server size={16} strokeWidth={1.75} className="text-text-faint" />
          <h2 className="eyebrow">Fluxo de roteamento ao vivo</h2>
        </div>
        <div className="h-[260px]">
          {upstreams.length === 0 ? (
            <div className="h-full flex items-center justify-center text-text-mut text-sm">
              Aguardando tráfego no Load Balancer...
            </div>
          ) : (
            <TrafficFlow upstreams={upstreams} />
          )}
        </div>
      </div>

      <div className="panel flex flex-col flex-1 min-h-0 overflow-hidden">
        {/* Toolbar */}
        <div className="p-4 border-b border-line bg-ink-850 flex items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <Network size={18} strokeWidth={1.75} className="text-text-faint" />
            <h2 className="text-text-hi font-semibold text-sm">Virtual hosts (Nginx)</h2>
          </div>
          <span className="badge badge-ok">
            <span className="w-1.5 h-1.5 rounded-full bg-ok animate-pulse"></span>
            ao vivo
          </span>
        </div>

        {/* Tabela */}
        <div className="flex-1 overflow-auto custom-scrollbar p-4">
          <table className="table-base whitespace-nowrap">
            <thead>
              <tr>
                <th>Domínio / host</th>
                <th>Upstream (proxy pass)</th>
                <th>Requisições (5s)</th>
                <th>Saúde</th>
              </tr>
            </thead>
            <tbody>
              {loadBalancing.length === 0 && (
                <tr>
                  <td colSpan={4} className="py-8 text-center text-text-mut">
                    Aguardando tráfego ou nenhum log do Nginx encontrado.
                  </td>
                </tr>
              )}
              {loadBalancing.map((host) => {
                const isError = ERROR_STATUSES.includes(host.status);
                const isWarn = WARN_STATUSES.includes(host.status);

                return (
                  <tr key={`${host.server_name}-${host.upstream_addr}-${host.status}`}>
                    <td>
                      <div className="flex items-center gap-2">
                        <Globe size={14} strokeWidth={1.75} className={isError ? 'text-crit' : 'text-text-faint'} />
                        <span className="mono-data text-text-hi">{host.server_name || 'Desconhecido'}</span>
                      </div>
                    </td>
                    <td className="mono-data text-text-mut">{host.upstream_addr}</td>
                    <td>
                      <span className="badge badge-muted mono-data">{host.requests_count} reqs</span>
                    </td>
                    <td>
                      {isError ? (
                        <span className="badge badge-crit">Erro {host.status}</span>
                      ) : isWarn ? (
                        <span className="badge badge-warn">Aviso {host.status}</span>
                      ) : (
                        <span className="badge badge-ok">Saudável 200</span>
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
