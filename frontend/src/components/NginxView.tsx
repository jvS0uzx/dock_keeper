import React, { useEffect, useState } from 'react';
import { Globe, ArrowUpRight, ArrowDownRight, Server, Activity, Network } from 'lucide-react';

interface LbStat {
  upstream_addr: string;
  server_name: string;
  status: string;
  requests_count: number;
}

const NginxView = () => {
  const [loadBalancing, setLoadBalancing] = useState<LbStat[]>([]);

  useEffect(() => {
    const fetchMetrics = () => {
      fetch('http://localhost:8080/api/metrics/live')
        .then(res => res.json())
        .then(data => {
          if (data && data.load_balancing) {
            setLoadBalancing(data.load_balancing);
          }
        })
        .catch(err => console.error("Erro API Nginx:", err));
    };
    
    fetchMetrics();
    const interval = setInterval(fetchMetrics, 2000);
    return () => clearInterval(interval);
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

      {/* Resumo Tráfego (Mock RX/TX, será implementado na Fase de Rede) */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mb-6">
        <div className="glass-panel rounded-xl p-6 border border-white/5 bg-white/[0.02] flex items-center gap-6">
          <div className="w-14 h-14 rounded-full bg-emerald-500/10 flex items-center justify-center text-emerald-400 border border-emerald-500/20 shadow-[0_0_15px_rgba(16,185,129,0.1)]">
            <ArrowDownRight size={24} />
          </div>
          <div>
            <h3 className="text-gray-400 font-medium text-sm mb-1">Download (RX)</h3>
            <div className="text-3xl font-bold text-white">45.2 <span className="text-lg text-gray-500 font-normal">MB/s</span></div>
          </div>
        </div>
        <div className="glass-panel rounded-xl p-6 border border-white/5 bg-white/[0.02] flex items-center gap-6">
          <div className="w-14 h-14 rounded-full bg-indigo-500/10 flex items-center justify-center text-indigo-400 border border-indigo-500/20 shadow-[0_0_15px_rgba(99,102,241,0.1)]">
            <ArrowUpRight size={24} />
          </div>
          <div>
            <h3 className="text-gray-400 font-medium text-sm mb-1">Upload (TX)</h3>
            <div className="text-3xl font-bold text-white">12.8 <span className="text-lg text-gray-500 font-normal">MB/s</span></div>
          </div>
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
              {loadBalancing.map((host, idx) => {
                const isError = ['500', '502', '503', '504'].includes(host.status);
                const isWarn = ['400', '404', '429'].includes(host.status);
                
                return (
                  <tr key={idx} className="hover:bg-white/[0.02] transition-colors">
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
