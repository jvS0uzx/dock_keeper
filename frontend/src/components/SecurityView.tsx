import LoadNotice from './ui/LoadNotice';
import { useLoadStatus } from './ui/load-status';
import { useState, useEffect, useRef } from 'react';
import { Activity, Terminal, RefreshCw, XCircle } from 'lucide-react';
import { api, type PortInfo, type ServerLiveStat } from '../lib/api';
import { useStreamComReconexao } from './ui/stream-reconnect';
import Select from './ui/Select';

type AuthLogType = 'error' | 'success' | 'info';

interface AuthLog {
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
  const carga = useLoadStatus();
  const { ok: cargaOk, fail: cargaFail } = carga;
  const radar = useLoadStatus();
  const { ok: radarOk, fail: radarFail } = radar;
  const [selectedServer, setSelectedServer] = useState<string>('');

  const [ports, setPorts] = useState<PortInfo[]>([]);
  const [loadingPorts, setLoadingPorts] = useState(false);

  const [authLogs, setAuthLogs] = useState<AuthLog[]>([]);
  const lineId = useRef(0);

  useEffect(() => {
    const controller = new AbortController();
    api.liveMetrics(controller.signal)
      .then(data => {
        const vpsList = data.servers.filter(s => s.name !== 'Load Balancer');
        setServers(vpsList);
        setSelectedServer(prev => prev || vpsList[0]?.id || '');
        cargaOk();
      })
      .catch(err => {
        if (!controller.signal.aborted) cargaFail(err, 'Falha ao listar os servidores.');
      });
    return () => controller.abort();
  }, [cargaOk, cargaFail]);

  useEffect(() => {
    if (activeTab !== 'radar' || !selectedServer) return;

    const controller = new AbortController();
    setLoadingPorts(true);
    api.securityRadar(selectedServer, controller.signal)
      .then(data => {
        setPorts(data);
        setLoadingPorts(false);
        radarOk();
      })
      .catch(err => {
        if (controller.signal.aborted) return;
        setPorts([]);
        radarFail(err, 'Falha ao ler as portas deste servidor.');
        setLoadingPorts(false);
      });

    return () => controller.abort();
  }, [activeTab, selectedServer, radarOk, radarFail]);

  const authStream = useStreamComReconexao({
    path: '/api/security/authlog/stream',
    params: activeTab === 'auth' && selectedServer ? { server_id: selectedServer } : null,
    onMessage: (raw) => {
      const time = new Date().toLocaleTimeString('pt-BR');
      lineId.current += 1;
      setAuthLogs(prev => [...prev, { id: lineId.current, time, raw, type: classifyAuthLine(raw) }].slice(-MAX_AUTH_LINES));
    },
  });

  useEffect(() => {
    setAuthLogs([]);
  }, [activeTab, selectedServer]);

  return (
    <div className="p-4 md:p-8 h-full flex flex-col overflow-hidden anim-rise">
      <LoadNotice error={carga.error} lastOk={carga.lastOk} className="mb-4" />
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
                      <span className="badge badge-warn">{port.state} 0.0.0.0</span>
                    </td>
                  </tr>
                ))}
                {ports.length === 0 && (
                   <tr>
                     <td colSpan={4} className="py-6 text-center text-text-faint">{radar.error ? radar.error : 'Nenhuma porta LISTEN exposta encontrada.'}</td>
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
                {authStream.conectado && (
                  <><RefreshCw size={14} className="animate-spin text-ok" /> <span className="text-ok/80">Túnel SSH aberto. Escutando /var/log/auth.log...</span></>
                )}
                {authStream.reconectando && (
                  <><RefreshCw size={14} className="animate-spin text-warn" /> <span className="text-warn/90">Conexão caiu. Reconectando ao auth.log...</span></>
                )}
                {!authStream.conectado && !authStream.reconectando && (
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
