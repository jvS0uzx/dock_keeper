import React, { useEffect, useState, useRef, useMemo } from 'react';
import { Search, Play, Square, RefreshCw, Terminal, X, Cpu, MemoryStick, ChevronDown, ChevronRight, Folder } from 'lucide-react';
import { api, openStream, type ContainerLiveStat } from '../lib/api';
import { formatBytes } from '../lib/format';
import { useDialog } from './ui/dialog-context';
import { useRole } from './ui/session-context';

const MAX_LOG_LINES = 100;

const ContainersView = () => {
  const dialog = useDialog();
  const { canOperate } = useRole();
  const [containers, setContainers] = useState<ContainerLiveStat[]>([]);
  const [search, setSearch] = useState('');

  const [selectedContainer, setSelectedContainer] = useState<ContainerLiveStat | null>(null);
  const [logs, setLogs] = useState<string[]>([]);
  const [expandedProjects, setExpandedProjects] = useState<Record<string, boolean>>({});

  const logsEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const controller = new AbortController();
    const fetchMetrics = () => {
      api.liveMetrics(controller.signal)
        .then(data => setContainers(data.containers))
        .catch(() => {});
    };
    fetchMetrics();
    const interval = setInterval(fetchMetrics, 3000);
    return () => {
      clearInterval(interval);
      controller.abort();
    };
  }, []);

  useEffect(() => {
    if (!selectedContainer) return;

    setLogs([]);

    // O ticket é buscado de forma assíncrona: se a tela fechar antes da
    // resposta, o stream nem chega a ser aberto.
    let source: EventSource | null = null;
    let cancelled = false;

    openStream('/api/containers/logs/stream', {
      server_id: selectedContainer.server_id,
      container_name: selectedContainer.name,
    })
      .then(es => {
        if (cancelled) {
          es.close();
          return;
        }
        source = es;
        es.onmessage = (event) => {
          setLogs(prev => [...prev, event.data].slice(-MAX_LOG_LINES));
        };
        es.onerror = () => {
          setLogs(prev => [...prev, '[Conexão de Logs Encerrada]']);
          es.close();
        };
      })
      .catch(() => setLogs(['[Falha ao autorizar o stream de logs]']));

    return () => {
      cancelled = true;
      source?.close();
    };
  }, [selectedContainer]);

  useEffect(() => {
    if (logsEndRef.current) {
      logsEndRef.current.scrollIntoView({ behavior: 'smooth' });
    }
  }, [logs]);

  const toggleProject = (project: string) => {
    setExpandedProjects(prev => ({
      // O projeto nasce aberto sem chave no mapa. Inverter `prev[project]`
      // direto gravava `true` no primeiro clique — que também é aberto — e o
      // operador precisava clicar duas vezes para fechar.
      ...prev,
      [project]: prev[project] === false,
    }));
  };

  const [actionBusy, setActionBusy] = useState<Record<string, boolean>>({});

  const containerAction = async (c: ContainerLiveStat, action: 'start' | 'stop' | 'restart') => {
    if (action === 'stop') {
      const confirmed = await dialog.confirm({
        title: `Parar o container ${c.name}?`,
        message: 'O serviço fica indisponível até ser iniciado de novo.',
        confirmLabel: 'Parar',
        danger: true,
      });
      if (!confirmed) return;
    }
    setActionBusy(prev => ({ ...prev, [c.docker_id]: true }));
    try {
      await api.containerAction(c.server_id, c.name, action);
      dialog.notify(`Comando "${action}" enviado para ${c.name}.`, 'success');
    } catch (err) {
      dialog.notify(`Falha ao ${action} ${c.name}: ${(err as Error).message}`, 'error');
    } finally {
      setActionBusy(prev => ({ ...prev, [c.docker_id]: false }));
    }
  };

  const groupedContainers = useMemo(() => {
    const filtered = containers.filter(c => c.name.toLowerCase().includes(search.toLowerCase()));
    const groups: Record<string, ContainerLiveStat[]> = {};

    filtered.forEach(c => {
      const proj = c.project || 'Sem Projeto (Avulsos)';
      if (!groups[proj]) groups[proj] = [];
      groups[proj].push(c);
    });

    return groups;
  }, [containers, search]);

  const liveSelected = selectedContainer ? containers.find(c => c.docker_id === selectedContainer.docker_id) || selectedContainer : null;

  return (
    <div className="p-4 md:p-8 min-h-full relative anim-rise">
      <div className="page-header flex-col md:flex-row items-start md:items-end">
        <div>
          <h1 className="page-title">Containers</h1>
          <p className="page-desc">Consumo em tempo real de todos os containers distribuídos na infraestrutura.</p>
        </div>
        <div className="relative w-full md:w-64">
          <Search size={16} strokeWidth={1.75} className="text-text-faint absolute left-3 top-1/2 -translate-y-1/2" />
          <input
            type="text"
            placeholder="Buscar container..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="input-base w-full pl-9"
          />
        </div>
      </div>

      <div className="panel p-5">
        <div className="flex items-center justify-between mb-4">
          <h2 className="eyebrow">Projetos e containers · {containers.length}</h2>
          <span className="badge badge-ok">
            <span className="w-1.5 h-1.5 rounded-full bg-ok animate-pulse"></span>
            ao vivo
          </span>
        </div>

        <div className="overflow-x-auto custom-scrollbar">
          <table className="table-base min-w-[800px]">
            <thead>
              <tr>
                <th className="w-8"></th>
                <th>Nome</th>
                <th>Estado / uptime</th>
                <th className="text-right">CPU</th>
                <th className="text-right">Memória (RAM)</th>
                <th className="text-center">Ações</th>
              </tr>
            </thead>
            <tbody>
              {Object.keys(groupedContainers).length === 0 ? (
                <tr>
                  <td colSpan={6} className="py-8 text-center text-text-mut">Nenhum container encontrado.</td>
                </tr>
              ) : (
                Object.entries(groupedContainers).map(([project, projContainers]) => {
                  const isExpanded = expandedProjects[project] !== false; // default true

                  return (
                    <React.Fragment key={project}>
                      {/* Linha do projeto */}
                      <tr
                        className="bg-ink-900 cursor-pointer"
                        onClick={() => toggleProject(project)}
                      >
                        <td className="text-text-faint">
                          {isExpanded ? <ChevronDown size={16} strokeWidth={1.75} /> : <ChevronRight size={16} strokeWidth={1.75} />}
                        </td>
                        <td colSpan={5}>
                          <div className="flex items-center gap-2.5">
                            <Folder size={14} strokeWidth={1.75} className="text-text-faint" />
                            <span className="font-semibold text-text-hi">{project}</span>
                            <span className="badge badge-muted">{projContainers.length} containers</span>
                          </div>
                        </td>
                      </tr>

                      {/* Linhas dos containers */}
                      {isExpanded && projContainers.map((c) => {
                        const memUsedStr = formatBytes(c.mem_used);
                        const memLimitStr = formatBytes(c.mem_limit);
                        const memPercent = c.mem_limit > 0 ? ((c.mem_used / c.mem_limit) * 100).toFixed(1) : '0.0';
                        const isRunning = c.state === 'running';

                        return (
                          <tr key={c.docker_id}>
                            <td></td>
                            <td>
                              <div className="flex items-center gap-3">
                                <div className="w-5 h-px bg-line" />
                                <span className="mono-data text-text-hi">{c.name}</span>
                              </div>
                            </td>
                            <td>
                              <div className="flex flex-col gap-1 items-start">
                                <span className={`badge ${!c.state ? 'badge-muted' : isRunning ? 'badge-ok' : 'badge-crit'}`}>
                                  {c.state || 'desconhecido'}
                                </span>
                                <span className="text-xs text-text-faint whitespace-nowrap" title={c.status}>{c.status}</span>
                              </div>
                            </td>
                            <td className="text-right">
                              {isRunning ? (
                                <div className="flex flex-col items-end">
                                  <span className="mono-data text-text-hi">{c.cpu.toFixed(2)}%</span>
                                  <div className="w-16 h-1 bg-ink-750 rounded-full mt-1.5 overflow-hidden">
                                    <div className="h-full bg-accent/80" style={{ width: `${Math.min(c.cpu, 100)}%` }}></div>
                                  </div>
                                </div>
                              ) : (
                                <span className="text-text-faint text-xs">-</span>
                              )}
                            </td>
                            <td className="text-right">
                              {isRunning ? (
                                <div className="flex flex-col items-end">
                                  <span className="mono-data text-text-hi">{memUsedStr} <span className="text-text-faint">/ {memLimitStr}</span></span>
                                  <span className="text-[11px] text-text-faint mt-0.5 tabular">{memPercent}%</span>
                                </div>
                              ) : (
                                <span className="text-text-faint text-xs">-</span>
                              )}
                            </td>
                            <td className="text-center">
                              <div className="flex items-center justify-center gap-1.5">
                                <button
                                  onClick={() => setSelectedContainer(c)}
                                  className="btn btn-ghost btn-sm text-accent"
                                  title="Ver logs ao vivo"
                                >
                                  <Terminal size={16} strokeWidth={1.75} />
                                </button>
                                {/* Start/stop/restart mudam a infraestrutura: só Suporte TI para cima. */}
                                {canOperate && (isRunning ? (
                                  <>
                                    <button
                                      disabled={actionBusy[c.docker_id]}
                                      onClick={() => containerAction(c, 'restart')}
                                      className="btn btn-ghost btn-sm disabled:opacity-40"
                                      title="Reiniciar"
                                    >
                                      <RefreshCw size={16} strokeWidth={1.75} className={actionBusy[c.docker_id] ? 'animate-spin' : ''} />
                                    </button>
                                    <button
                                      disabled={actionBusy[c.docker_id]}
                                      onClick={() => containerAction(c, 'stop')}
                                      className="btn btn-ghost btn-sm hover:text-crit disabled:opacity-40"
                                      title="Parar"
                                    >
                                      <Square size={16} strokeWidth={1.75} />
                                    </button>
                                  </>
                                ) : (
                                  <button
                                    disabled={actionBusy[c.docker_id]}
                                    onClick={() => containerAction(c, 'start')}
                                    className="btn btn-ghost btn-sm hover:text-ok disabled:opacity-40"
                                    title="Iniciar"
                                  >
                                    <Play size={16} strokeWidth={1.75} />
                                  </button>
                                ))}
                              </div>
                            </td>
                          </tr>
                        )
                      })}
                    </React.Fragment>
                  )
                })
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* Modal de logs */}
      {selectedContainer && liveSelected && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm">
          <div className="panel w-full max-w-4xl h-[80vh] flex flex-col overflow-hidden shadow-pop">
            <div className="flex items-center justify-between p-4 border-b border-line bg-ink-850">
              <div className="flex items-center gap-3">
                <Terminal size={18} strokeWidth={1.75} className="text-accent" />
                <h3 className="mono-data text-text-hi font-semibold text-sm">{liveSelected.name}</h3>
                <span className="badge badge-ok">
                  <span className="w-1.5 h-1.5 rounded-full bg-ok animate-pulse"></span>
                  ao vivo
                </span>
              </div>
              <button onClick={() => setSelectedContainer(null)} className="btn btn-ghost btn-sm" title="Fechar">
                <X size={18} strokeWidth={1.75} />
              </button>
            </div>

            {/* Consumo isolado do container */}
            {liveSelected.state === 'running' && (
              <div className="grid grid-cols-2 gap-3 p-4 border-b border-line bg-ink-900">
                <div className="flex items-center gap-3 p-3 rounded-ctrl border border-line bg-ink-850">
                  <Cpu size={18} strokeWidth={1.75} className="text-accent shrink-0" />
                  <div>
                    <div className="eyebrow">Uso de CPU</div>
                    <div className="stat-value text-lg">{liveSelected.cpu.toFixed(2)}%</div>
                  </div>
                </div>
                <div className="flex items-center gap-3 p-3 rounded-ctrl border border-line bg-ink-850">
                  <MemoryStick size={18} strokeWidth={1.75} className="text-info shrink-0" />
                  <div>
                    <div className="eyebrow">Memória RAM</div>
                    <div className="stat-value text-lg">
                      {formatBytes(liveSelected.mem_used)} <span className="text-sm text-text-faint">/ {formatBytes(liveSelected.mem_limit)}</span>
                    </div>
                  </div>
                </div>
              </div>
            )}

            {/* Terminal de logs */}
            <div className="flex-1 bg-ink-950 p-4 overflow-y-auto font-mono text-xs text-text leading-relaxed custom-scrollbar">
              {logs.length === 0 ? (
                <div className="text-text-faint">Conectando ao container via SSH e puxando os logs...</div>
              ) : (
                logs.map((log, i) => <div key={i} className="whitespace-pre-wrap break-all selectable">{log}</div>)
              )}
              <div ref={logsEndRef} />
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

export default ContainersView;
