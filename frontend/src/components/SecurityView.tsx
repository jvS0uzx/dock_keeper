import { useState, useEffect } from 'react';
import { ShieldAlert, Activity, Terminal, RefreshCw, XCircle } from 'lucide-react';
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
    <div className="p-4 md:p-8 h-full flex flex-col overflow-hidden">
      <div className="mb-6 flex flex-col md:flex-row md:items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-light text-white mb-2 flex items-center gap-3">
            <ShieldAlert className="text-[#10b981]" /> Segurança & <span className="font-bold">Auditoria (Live SSH)</span>
          </h1>
          <p className="text-[#737373] text-sm">Monitoramento de portas expostas (ss -tuln) e tentativas de intrusão via syslog em tempo real.</p>
        </div>
        
        <div className="flex items-center gap-2">
          <label htmlFor="security-server" className="text-gray-400 text-sm">Servidor Alvo:</label>
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

      <div className="glass-panel rounded-xl border border-white/5 bg-white/[0.02] flex flex-col flex-1 min-h-0 overflow-hidden">
        {/* Tabs */}
        <div className="flex border-b border-white/5 bg-black/20">
          <button 
            onClick={() => setActiveTab('radar')}
            className={`flex-1 py-4 text-sm font-medium transition-colors border-b-2 flex justify-center items-center gap-2 ${activeTab === 'radar' ? 'border-[#10b981] text-[#10b981] bg-[#10b981]/5' : 'border-transparent text-gray-500 hover:text-gray-300'}`}
          >
            <Activity size={16} /> Radar de Portas (ss -tuln)
          </button>
          <button 
            onClick={() => setActiveTab('auth')}
            className={`flex-1 py-4 text-sm font-medium transition-colors border-b-2 flex justify-center items-center gap-2 ${activeTab === 'auth' ? 'border-[#10b981] text-[#10b981] bg-[#10b981]/5' : 'border-transparent text-gray-500 hover:text-gray-300'}`}
          >
            <Terminal size={16} /> Logs de Autenticação (auth.log)
          </button>
        </div>

        {/* Content */}
        <div className="flex-1 overflow-auto custom-scrollbar p-4">
          {activeTab === 'radar' ? (
            loadingPorts ? (
               <div className="flex justify-center items-center h-full text-emerald-400">
                  <RefreshCw className="animate-spin mr-2" size={20} /> Rodando ss -tuln na VPS...
               </div>
            ) : (
            <table className="w-full text-left text-sm whitespace-nowrap">
              <thead className="text-gray-500">
                <tr>
                  <th className="py-2 px-4 font-medium">Porta (0.0.0.0)</th>
                  <th className="py-2 px-4 font-medium">Protocolo</th>
                  <th className="py-2 px-4 font-medium">Serviço / Processo</th>
                  <th className="py-2 px-4 font-medium">Estado</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/5">
                {ports.map((port) => (
                  <tr key={`${port.protocol}-${port.port}`} className="hover:bg-white/[0.02] transition-colors">
                    <td className="py-3 px-4 font-mono text-gray-300 border-l-2 border-transparent hover:border-emerald-500">{port.port}</td>
                    <td className="py-3 px-4 text-gray-400 uppercase">{port.protocol}</td>
                    <td className="py-3 px-4 text-gray-300">
                      <span className="bg-white/10 px-2 py-1 rounded text-xs text-amber-300">{port.process}</span>
                    </td>
                    <td className="py-3 px-4">
                      <span className="text-emerald-400 text-xs bg-emerald-400/10 px-2 py-1 rounded border border-emerald-400/20">{port.state}</span>
                    </td>
                  </tr>
                ))}
                {ports.length === 0 && (
                   <tr>
                     <td colSpan={4} className="py-6 text-center text-gray-500">Nenhuma porta LISTEN exposta encontrada.</td>
                   </tr>
                )}
              </tbody>
            </table>
            )
          ) : (
            <div className="font-mono text-sm bg-[#0c0c0e] rounded-lg p-4 border border-white/5 h-full overflow-y-auto">
              {authLogs.map((log) => (
                <div key={log.id} className="mb-1.5 flex gap-3 break-all selectable">
                  <span className="text-gray-600 shrink-0">[{log.time}]</span>
                  <span className={log.type === 'error' ? 'text-rose-400' : log.type === 'success' ? 'text-emerald-400' : 'text-gray-400'}>
                    {log.raw}
                  </span>
                </div>
              ))}
              <div className="mt-4 flex items-center text-gray-500 gap-2 border-t border-white/5 pt-2">
                {streamActive ? (
                  <><RefreshCw size={14} className="animate-spin text-emerald-500" /> <span className="text-emerald-500/70">Túnel SSH Aberto. Escutando /var/log/auth.log...</span></>
                ) : (
                  <><XCircle size={14} className="text-rose-500" /> <span className="text-rose-500/70">Conexão SSE Fechada.</span></>
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
