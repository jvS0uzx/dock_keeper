import LoadNotice from './ui/LoadNotice';
import { useLoadStatus } from './ui/load-status';
import { useCallback, useEffect, useState, useMemo } from 'react';
import { Globe, Server, Network, Link2 } from 'lucide-react';
import {
  api,
  apiErrorMessage,
  type LbStat,
  type NginxUpstreamLink,
  type ServerLiveStat,
  type ServerRecord,
} from '../lib/api';
import {
  arestasDaMalha,
  balanceadoresDaMalha,
  deriveUpstreams,
  destinosDaMalha,
  rotuloDaJanela,
  upstreamsSemCadastro,
  type BalanceadorDaMalha,
  type SeveridadeDoDestino,
} from '../lib/upstream';
import { hasGlobalAdmin } from '../lib/panels';
import { useDialog } from './ui/dialog-context';
import { useSession } from './ui/session-context';
import Select from './ui/Select';
import { POLL } from '../lib/polling';

const ERROR_STATUSES = ['500', '502', '503', '504'];
const WARN_STATUSES = ['400', '404', '429'];

const corDaSeveridade = (s: SeveridadeDoDestino) =>
  s === 'error' ? 'var(--color-crit)' : s === 'warn' ? 'var(--color-warn)' : 'var(--color-ok)';

const LARGURA = 640;

const posicao = (indice: number, total: number, altura: number): number =>
  total <= 1 ? altura / 2 : 44 + (indice * (altura - 88)) / (total - 1);

const MalhaDeTrafego = ({
  balanceadores,
  janela,
}: {
  balanceadores: BalanceadorDaMalha[];
  janela: string;
}) => {
  const destinos = destinosDaMalha(balanceadores);
  const altura = Math.max(260, 60 + Math.max(destinos.length, balanceadores.length) * 46);
  const lbX = 86;
  const destX = LARGURA - 150;
  const yDoDestino = new Map(destinos.map((d, i) => [d.destino, posicao(i, destinos.length, altura)]));

  return (
    <svg
      viewBox={`0 0 ${LARGURA} ${altura}`}
      className="w-full h-full"
      preserveAspectRatio="xMidYMid meet"
      role="img"
      aria-label="Malha de roteamento do Nginx"
    >
      {balanceadores.map((balanceador, iLb) => {
        const yLb = posicao(iLb, balanceadores.length, altura);
        const corDoNo = balanceador.potencial ? 'var(--color-text-faint)' : 'var(--color-accent)';

        return (
          <g key={balanceador.id || balanceador.nome}>
            {balanceador.arestas.map((aresta, iAresta) => {
              const yDestino = yDoDestino.get(aresta.destino) ?? altura / 2;
              const cor = aresta.potencial ? 'var(--color-text-faint)' : corDaSeveridade(aresta.severidade);
              const caminho = `malha-${iLb}-${iAresta}`;
              const d = `M ${lbX + 32} ${yLb} C ${(lbX + destX) / 2} ${yLb}, ${(lbX + destX) / 2} ${yDestino}, ${destX - 12} ${yDestino}`;
              const anima = !aresta.potencial && aresta.reqs > 0;
              const pontos = anima ? Math.min(1 + Math.floor(aresta.reqs / 3), 6) : 0;
              const duracao = Math.max(0.7, 2.6 - aresta.reqs * 0.04);

              return (
                <g key={`${aresta.bloco}-${aresta.destino}`}>
                  <path
                    id={caminho}
                    data-testid={aresta.potencial ? 'aresta-potencial' : anima ? 'aresta-com-trafego' : 'aresta-parada'}
                    d={d}
                    fill="none"
                    stroke={cor}
                    strokeOpacity={aresta.potencial ? 0.16 : 0.22}
                    strokeWidth="2"
                    strokeDasharray={aresta.potencial ? '5 5' : undefined}
                  />
                  {Array.from({ length: pontos }).map((_, k) => (
                    <circle key={k} r="3.5" fill={cor}>
                      <animateMotion
                        dur={`${duracao}s`}
                        begin={`${(k * duracao) / pontos}s`}
                        repeatCount="indefinite"
                      >
                        <mpath href={`#${caminho}`} />
                      </animateMotion>
                    </circle>
                  ))}
                </g>
              );
            })}

            <circle
              cx={lbX}
              cy={yLb}
              r="32"
              fill={corDoNo}
              fillOpacity="0.08"
              stroke={corDoNo}
              strokeOpacity="0.4"
              strokeDasharray={balanceador.potencial ? '5 5' : undefined}
            />
            {!balanceador.potencial && (
              <circle cx={lbX} cy={yLb} r="32" fill="none" stroke={corDoNo} strokeOpacity="0.25">
                <animate attributeName="r" values="32;44;32" dur="2.5s" repeatCount="indefinite" />
                <animate attributeName="stroke-opacity" values="0.35;0;0.35" dur="2.5s" repeatCount="indefinite" />
              </circle>
            )}
            <text
              x={lbX}
              y={yLb - 42}
              textAnchor="middle"
              fill="var(--color-text-faint)"
              fontSize="11"
              fontWeight="600"
            >
              {balanceador.nome}
            </text>
            <text
              x={lbX}
              y={yLb + 4}
              textAnchor="middle"
              fill="var(--color-text-hi)"
              fontSize="9"
              fontFamily="var(--font-mono)"
            >
              {balanceador.potencial ? 'reserva' : `${balanceador.reqs} req`}
            </text>
            {!balanceador.potencial && (
              <text
                x={lbX}
                y={yLb + 16}
                textAnchor="middle"
                fill="var(--color-text-mut)"
                fontSize="8"
                fontFamily="var(--font-mono)"
              >
                {janela}
              </text>
            )}
          </g>
        );
      })}

      {destinos.map((destino, i) => {
        const y = posicao(i, destinos.length, altura);
        const cor = corDaSeveridade(destino.severidade);
        return (
          <g key={destino.destino}>
            <circle cx={destX} cy={y} r="9" fill={cor} fillOpacity="0.15" stroke={cor} strokeOpacity="0.6" />
            <circle cx={destX} cy={y} r="3.5" fill={cor} />
            <text x={destX + 16} y={y - 4} fill="var(--color-text-hi)" fontSize="10" fontFamily="var(--font-mono)">
              {destino.rotulo}
            </text>
            <text x={destX + 16} y={y + 9} fill="var(--color-text-mut)" fontSize="9">
              {destino.reqs} reqs
            </text>
          </g>
        );
      })}
    </svg>
  );
};

const NginxView = () => {
  const dialog = useDialog();
  const session = useSession();
  const podeAssociar = hasGlobalAdmin(session.accesses);
  const [loadBalancing, setLoadBalancing] = useState<LbStat[]>([]);
  const [janelaDoLb, setJanelaDoLb] = useState<number | null>(null);
  const [servidores, setServidores] = useState<ServerLiveStat[]>([]);
  const [topologia, setTopologia] = useState<NginxUpstreamLink[]>([]);
  const [cadastro, setCadastro] = useState<ServerRecord[]>([]);
  const [escolha, setEscolha] = useState<Record<string, string>>({});
  const [associando, setAssociando] = useState('');
  const carga = useLoadStatus();
  const { ok: cargaOk, fail: cargaFail } = carga;
  const balanceadores = useMemo(
    () => balanceadoresDaMalha(arestasDaMalha(topologia, servidores, loadBalancing)),
    [topologia, servidores, loadBalancing],
  );

  const soltos = useMemo(
    () => upstreamsSemCadastro(deriveUpstreams(loadBalancing), servidores),
    [loadBalancing, servidores],
  );

  const carregarCadastro = useCallback(() => {
    if (!podeAssociar) return;
    api.servers().then(setCadastro).catch(() => setCadastro([]));
  }, [podeAssociar]);

  useEffect(() => {
    carregarCadastro();
  }, [carregarCadastro]);

  const associar = async (endereco: string) => {
    const alvo = escolha[endereco];
    if (!alvo) {
      dialog.notify('Escolha o servidor que responde por esse endereço.', 'error');
      return;
    }

    const servidor = cadastro.find((s) => s.id === alvo);
    const atuais = servidor?.aliases ?? [];
    setAssociando(endereco);
    try {
      await api.updateServerAliases(alvo, [...new Set([...atuais, endereco])]);
      dialog.notify(`${endereco} passou a pertencer a ${servidor?.name ?? 'servidor'}.`, 'success');
      carregarCadastro();
    } catch (err) {
      dialog.notify(apiErrorMessage(err, 'Falha ao associar o endereço.'), 'error');
    } finally {
      setAssociando('');
    }
  };

  useEffect(() => {
    const controller = new AbortController();
    const fetchMetrics = () => {
      api.liveMetrics(controller.signal)
        .then(data => {
          setLoadBalancing(data.load_balancing);
          setJanelaDoLb(data.lb_window_sec);
          setServidores(data.servers);
          setTopologia(data.nginx_topologia ?? []);
          cargaOk();
        })
        .catch(err => {
          if (!controller.signal.aborted) cargaFail(err, 'Falha ao ler o tráfego do Nginx.');
        });
    };

    fetchMetrics();
    const interval = setInterval(fetchMetrics, POLL.balanceador);
    return () => {
      clearInterval(interval);
      controller.abort();
    };
  }, [cargaOk, cargaFail]);

  return (
    <div className="p-4 md:p-8 h-full flex flex-col overflow-hidden anim-rise">
      <LoadNotice error={carga.error} lastOk={carga.lastOk} className="mb-4" />
      <div className="page-header flex-col md:flex-row items-start md:items-end">
        <div>
          <h1 className="page-title">Nginx e tráfego</h1>
          <p className="page-desc">Monitoramento de tráfego de rede e roteamento reverso do Load Balancer.</p>
        </div>
        <span className="badge badge-muted" title="Esta tela não executa ação nenhuma no Nginx">somente leitura</span>
      </div>

      <div className="panel mb-6 p-4">
        <div className="flex items-center gap-2 mb-2">
          <Server size={16} strokeWidth={1.75} className="text-text-faint" />
          <h2 className="eyebrow">Fluxo de roteamento ao vivo</h2>
        </div>
        <div className="min-h-[260px]">
          {balanceadores.length === 0 ? (
            <div className="h-[260px] flex items-center justify-center text-text-mut text-sm">
              {carga.error && !carga.lastOk
                ? carga.error
                : 'Nenhum balanceador descoberto ainda. A malha aparece assim que a sonda encontrar um Nginx com blocos upstream.'}
            </div>
          ) : (
            <MalhaDeTrafego balanceadores={balanceadores} janela={rotuloDaJanela(janelaDoLb)} />
          )}
        </div>
      </div>

      {soltos.length > 0 && (
        <div className="panel mb-6 p-4">
          <div className="mb-3 flex items-center gap-2">
            <Link2 size={16} strokeWidth={1.75} className="text-warn" />
            <h2 className="eyebrow">Endereços sem servidor cadastrado</h2>
          </div>
          <p className="mb-3 max-w-prose text-xs text-text-mut">
            O Nginx encaminha para estes endereços, e nenhum servidor cadastrado os declara. Enquanto
            isso, a malha mostra o endereço em vez do nome da máquina.
          </p>
          <ul className="flex flex-col gap-2">
            {soltos.map((node) => (
              <li
                key={node.addr}
                className="flex flex-col gap-2 rounded-ctrl border border-line bg-ink-850 p-3 md:flex-row md:items-center md:justify-between"
              >
                <span className="mono-data text-sm text-text-hi">{node.addr}</span>
                {podeAssociar && (
                  <div className="flex items-center gap-3">
                    <Select
                      ariaLabel={`Associar a ${node.addr}`}
                      className="w-56"
                      value={escolha[node.host] ?? ''}
                      onChange={(v) => setEscolha((atual) => ({ ...atual, [node.host]: v }))}
                      placeholder="Escolha o servidor"
                      options={cadastro.map((s) => ({ value: s.id, label: `${s.name} — ${s.host_ip}` }))}
                    />
                    <button
                      className="btn btn-primary btn-sm"
                      disabled={associando === node.host}
                      onClick={() => associar(node.host)}
                    >
                      Associar
                    </button>
                  </div>
                )}
              </li>
            ))}
          </ul>
        </div>
      )}

      <div className="panel flex flex-col flex-1 min-h-0 overflow-hidden">
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

        <div className="flex-1 overflow-auto custom-scrollbar p-4">
          <table className="table-base whitespace-nowrap">
            <thead>
              <tr>
                <th>Domínio / host</th>
                <th>Upstream (proxy pass)</th>
                <th>Requisições ({rotuloDaJanela(janelaDoLb)})</th>
                <th>Saúde</th>
              </tr>
            </thead>
            <tbody>
              {loadBalancing.length === 0 && (
                <tr>
                  <td colSpan={4} className="py-8 text-center text-text-mut">
                    {carga.error && !carga.lastOk ? carga.error : 'Aguardando tráfego ou nenhum log do Nginx encontrado.'}
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
