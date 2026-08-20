import React, { useState, useEffect, useRef } from 'react';
import { ShieldAlert, Activity, Server, AlertOctagon, Terminal, Shield, RefreshCw, XCircle } from 'lucide-react';

interface PortInfo {
  protocol: string;
  state: string;
  port: string;
  process: string;
}

interface AuthLog {
  time: string;
  raw: string;
  type: 'error' | 'success' | 'info';
}

const SecurityView = () => {
  const [activeTab, setActiveTab] = useState<'radar' | 'auth'>('radar');
  const [servers, setServers] = useState<any[]>([]);
  const [selectedServer, setSelectedServer] = useState<string>('');
  
  const [ports, setPorts] = useState<PortInfo[]>([]);
  const [loadingPorts, setLoadingPorts] = useState(false);
  
  const [authLogs, setAuthLogs] = useState<AuthLog[]>([]);
  const [streamActive, setStreamActive] = useState(false);
  const eventSourceRef = useRef<EventSource | null>(null);

  // Busca servidores iniciais
  useEffect(() => {
    fetch('http://localhost:8080/api/metrics/live')
      .then(res => res.json())
      .then(data => {
        if (data && data.servers && data.servers.length > 0) {
          // Filtra o Load Balancer, pois queremos VPS reais para SSH
          const vpsList = data.servers.filter((s: any) => s.name !== 'Load Balancer');
          setServers(vpsList);
          if (vpsList.length > 0) {
            setSelectedServer(vpsList[0].id);
          }
        }
      })
      .catch(console.error);
  }, []);

  // Efeito Radar de Portas
  useEffect(() => {
    if (activeTab === 'radar' && selectedServer) {
      setLoadingPorts(true);
      fetch(`http://localhost:8080/api/security/radar?server_id=${selectedServer}`)
        .then(res => res.json())
        .then(data => {
          setPorts(data || []);
          setLoadingPorts(false);
        })
        .catch(err => {
          console.error(err);
          setLoadingPorts(false);
        });
    }
  }, [activeTab, selectedServer]);

  // Efeito Stream Auth.log
  useEffect(() => {
    if (activeTab === 'auth' && selectedServer) {
      setAuthLogs([]);
      setStreamActive(true);
      
      const es = new EventSource(`http://localhost:8080/api/security/authlog/stream?server_id=${selectedServer}`);
      eventSourceRef.current = es;

      es.onmessage = (event) => {
        const line = event.data;
        let type: 'error' | 'success' | 'info' = 'info';
        if (line.includes('Failed') || line.includes('Invalid') || line.includes('error')) type = 'error';
        if (line.includes('Accepted')) type = 'success';
        
        const now = new Date().toLocaleTimeString('pt-BR');
        setAuthLogs(prev => [...prev.slice(-49), { time: now, raw: line, type }]);
      };

      es.onerror = () => {
        es.close();
        setStreamActive(false);
      };

      return () => {
        es.close();
        setStreamActive(false);
      };
    }
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
          <label className="text-gray-400 text-sm">Servidor Alvo:</label>
          <select 
            className="bg-[#0c0c0e] border border-white/10 text-white text-sm rounded-lg p-2.5 focus:border-[#10b981] focus:outline-none"
            value={selectedServer}
            onChange={e => setSelectedServer(e.target.value)}
          >
            {servers.map(s => (
              <option key={s.id} value={s.id}>{s.name} ({s.host_ip})</option>
            ))}
          </select>
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
                {ports.map((port, idx) => (
                  <tr key={idx} className="hover:bg-white/[0.02] transition-colors">
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
              {authLogs.map((log, idx) => (
                <div key={idx} className="mb-1.5 flex gap-3 break-all">
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
