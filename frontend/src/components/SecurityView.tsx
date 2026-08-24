import { useState, useEffect } from 'react';
import { Activity, Terminal, RefreshCw, XCircle } from 'lucide-react';
import { api, openStream, type PortInfo, type ServerLiveStat } from '../lib/api';
import Select from './ui/Select';

type AuthLogType = 'error' | 'success' | 'info';

interface AuthLog {
  // A lista é uma janela deslizante: o índice muda de linha a cada evento,
  // então precisa de id próprio para o React não remontar tudo.
  id: number;
  time: string;
  raw: string;
  type: AuthLogType;
}

const MAX_AUTH_LINES = 50;

const classifyAuthLine = (line: string): AuthLogType => {
  if (line.includes('Accepted')) return 'success';
  if (line.includes('Failed') || line.includes('Invalid') || line.includes('error')) return 'error';
  return 'info';
};

const AUTH_LINE_CLASS: Record<AuthLogType, string> = {
  error: 'text-crit',
  success: 'text-ok',
  info: 'text-text-mut',
};

const SecurityView = () => {
  const [activeTab, setActiveTab] = useState<'radar' | 'auth'>('radar');
  const [servers, setServers] = useState<ServerLiveStat[]>([]);
  const [selectedServer, setSelectedServer] = useState<string>('');

  const [ports, setPorts] = useState<PortInfo[]>([]);
  const [loadingPorts, setLoadingPorts] = useState(false);

  const [authLogs, setAuthLogs] = useState<AuthLog[]>([]);
  const [streamActive, setStreamActive] = useState(false);

  // Busca servidores iniciais
  useEffect(() => {
    const controller = new AbortController();
    api.liveMetrics(controller.signal)
      .then(data => {
        // Filtra o Load Balancer, pois queremos VPS reais para SSH
        const vpsList = data.servers.filter(s => s.name !== 'Load Balancer');
        setServers(vpsList);
        setSelectedServer(prev => prev || vpsList[0]?.id || '');
      })
      .catch(err => {
        if (!controller.signal.aborted) console.error(err);
      });
    return () => controller.abort();
  }, []);

  // Efeito Radar de Portas
  useEffect(() => {
    if (activeTab !== 'radar' || !selectedServer) return;

    const controller = new AbortController();
    setLoadingPorts(true);
    api.securityRadar(selectedServer, controller.signal)
      .then(data => {
        setPorts(data);
        setLoadingPorts(false);
      })
      .catch(err => {
        if (controller.signal.aborted) return;
        console.error(err);
        setLoadingPorts(false);
      });

    return () => controller.abort();
  }, [activeTab, selectedServer]);

  // Efeito Stream Auth.log
  useEffect(() => {
    if (activeTab !== 'auth' || !selectedServer) return;

    setAuthLogs([]);
    setStreamActive(true);

    // O ticket é buscado de forma assíncrona: se a aba mudar antes da
    // resposta, o stream nem chega a ser aberto.
    let source: EventSource | null = null;
    let cancelled = false;
    let lineId = 0;

    openStream('/api/security/authlog/stream', { server_id: selectedServer })
      .then(es => {
        if (cancelled) {
          es.close();
          return;
        }
        source = es;
        es.onmessage = (event) => {
          const raw = event.data;
          const time = new Date().toLocaleTimeString('pt-BR');
          const entry = { id: ++lineId, time, raw, type: classifyAuthLine(raw) };
          setAuthLogs(prev => [...prev, entry].slice(-MAX_AUTH_LINES));
        };
        es.onerror = () => {
          es.close();
          setStreamActive(false);
        };
      })
      .catch(() => setStreamActive(false));

    return () => {
      cancelled = true;
      source?.close();
      setStreamActive(false);
    };
  }, [activeTab, selectedServer]);

  return (
    <div className="p-4 md:p-8 h-full flex flex-col overflow-hidden anim-rise">
      <div className="page-header flex-col md:flex-row md:items-end items-start">
        <div>
          <h1 className="page-title">Segurança &amp; Auditoria</h1>
          <p className="page-desc">
            Portas expostas (ss -tulnp) e tentativas de intrusão no auth.log, ao vivo por SSH.
          </p>
        </div>

        <div className="flex items-center gap-3">
          <label htmlFor="security-server" className="eyebrow">Servidor</label>
          <Select
            id="security-server"
            value={selectedServer}
            onChange={setSelectedServer}
            className="min-w-[260px]"
            placeholder="Nenhum servidor"
            options={servers.map(s => ({ value: s.id, label: `${s.name} (${s.host_ip})` }))}
          />
        </div>
      </div>

      <div className="panel flex flex-col flex-1 min-h-0 overflow-hidden">
        {/* Tabs */}
        <div className="flex border-b border-line bg-ink-950/60">
          <button
            onClick={() => setActiveTab('radar')}
            className={`flex-1 py-3.5 text-sm font-medium transition-colors border-b-2 flex justify-center items-center gap-2 ${activeTab === 'radar' ? 'border-accent text-accent bg-accent/5' : 'border-transparent text-text-faint hover:text-text'}`}
          >
            <Activity size={16} strokeWidth={1.75} /> Radar de portas
          </button>
          <button
            onClick={() => setActiveTab('auth')}
            className={`flex-1 py-3.5 text-sm font-medium transition-colors border-b-2 flex justify-center items-center gap-2 ${activeTab === 'auth' ? 'border-accent text-accent bg-accent/5' : 'border-transparent text-text-faint hover:text-text'}`}
          >
            <Terminal size={16} strokeWidth={1.75} /> Logs de autenticação
          </button>
        </div>

        {/* Content */}
        <div className="flex-1 overflow-auto custom-scrollbar p-4">
          {activeTab === 'radar' ? (
            loadingPorts ? (
               <div className="flex justify-center items-center h-full text-text-mut gap-2 text-sm">
                  <RefreshCw className="animate-spin text-accent" size={18} strokeWidth={1.75} /> Rodando ss -tulnp na VPS...
               </div>
            ) : (
            <table className="table-base whitespace-nowrap">
              <thead>
                <tr>
                  <th>Porta</th>
                  <th>Protocolo</th>
                  <th>Serviço / Processo</th>
                  <th>Exposição</th>
                </tr>
              </thead>
              <tbody>
                {ports.map((port, idx) => (
                  <tr key={`${port.protocol}-${port.port}-${idx}`}>
                    <td className="mono-data text-text-hi">{port.port}</td>
                    <td className="text-text-mut uppercase text-xs">{port.protocol}</td>
                    <td>
                      <span className="mono-data text-text bg-ink-800 border border-line px-2 py-0.5 rounded">{port.process}</span>
                    </td>
                    <td>
                      {/* Porta em LISTEN para 0.0.0.0 é exatamente o achado que
                          esta tela existe para mostrar — nunca verde. */}
                      <span className="badge badge-warn">{port.state} 0.0.0.0</span>
                    </td>
                  </tr>
                ))}
                {ports.length === 0 && (
                   <tr>
                     <td colSpan={4} className="py-6 text-center text-text-faint">Nenhuma porta LISTEN exposta encontrada.</td>
                   </tr>
                )}
              </tbody>
            </table>
            )
          ) : (
            <div className="font-mono text-xs bg-ink-950 rounded-ctrl p-4 border border-line h-full overflow-y-auto custom-scrollbar">
              {authLogs.map((log) => (
                <div key={log.id} className="mb-1.5 flex gap-3 break-all selectable">
                  <span className="text-text-faint shrink-0">[{log.time}]</span>
                  <span className={AUTH_LINE_CLASS[log.type]}>
                    {log.raw}
                  </span>
                </div>
              ))}
              <div className="mt-4 flex items-center text-text-faint gap-2 border-t border-line pt-2">
                {streamActive ? (
                  <><RefreshCw size={14} className="animate-spin text-ok" /> <span className="text-ok/80">Túnel SSH aberto. Escutando /var/log/auth.log...</span></>
                ) : (
                  <><XCircle size={14} className="text-crit" /> <span className="text-crit/80">Conexão SSE fechada.</span></>
                )}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};

export default SecurityView;
