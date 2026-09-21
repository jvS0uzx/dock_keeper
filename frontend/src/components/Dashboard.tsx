import LoadNotice from './ui/LoadNotice';
import { useLoadStatus } from './ui/load-status';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Clock, Filter, Globe, Server, Database, Activity, ArrowRight, HardDrive } from 'lucide-react';
import { AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from 'recharts';
import {
  api,
  apiErrorMessage,
  type ContainerLiveStat,
  type HistoryRange,
  type LbStat,
  type NginxUpstreamLink,
  type ServerLiveStat,
  type ServerRecord,
} from '../lib/api';
import {
  ALTURA_MAXIMA_MALHA,
  LARGURA_PADRAO_MALHA,
  PASSO_BALANCEADOR,
  PASSO_CARTAO,
  alturaDoConteudo,
  alturaVisivel,
  centroDaLinha,
} from '../lib/malhaLayout';
import { hasGlobalAdmin } from '../lib/panels';
import { useDialog } from './ui/dialog-context';
import { useSession } from './ui/session-context';
import { formatBytes, formatGB, formatPercent } from '../lib/format';
import { mediaDefinida, percentualDeUso } from '../lib/agregado';
import {
  arestasDaMalha,
  balanceadoresDaMalha,
  carregarFiltroDaMalha,
  classificarMalha,
  deriveUpstreams,
  ehBalanceador,
  ehPrincipal,
  ehReserva,
  indiceDeEnderecos,
  nosDaMalha,
  rotuloDoUpstream,
  salvarFiltroDaMalha,
  splitUpstreams,
  totalRequests,
  type ArestaDaMalha,
  type FiltroDaMalha,
  type NoDaMalha,
  type UpstreamNode,
  rotuloDaJanela,
} from '../lib/upstream';
import Select, { type SelectOption } from './ui/Select';
import { POLL } from '../lib/polling';


const ERROR_STATUSES = ['500', '502', '503', '504', '400', '404'];


const LB_IP: string = import.meta.env.VITE_LB_IP || '';

const Gauge = ({ value, title, detalhe }: { value: number | null; title: string; detalhe?: string }) => {
  const clamped = value === null ? 0 : Math.max(0, Math.min(value, 100));
  const R = 78;
  const comprimento = Math.PI * R;
  const off = comprimento * (1 - clamped / 100);
  const gradId = `gauge-grad-${title.replace(/\W+/g, '-')}`;
  return (
    <div className="stat-card flex flex-col items-center justify-center relative h-full">
      <span className="eyebrow absolute top-4">{title}</span>
      <div className="relative mt-6">
        <svg width="200" height="110" viewBox="0 0 200 110" aria-hidden="true">
          <defs>
            <linearGradient id={gradId} x1="0" y1="0" x2="1" y2="0">
              <stop offset="0%" stopColor="var(--color-ok)" />
              <stop offset="55%" stopColor="var(--color-warn)" />
              <stop offset="100%" stopColor="var(--color-crit)" />
            </linearGradient>
          </defs>
          <path
            d={`M ${100 - R},96 A ${R},${R} 0 0 1 ${100 + R},96`}
            fill="none"
            stroke="var(--color-ink-750)"
            strokeWidth="14"
            strokeLinecap="round"
          />
          <path
            d={`M ${100 - R},96 A ${R},${R} 0 0 1 ${100 + R},96`}
            fill="none"
            stroke={`url(#${gradId})`}
            strokeWidth="14"
            strokeLinecap="round"
            strokeDasharray={comprimento}
            strokeDashoffset={off}
            style={{ transition: 'stroke-dashoffset 600ms cubic-bezier(0.4, 0, 0.2, 1)' }}
          />
        </svg>
        <div className="absolute inset-x-0 bottom-0 flex flex-col items-center">
          <span className={`stat-value text-3xl ${value === null ? 'text-text-faint' : ''}`}>
            {value === null ? '—' : `${value.toFixed(1)}%`}
          </span>
          {detalhe && <span className="text-xs text-text-faint">{detalhe}</span>}
        </div>
      </div>
    </div>
  );
};

const TRACO_ATIVO = 'color-mix(in srgb, var(--color-accent) 45%, var(--color-ink-900))';
const TRACO_OCIOSO = 'var(--color-text-faint)';
const TRACEJADO_OCIOSO = '4 4';
const TRACEJADO_POTENCIAL = '5 5';

const FILTROS_DA_MALHA: { value: FiltroDaMalha; label: string }[] = [
  { value: 'tudo', label: 'Tudo' },
  { value: 'atras', label: 'Só atrás do balanceador' },
  { value: 'fora', label: 'Só fora do balanceador' },
];

interface BalanceadorNaMalha {
  id: string;
  label: string;
  reqs: number;
  potencial: boolean;
  principal: boolean;
  arestas: ArestaDaMalha[];
}

interface ArestaParaNo {
  reqs: number;
  potencial: boolean;
}

const LoadBalancerFlow = ({
  stats,
  servers,
  topologia,
  janela,
  onAssociado,
}: {
  stats: LbStat[];
  servers: ServerLiveStat[];
  topologia: NginxUpstreamLink[];
  janela: string;
  onAssociado: () => void;
}) => {
  const session = useSession();
  const dialog = useDialog();
  const podeAssociar = hasGlobalAdmin(session.accesses);
  const [filtro, setFiltro] = useState<FiltroDaMalha>(carregarFiltroDaMalha);
  const [cadastro, setCadastro] = useState<ServerRecord[]>([]);
  const cadastroRef = useRef<ServerRecord[]>([]);
  const [abertoEm, setAbertoEm] = useState('');
  const [escolha, setEscolha] = useState('');
  const escolhaRef = useRef('');
  const [salvando, setSalvando] = useState(false);

  const destinosDeclarados = useMemo(() => topologia.map((enlace) => enlace.destino), [topologia]);
  const upstreams = useMemo(
    () => deriveUpstreams(stats, destinosDeclarados),
    [stats, destinosDeclarados],
  );
  const grupos = useMemo(() => classificarMalha(servers, upstreams, LB_IP), [servers, upstreams]);
  const todosOsNodes = useMemo(() => nosDaMalha(servers, upstreams, LB_IP), [servers, upstreams]);
  const nodes = filtro === 'fora' ? [] : todosOsNodes;
  const total = totalRequests(stats);

  const trocarFiltro = (valor: FiltroDaMalha) => {
    setFiltro(valor);
    salvarFiltroDaMalha(valor);
  };

  const carregarCadastro = useCallback(() => {
    if (!podeAssociar) return;
    api
      .servers()
      .then((lista) => {
        cadastroRef.current = lista;
        setCadastro(lista);
      })
      .catch(() => {
        cadastroRef.current = [];
        setCadastro([]);
      });
  }, [podeAssociar]);

  useEffect(() => {
    carregarCadastro();
  }, [carregarCadastro]);

  const escolher = (valor: string) => {
    escolhaRef.current = valor;
    setEscolha(valor);
  };

  const abrirAssociacao = (addr: string) => {
    escolher('');
    setAbertoEm((atual) => (atual === addr ? '' : addr));
  };

  const associar = async (node: NoDaMalha) => {
    const servidor = cadastroRef.current.find((s) => s.id === escolhaRef.current);
    if (!servidor) {
      dialog.notify('Escolha o servidor que responde por esse endereço.', 'error');
      return;
    }
    setSalvando(true);
    try {
      await api.updateServerAliases(servidor.id, [...new Set([...(servidor.aliases ?? []), node.host])]);
      dialog.notify(`${node.host} passou a pertencer a ${servidor.name}.`, 'success');
      setAbertoEm('');
      escolher('');
      carregarCadastro();
      onAssociado();
    } catch (err) {
      dialog.notify(apiErrorMessage(err, 'Falha ao associar o endereço.'), 'error');
    } finally {
      setSalvando(false);
    }
  };

  const opcoesDeServidor: SelectOption[] = cadastro.map((s) => ({ value: s.id, label: s.name }));

  const arestasDaTopologia = useMemo(
    () => arestasDaMalha(topologia, servers, stats),
    [topologia, servers, stats],
  );

  const lbs = useMemo<BalanceadorNaMalha[]>(() => {
    const porId = new Map<string, BalanceadorNaMalha>();

    for (const servidor of grupos.balanceadores) {
      porId.set(servidor.id, {
        id: servidor.id,
        label: servidor.name,
        reqs: 0,
        potencial: ehReserva(servidor),
        principal: ehPrincipal(servidor),
        arestas: [],
      });
    }

    for (const balanceador of balanceadoresDaMalha(arestasDaTopologia)) {
      const atual = porId.get(balanceador.id);
      if (atual === undefined) {
        porId.set(balanceador.id, {
          id: balanceador.id,
          label: balanceador.id === '' ? 'Load Balancer' : balanceador.nome,
          reqs: balanceador.reqs,
          potencial: balanceador.potencial,
          principal: false,
          arestas: balanceador.arestas,
        });
        continue;
      }
      atual.reqs = balanceador.reqs;
      atual.arestas = balanceador.arestas;
    }

    if (porId.size === 0) {
      porId.set('', {
        id: '',
        label: 'Load Balancer',
        reqs: 0,
        potencial: false,
        principal: false,
        arestas: [],
      });
    }

    return [...porId.values()].sort(
      (a, b) =>
        Number(a.potencial) - Number(b.potencial) ||
        b.reqs - a.reqs ||
        a.label.localeCompare(b.label),
    );
  }, [grupos.balanceadores, arestasDaTopologia]);

  const areaRef = useRef<HTMLDivElement>(null);
  const [largura, setLargura] = useState(0);
  useEffect(() => {
    const el = areaRef.current;
    if (!el) return;
    const observer = new ResizeObserver(() => setLargura(el.clientWidth));
    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  const enderecoParaNo = indiceDeEnderecos(nodes);
  const posicaoDoNo = new Map(nodes.map((n, i) => [n.id, i]));
  const arestas = lbs.map((lb) => {
    const porNo = new Map<string, ArestaParaNo>();
    for (const aresta of lb.arestas) {
      const id = enderecoParaNo.get(aresta.destino);
      if (id === undefined) continue;
      const atual = porNo.get(id) ?? { reqs: 0, potencial: lb.potencial || aresta.potencial };
      atual.reqs += aresta.reqs;
      porNo.set(id, atual);
    }
    return porNo;
  });
  const alturaConteudo = alturaDoConteudo(nodes.length, lbs.length);
  const alturaCaixa = alturaVisivel(alturaConteudo);
  const rolaDentro = alturaConteudo > ALTURA_MAXIMA_MALHA;

  const w = largura > 0 ? largura : LARGURA_PADRAO_MALHA;
  const yNo = (i: number) => centroDaLinha(i, nodes.length, alturaConteudo, PASSO_CARTAO);
  const yLb = (i: number) => centroDaLinha(i, lbs.length, alturaConteudo, PASSO_BALANCEADOR);
  const yMeio = alturaConteudo / 2;
  const xIn = 55;
  const xLbIn = w / 2 - 27;
  const xLbOut = w / 2 + 27;
  const xUp = w - 199;
  const curva = (x0: number, y0: number, x1: number, y1: number) => {
    const cx = (x1 - x0) * 0.45;
    return `M ${x0},${y0} C ${x0 + cx},${y0} ${x1 - cx},${y1} ${x1},${y1}`;
  };

  return (
    <div className="panel p-6 mb-6 relative">
      <div className="flex flex-wrap items-start justify-between gap-4 mb-8 pb-3 border-b border-line">
        <div className="flex flex-wrap items-center gap-4">
          <span className="eyebrow flex items-center gap-2">
            <Activity size={16} strokeWidth={1.75} className="text-accent" />
            Malha de roteamento (cluster NGINX)
          </span>
          <Select
            ariaLabel="Filtrar a malha"
            className="w-56"
            value={filtro}
            onChange={(v) => trocarFiltro(v as FiltroDaMalha)}
            options={FILTROS_DA_MALHA}
          />
        </div>
        <div className="flex flex-wrap items-center gap-3 md:ml-auto">
          {lbs.length > 1 && (
            <span className="text-[11px] text-text-faint" title="Máquinas que reportam tráfego como balanceador">
              {`${lbs.length} balanceadores`}
            </span>
          )}
          <span className="text-[11px] text-text-faint" title="Máquinas cadastradas que o Nginx usa como upstream">
            {`${grupos.atras.length} atrás do balanceador`}
          </span>
          {grupos.fora.length > 0 && (
            <span
              className="text-[11px] text-text-faint"
              title="Máquinas cadastradas que não recebem tráfego do balanceador"
            >
              {`${grupos.fora.length} fora do balanceador`}
            </span>
          )}
          {grupos.soltos.length > 0 && (
            <span
              className="text-[11px] text-warn"
              title="Endereços que aparecem no Nginx e não batem com servidor cadastrado"
            >
              {`${grupos.soltos.length} ${grupos.soltos.length === 1 ? 'endereço sem cadastro' : 'endereços sem cadastro'}`}
            </span>
          )}
          <span data-testid="malha-trafego" className={`badge ${total > 0 ? 'badge-ok' : 'badge-muted'}`}>
            <span className={`w-1.5 h-1.5 rounded-full ${total > 0 ? 'bg-ok animate-pulse' : 'bg-text-faint'}`} />
            {`${total} req / ${janela}`}
          </span>
        </div>
      </div>

      {filtro !== 'fora' && (
        <div
          ref={areaRef}
          data-testid="malha-area"
          className={`relative w-full max-w-4xl mx-auto ${rolaDentro ? 'overflow-y-auto custom-scrollbar pr-1' : ''}`}
          style={{ height: alturaCaixa }}
        >
          <div data-testid="malha-conteudo" className="relative w-full" style={{ height: alturaConteudo }}>
            <svg
              className="absolute inset-0 z-0 overflow-visible"
              width={w}
              height={alturaConteudo}
              viewBox={`0 0 ${w} ${alturaConteudo}`}
              aria-hidden="true"
            >
              {lbs.map((lb, li) => {
                const entrandoAnima = !lb.potencial && lb.reqs > 0;
                return (
                  <g key={`lb-${lb.id}`}>
                    <path
                      id={`path-in-${li}`}
                      data-testid="malha-aresta"
                      data-estado={lb.potencial ? 'potencial' : entrandoAnima ? 'com-trafego' : 'parada'}
                      d={curva(xIn, yMeio, xLbIn, yLb(li))}
                      fill="none"
                      stroke={entrandoAnima ? TRACO_ATIVO : TRACO_OCIOSO}
                      strokeDasharray={
                        lb.potencial ? TRACEJADO_POTENCIAL : entrandoAnima ? undefined : TRACEJADO_OCIOSO
                      }
                      strokeOpacity={lb.potencial ? 0.45 : undefined}
                      strokeWidth="1.25"
                    />
                    {entrandoAnima && (
                      <circle r="3" fill="var(--color-accent)">
                        <animateMotion dur="1.1s" repeatCount="indefinite">
                          <mpath href={`#path-in-${li}`} />
                        </animateMotion>
                      </circle>
                    )}
                    {[...arestas[li].entries()].map(([idDoNo, aresta]) => {
                      const ui = posicaoDoNo.get(idDoNo);
                      if (ui === undefined) return null;
                      const anima = !aresta.potencial && aresta.reqs > 0;
                      return (
                        <g key={`edge-${lb.id}-${idDoNo}`}>
                          <path
                            id={`edge-${li}-${ui}`}
                            data-testid="malha-aresta"
                            data-estado={aresta.potencial ? 'potencial' : anima ? 'com-trafego' : 'parada'}
                            data-de={lb.id}
                            data-para={idDoNo}
                            d={curva(xLbOut, yLb(li), xUp, yNo(ui))}
                            fill="none"
                            stroke={anima ? TRACO_ATIVO : TRACO_OCIOSO}
                            strokeDasharray={
                              aresta.potencial ? TRACEJADO_POTENCIAL : anima ? undefined : TRACEJADO_OCIOSO
                            }
                            strokeOpacity={aresta.potencial ? 0.45 : undefined}
                            strokeWidth="1.25"
                          />
                          {anima &&
                            Array.from({ length: Math.min(aresta.reqs, 5) }).map((_, i) => (
                              <circle key={`p-${li}-${ui}-${i}`} r="3" fill="var(--color-accent)">
                                <animateMotion dur="1.5s" begin={`${i * 0.3}s`} repeatCount="indefinite">
                                  <mpath href={`#edge-${li}-${ui}`} />
                                </animateMotion>
                              </circle>
                            ))}
                        </g>
                      );
                    })}
                  </g>
                );
              })}
              {nodes.map((node, ui) => {
                if (arestas.some((porNo) => porNo.has(node.id))) return null;
                return (
                  <path
                    key={`idle-${node.id}`}
                    data-testid="malha-aresta"
                    data-estado="parada"
                    d={curva(xLbOut, yLb(0), xUp, yNo(ui))}
                    fill="none"
                    stroke={TRACO_OCIOSO}
                    strokeDasharray={TRACEJADO_OCIOSO}
                    strokeWidth="1.25"
                  />
                );
              })}
            </svg>

            <div className="absolute left-1/4 -translate-x-1/2 top-0 z-10 bg-ink-950 px-2 py-0.5 rounded-full border border-line text-[10px] text-text-mut mono-data flex items-center gap-1">
              {total} <ArrowRight size={12} strokeWidth={1.75} className="text-accent" />
            </div>

            <div
              className="absolute z-10 flex flex-col items-center left-0"
              style={{ top: yMeio, transform: 'translateY(-50%)' }}
            >
              <div className="w-14 h-14 rounded-full bg-ink-800 border border-line flex items-center justify-center">
                <Globe size={22} strokeWidth={1.75} className="text-text-mut" />
              </div>
              <span className="eyebrow mt-2">Internet</span>
            </div>

            {lbs.map((lb, li) => {
              const recebendo = !lb.potencial && lb.reqs > 0;
              const papel = lb.potencial ? 'reserva' : lb.principal ? 'principal' : '';
              return (
                <div
                  key={`lb-box-${lb.id}`}
                  data-testid="malha-lb"
                  data-papel={lb.potencial ? 'reserva' : lb.principal ? 'principal' : 'indefinido'}
                  className="absolute z-10 left-1/2 flex flex-col items-center"
                  style={{ top: yLb(li), transform: 'translate(-50%, -50%)' }}
                >
                  <div
                    className={`w-14 h-14 rounded-card bg-ink-800 border flex items-center justify-center transition-colors ${
                      lb.potencial
                        ? 'border-dashed border-line opacity-70'
                        : recebendo
                          ? 'border-accent/40'
                          : 'border-line'
                    }`}
                  >
                    <Server
                      size={24}
                      strokeWidth={1.75}
                      className={
                        lb.potencial ? 'text-text-faint' : recebendo ? 'text-accent' : 'text-text-mut'
                      }
                    />
                  </div>
                  <span className="eyebrow mt-2 max-w-[150px] truncate" title={lb.label}>
                    {lb.label}
                  </span>
                  {papel !== '' && (
                    <span
                      className={`text-[10px] ${lb.potencial ? 'text-text-faint' : 'text-accent'}`}
                      title={
                        lb.potencial
                          ? 'Candidato a balanceador: assumiria o tráfego se a principal cair'
                          : 'Balanceador que está recebendo tráfego agora'
                      }
                    >
                      {papel}
                    </span>
                  )}
                </div>
              );
            })}

            {nodes.map((node, ui) => (
              <div
                key={node.id}
                data-testid="malha-no"
                className={`absolute right-0 w-[200px] panel panel-hover p-3 flex items-center gap-3 h-[72px] ${
                  abertoEm === node.id ? 'z-30' : 'z-10'
                } ${node.reqs > 0 ? 'border-accent/30' : ''}`}
                style={{ top: yNo(ui), transform: 'translateY(-50%)' }}
                title={node.enderecos.join(' · ')}
              >
                <div className="w-9 h-9 rounded-ctrl bg-ink-800 flex items-center justify-center flex-shrink-0">
                  <Database
                    size={16}
                    strokeWidth={1.75}
                    className={node.reqs > 0 ? 'text-accent' : 'text-text-faint'}
                  />
                </div>
                <div className="flex flex-col min-w-0 flex-1 gap-0.5">
                  <span className="text-xs text-text-hi font-medium truncate" title={node.rotulo}>
                    {node.rotulo}
                  </span>
                  {node.cadastrado ? (
                    <span
                      className="text-[10px] text-text-faint mono-data selectable truncate"
                      title={node.enderecos.join(' · ')}
                    >
                      {node.enderecos.join(' · ')}
                    </span>
                  ) : podeAssociar ? (
                    <button
                      type="button"
                      onClick={() => abrirAssociacao(node.id)}
                      className="w-fit text-[10px] text-warn transition-colors hover:text-accent-hi"
                      title="Endereço que não bate com nenhum servidor cadastrado"
                    >
                      não cadastrado · associar
                    </button>
                  ) : (
                    <span
                      className="text-[10px] text-text-faint truncate"
                      title="Endereço que não bate com nenhum servidor cadastrado"
                    >
                      não cadastrado · peça a um administrador
                    </span>
                  )}
                  {node.reqs > 0 ? (
                    <span className="text-[10px] mono-data text-accent">{`${node.reqs} req / ${janela}`}</span>
                  ) : (
                    <span className="text-[10px] text-text-faint truncate">sem tráfego na janela</span>
                  )}
                </div>

                {abertoEm === node.id && (
                  <div className="absolute right-0 top-full z-30 mt-1 flex w-[224px] flex-col gap-2 rounded-card border border-line bg-ink-900 p-2 shadow-lg">
                    <span className="text-[10px] text-text-faint">Quem responde por {node.host}?</span>
                    <Select
                      ariaLabel={`Associar ${node.rotulo} a um servidor`}
                      value={escolha}
                      onChange={escolher}
                      options={opcoesDeServidor}
                      placeholder="Escolha o servidor"
                    />
                    <div className="flex items-center gap-2">
                      <button
                        type="button"
                        className="btn btn-primary btn-sm"
                        disabled={salvando}
                        onClick={() => associar(node)}
                      >
                        Salvar
                      </button>
                      <button type="button" className="btn btn-ghost btn-sm" onClick={() => setAbertoEm('')}>
                        Cancelar
                      </button>
                    </div>
                  </div>
                )}
              </div>
            ))}
          </div>
        </div>
      )}

      {filtro !== 'atras' && grupos.fora.length > 0 && (
        <section className="mt-6 border-t border-line pt-4">
          <div className="mb-1 flex flex-wrap items-center gap-2">
            <span className="eyebrow">Fora do balanceador</span>
            <span className="text-[11px] text-text-faint">
              cadastradas e coletadas pelo painel, sem receber tráfego do Nginx
            </span>
          </div>
          <div className="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {grupos.fora.map((maquina) => (
              <div key={maquina.id} className="panel p-3">
                <div className="flex items-center justify-between gap-2">
                  <span className="truncate text-xs font-medium text-text-hi">{maquina.name}</span>
                  <span className={`badge ${maquina.online ? 'badge-ok' : 'badge-crit'}`}>
                    {maquina.online ? 'Online' : 'Offline'}
                  </span>
                </div>
                <span className="mono-data mt-1 block text-[10px] text-text-faint">{maquina.host_ip}</span>
                <div className="mt-2 flex items-center gap-4 text-[11px] text-text-mut">
                  <span>{`CPU ${formatPercent(maquina.cpu)}`}</span>
                  <span>
                    {maquina.mem_total > 0
                      ? `RAM ${formatGB(maquina.mem_used)}/${formatGB(maquina.mem_total)} GB`
                      : 'RAM —'}
                  </span>
                </div>
              </div>
            ))}
          </div>
        </section>
      )}
    </div>
  );
};

interface ProjectTraffic {
  name: string;
  total: number;
  errors: number;
  vpsCounts: Record<string, number>;
  local: number;
  algo: string;
}

const groupTrafficByProject = (stats: LbStat[], nodes: UpstreamNode[]): ProjectTraffic[] => {
  const projects = new Map<string, ProjectTraffic>();
  const known = new Set(nodes.map((n) => n.addr));

  stats.forEach((s) => {
    const name = s.server_name || 'Desconhecido';
    if (!projects.has(name)) {
      const isWebSocket = name.includes('ws.') || name.includes('soketi') || s.upstream_addr.includes('6001');
      projects.set(name, {
        name,
        total: 0,
        errors: 0,
        vpsCounts: {},
        local: 0,
        algo: isWebSocket ? 'IP_HASH (Sticky)' : 'LEAST_CONN',
      });
    }
    const data = projects.get(name)!;
    data.total += s.requests_count;
    if (ERROR_STATUSES.includes(s.status)) data.errors += s.requests_count;

    const addrs = splitUpstreams(s.upstream_addr).filter((a) => known.has(a));
    if (addrs.length === 0) {
      data.local += s.requests_count;
      return;
    }
    for (const addr of addrs) {
      data.vpsCounts[addr] = (data.vpsCounts[addr] || 0) + s.requests_count;
    }
  });

  return Array.from(projects.values()).sort((a, b) => b.total - a.total);
};

const LoadBalancerDashboard = ({
  stats,
  servers,
  janela,
}: {
  stats: LbStat[];
  servers: ServerLiveStat[];
  janela: string;
}) => {
  const nodes = useMemo(() => deriveUpstreams(stats), [stats]);
  const projects = useMemo(() => groupTrafficByProject(stats, nodes), [stats, nodes]);
  const totalErrors = projects.reduce((acc, p) => acc + p.errors, 0);

  return (
    <div className="flex flex-col gap-6 anim-rise">
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4 md:gap-6">
        <div className="stat-card">
          <span className="eyebrow">Erros HTTP na janela ({janela})</span>
          <div className="flex items-baseline gap-3 mt-2">
            <span className={`stat-value text-4xl ${totalErrors > 0 ? 'text-crit' : 'text-ok'}`}>{totalErrors}</span>
            <span className="text-xs text-text-mut">respostas 5xx/4xx</span>
          </div>
          {totalErrors > 0 && (
            <span className="text-xs text-crit mt-2 block">Verifique a tela de Logs para o detalhe das falhas.</span>
          )}
        </div>

        <div className="stat-card flex flex-col justify-center">
          <span className="eyebrow mb-3">Algoritmos ativos (NGINX)</span>
          <div className="flex flex-col gap-2">
            <div className="flex justify-between items-center bg-ink-950 px-3 py-1.5 rounded-ctrl border border-line">
              <span className="text-xs text-text">Tráfego HTTP/API</span>
              <span className="text-xs mono-data text-text-mut">LEAST_CONN</span>
            </div>
            <div className="flex justify-between items-center bg-ink-950 px-3 py-1.5 rounded-ctrl border border-line">
              <span className="text-xs text-text">WebSockets (Kanban)</span>
              <span className="text-xs mono-data text-text-mut">IP_HASH</span>
            </div>
          </div>
        </div>
      </div>

      <div className="panel p-6 overflow-hidden">
        <div className="flex items-center justify-between mb-4 border-b border-line pb-3">
          <span className="eyebrow flex items-center gap-2">
            <Server size={16} strokeWidth={1.75} className="text-accent" />
            Comportamento por sistema (projetos)
          </span>
        </div>

        <div className="overflow-x-auto">
          <table className="table-base">
            <thead>
              <tr>
                <th>Domínio / Sistema</th>
                <th>Algoritmo</th>
                {nodes.map((node) => (
                  <th key={node.addr} className="text-right" title={node.addr}>
                    {rotuloDoUpstream(node, servers)}
                  </th>
                ))}
                <th className="text-right">Local/Cache</th>
                <th className="text-right">Total req / {janela}</th>
                <th className="text-right">Erros</th>
              </tr>
            </thead>
            <tbody>
              {projects.length === 0 && (
                <tr>
                  <td colSpan={5 + nodes.length} className="text-center py-8 text-text-faint text-xs">
                    Sem tráfego nos últimos 5 segundos.
                  </td>
                </tr>
              )}
              {projects.map((p) => (
                <tr key={p.name}>
                  <td className="text-text-hi font-medium">{p.name}</td>
                  <td>
                    <span className="badge badge-muted mono-data">{p.algo}</span>
                  </td>
                  {nodes.map((node) => (
                    <td key={node.addr} className="text-right mono-data text-text-mut">{p.vpsCounts[node.addr] || 0}</td>
                  ))}
                  <td className="text-right mono-data text-text-mut">{p.local > 0 ? p.local : '-'}</td>
                  <td className="text-right mono-data text-text-hi font-semibold">{p.total}</td>
                  <td className={`text-right mono-data font-semibold ${p.errors > 0 ? 'text-crit' : 'text-text-faint'}`}>{p.errors}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
};

const RANGE_OPTIONS: { value: HistoryRange; label: string }[] = [
  { value: '1h', label: 'Última hora' },
  { value: '6h', label: 'Últimas 6 horas' },
  { value: '24h', label: 'Últimas 24 horas' },
  { value: '7d', label: 'Últimos 7 dias' },
];

interface DiskPoint {
  time: string;
  value: number;
}

export default function Dashboard() {
  const [servers, setServers] = useState<ServerLiveStat[]>([]);
  const [containers, setContainers] = useState<ContainerLiveStat[]>([]);
  const [loadBalancing, setLoadBalancing] = useState<LbStat[]>([]);
  const [topologiaDoNginx, setTopologiaDoNginx] = useState<NginxUpstreamLink[]>([]);
  const [janelaDoLb, setJanelaDoLb] = useState<number | null>(null);
  const carga = useLoadStatus();
  const { ok: cargaOk, fail: cargaFail } = carga;
  const disco = useLoadStatus();
  const { ok: discoOk, fail: discoFail } = disco;

  const [selectedServerId, setSelectedServerId] = useState('all');
  const [historyRange, setHistoryRange] = useState<HistoryRange>('1h');
  const [diskHistory, setDiskHistory] = useState<DiskPoint[]>([]);

  const [now, setNow] = useState<Date | null>(null);

  useEffect(() => {
    setNow(new Date());
    const timer = setInterval(() => setNow(new Date()), 1000);
    return () => clearInterval(timer);
  }, []);

  const recarregarVivo = useCallback(() => {
    api
      .liveMetrics()
      .then((data) => {
        setServers(data.servers);
        setContainers(data.containers);
        setLoadBalancing(data.load_balancing);
        setTopologiaDoNginx(data.nginx_topologia ?? []);
        setJanelaDoLb(data.lb_window_sec);
        cargaOk();
      })
      .catch((err) => cargaFail(err, 'Falha ao ler as métricas ao vivo.'));
  }, [cargaOk, cargaFail]);

  useEffect(() => {
    const controller = new AbortController();
    const fetchMetrics = () => {
      api.liveMetrics(controller.signal)
        .then((data) => {
          setServers(data.servers);
          setContainers(data.containers);
          setLoadBalancing(data.load_balancing);
          setTopologiaDoNginx(data.nginx_topologia ?? []);
          setJanelaDoLb(data.lb_window_sec);
          cargaOk();
        })
        .catch((err) => {
          if (!controller.signal.aborted) cargaFail(err, 'Falha ao ler as métricas ao vivo.');
        });
    };
    fetchMetrics();
    const interval = setInterval(fetchMetrics, POLL.aoVivo);
    return () => {
      clearInterval(interval);
      controller.abort();
    };
  }, [cargaOk, cargaFail]);

  useEffect(() => {
    if (selectedServerId === 'all') {
      setDiskHistory([]);
      return;
    }
    const controller = new AbortController();
    const fetchHistory = () => {
      api.history(selectedServerId, 'disk', historyRange, controller.signal)
        .then((points) =>
          setDiskHistory(points.map((p) => ({
            time: new Date(p.ts).toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' }),
            value: Number(p.value.toFixed(1)),
          }))),
        )
        .then(() => discoOk())
        .catch((err) => {
          if (!controller.signal.aborted) discoFail(err, 'Falha ao ler o histórico de disco.');
        });
    };
    fetchHistory();
    const interval = setInterval(fetchHistory, POLL.historicoDoPainel);
    return () => {
      clearInterval(interval);
      controller.abort();
    };
  }, [selectedServerId, historyRange, discoOk, discoFail]);

  const activeServer = selectedServerId === 'all' ? null : servers.find((s) => s.id === selectedServerId) ?? null;
  const scopedServers = activeServer ? [activeServer] : servers;
  const onlineServers = scopedServers.filter((s) => s.online);

  const filteredContainers = selectedServerId === 'all' ? containers : containers.filter((c) => c.server_id === selectedServerId);

  const cpuMedia = mediaDefinida(onlineServers.map((s) => s.cpu));
  const cpuDetalhe = cpuMedia.considerados === cpuMedia.total
    ? undefined
    : `média de ${cpuMedia.considerados} de ${cpuMedia.total} hosts`;

  const memUsed = onlineServers.reduce((acc, s) => acc + s.mem_used, 0);
  const memTotal = onlineServers.reduce((acc, s) => acc + s.mem_total, 0);
  const memPercent = percentualDeUso(memUsed, memTotal);

  const diskUsed = onlineServers.reduce((acc, s) => acc + s.disk_used, 0);
  const diskTotal = onlineServers.reduce((acc, s) => acc + s.disk_total, 0);
  const diskPercent = percentualDeUso(diskUsed, diskTotal);

  const isUp = onlineServers.length > 0;
  const offlineCount = scopedServers.length - onlineServers.length;

  const uniqueServers = useMemo(() => {
    const byIp = new Map<string, ServerLiveStat>();
    servers.forEach((s) => {
      if (!byIp.has(s.host_ip)) byIp.set(s.host_ip, s);
    });
    return Array.from(byIp.values());
  }, [servers]);

  const vpsOptions: SelectOption[] = [
    { value: 'all', label: 'Global (Cluster)' },
    ...uniqueServers.map((s) => ({
      value: s.id,
      label: ehBalanceador(s, LB_IP) ? `${s.name} — ${s.host_ip} (LB)` : `${s.name} — ${s.host_ip}`,
    })),
  ];

  const isLoadBalancerSelected = activeServer ? ehBalanceador(activeServer, LB_IP) : false;

  return (
    <div className="min-h-full px-4 pb-4 pt-2 md:px-6 md:pb-6 md:pt-3 lg:px-8 lg:pb-8 lg:pt-4 anim-rise">
      <LoadNotice error={carga.error} lastOk={carga.lastOk} className="mb-4" />
      <LoadNotice error={disco.error} lastOk={disco.lastOk} className="mb-4" />
      <div className="panel p-4 flex flex-wrap items-center gap-6 mb-6 relative z-50">
        <div className="flex items-center gap-3">
          <Filter size={16} strokeWidth={1.75} className="text-text-faint" />
          <span className="eyebrow">Filtro IP</span>
          <Select ariaLabel="Filtrar por IP" options={vpsOptions} value={selectedServerId} onChange={setSelectedServerId} />
        </div>

        <div className="h-6 w-px bg-line" />

        <div className="flex items-center gap-3">
          <Clock size={16} strokeWidth={1.75} className="text-text-faint" />
          <span className="eyebrow">Histórico</span>
          <Select
            ariaLabel="Janela do histórico de disco"
            options={RANGE_OPTIONS}
            value={historyRange}
            onChange={(v) => setHistoryRange(v as HistoryRange)}
          />
        </div>
      </div>

      {isLoadBalancerSelected ? (
        <LoadBalancerDashboard stats={loadBalancing} servers={servers} janela={rotuloDaJanela(janelaDoLb)} />
      ) : (
        <>
          <div className="grid grid-cols-1 lg:grid-cols-3 gap-4 md:gap-6 mb-6 stagger">
            <div className="stat-card lg:col-span-2 flex flex-col justify-between">
              <div className="flex items-center justify-between">
                <span className="eyebrow">Estado do cluster</span>
                <span className={`badge ${isUp ? 'badge-ok' : 'badge-crit'}`}>
                  <span className={`w-1.5 h-1.5 rounded-full ${isUp ? 'bg-ok animate-pulse' : 'bg-crit'}`} />
                  {isUp ? 'Operacional' : 'Indisponível'}
                </span>
              </div>
              <div className="grid grid-cols-3 gap-4 mt-4">
                <div>
                  <div className={`stat-value ${isUp ? '' : 'text-crit'}`}>{onlineServers.length}</div>
                  <div className="text-xs text-text-mut mt-1">hosts online</div>
                </div>
                <div>
                  <div className={`stat-value ${offlineCount > 0 ? 'text-crit' : 'text-text-faint'}`}>{offlineCount}</div>
                  <div className="text-xs text-text-mut mt-1">hosts offline</div>
                </div>
                <div>
                  <div className="stat-value">{filteredContainers.length}</div>
                  <div className="text-xs text-text-mut mt-1">containers ativos</div>
                </div>
              </div>
            </div>

            <div className="stat-card flex flex-col items-center justify-center">
              <span className="eyebrow">
                {now ? now.toLocaleDateString('pt-BR').replace(/\//g, '-') : 'Carregando...'}
              </span>
              <span className="stat-value text-4xl mt-2">
                {now ? now.toLocaleTimeString('pt-BR') : '00:00:00'}
              </span>
            </div>
          </div>

          {selectedServerId === 'all' && (
            <LoadBalancerFlow
              stats={loadBalancing}
              servers={servers}
              topologia={topologiaDoNginx}
              janela={rotuloDaJanela(janelaDoLb)}
              onAssociado={recarregarVivo}
            />
          )}

          <div className="grid grid-cols-1 md:grid-cols-3 gap-4 md:gap-6 mb-6 h-56 stagger">
            <Gauge value={cpuMedia.media} title="CPU do host" detalhe={cpuDetalhe} />
            <Gauge value={memPercent} title="Memória" />
            <Gauge value={diskPercent} title="Disco" />
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4 md:gap-6">
            <div className="panel p-6 flex flex-col relative h-80 overflow-hidden">
              <div className="flex items-center justify-between mb-4 border-b border-line pb-3">
                <span className="eyebrow">Consumo (ao vivo)</span>
                <span className="badge badge-ok">
                  <span className="w-1.5 h-1.5 rounded-full bg-ok animate-pulse" />
                  Live
                </span>
              </div>
              <div className="flex-1 overflow-y-auto pr-2 custom-scrollbar">
                <table className="table-base">
                  <thead className="sticky top-0 bg-ink-900 z-10">
                    <tr>
                      <th>Container</th>
                      <th className="text-right">CPU</th>
                      <th className="text-right">Memória</th>
                    </tr>
                  </thead>
                  <tbody>
                    {filteredContainers.length === 0 && (
                      <tr>
                        <td colSpan={3} className="text-center py-8 text-text-faint text-xs">Buscando dados no motor Go...</td>
                      </tr>
                    )}
                    {filteredContainers.map((c) => (
                      <tr key={c.docker_id}>
                        <td className="mono-data text-text selectable">{c.name}</td>
                        <td className="text-right mono-data text-text-hi">{c.cpu.toFixed(2)}%</td>
                        <td className="text-right mono-data text-text-hi">{formatBytes(c.mem_used)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>

            <div className="panel p-6 flex flex-col relative h-80 overflow-hidden">
              <div className="flex items-center justify-between mb-4 border-b border-line pb-3">
                <span className="eyebrow">Espaço de disco ocupado</span>
                <span className="text-[11px] text-text-faint mono-data">
                  {activeServer ? `${activeServer.host_ip} · ${historyRange}` : 'Cluster'}
                </span>
              </div>

              {activeServer ? (
                <div className="flex-1 mt-2">
                  {diskHistory.length === 0 ? (
                    <div className="h-full flex items-center justify-center text-xs text-text-faint">
                      Sem histórico de disco na janela selecionada.
                    </div>
                  ) : (
                    <ResponsiveContainer width="100%" height="100%">
                      <AreaChart data={diskHistory} margin={{ top: 10, right: 0, left: -20, bottom: 0 }}>
                        <defs>
                          <linearGradient id="colorDisk" x1="0" y1="0" x2="0" y2="1">
                            <stop offset="5%" stopColor="var(--color-accent)" stopOpacity={0.12} />
                            <stop offset="95%" stopColor="var(--color-accent)" stopOpacity={0} />
                          </linearGradient>
                        </defs>
                        <CartesianGrid strokeDasharray="3 3" stroke="var(--color-line)" strokeOpacity={0.5} vertical={false} />
                        <XAxis
                          dataKey="time"
                          tick={{ fill: 'var(--color-text-faint)', fontSize: 11 }}
                          tickLine={false}
                          axisLine={false}
                          minTickGap={30}
                        />
                        <YAxis
                          tick={{ fill: 'var(--color-text-faint)', fontSize: 11 }}
                          tickLine={false}
                          axisLine={false}
                          domain={[0, 100]}
                          tickFormatter={(v) => `${v}%`}
                        />
                        <Tooltip
                          contentStyle={{
                            backgroundColor: 'var(--color-ink-800)',
                            border: '1px solid var(--color-line-hi)',
                            borderRadius: 10,
                            fontSize: 12,
                            fontFamily: 'var(--font-mono)',
                          }}
                          labelStyle={{ color: 'var(--color-text-mut)' }}
                          itemStyle={{ color: 'var(--color-text-hi)' }}
                          formatter={(value) => [`${value}%`, 'Disco']}
                        />
                        <Area
                          type="monotone"
                          dataKey="value"
                          stroke="var(--color-accent)"
                          strokeWidth={2}
                          fill="url(#colorDisk)"
                          isAnimationActive={false}
                        />
                      </AreaChart>
                    </ResponsiveContainer>
                  )}
                </div>
              ) : (
                <div className="flex-1 mt-2 overflow-y-auto custom-scrollbar flex flex-col gap-3">
                  {onlineServers.length === 0 && (
                    <div className="text-xs text-text-faint">{carga.error && !carga.lastOk ? carga.error : 'Nenhum host online reportando disco.'}</div>
                  )}
                  {onlineServers.map((s) => {
                    const pct = s.disk_total > 0 ? (s.disk_used / s.disk_total) * 100 : 0;
                    return (
                      <div key={s.id} className="flex flex-col gap-1">
                        <div className="flex items-center justify-between text-[11px]">
                          <span className="flex items-center gap-2 text-text">
                            <HardDrive size={12} strokeWidth={1.75} className="text-text-faint" />
                            <span className="mono-data selectable">{s.host_ip}</span>
                          </span>
                          <span className="mono-data text-text-mut">
                            {formatGB(s.disk_used)} / {formatGB(s.disk_total)} GB
                          </span>
                        </div>
                        <div className="w-full h-1.5 bg-ink-750 rounded-full overflow-hidden">
                          <div
                            className={`h-full ${pct > 85 ? 'bg-crit' : 'bg-ok'}`}
                            style={{ width: `${Math.min(pct, 100)}%` }}
                          />
                        </div>
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
          </div>
        </>
      )}
    </div>
  );
}
