import React from 'react';
import { Globe, ArrowUpRight, ArrowDownRight, Server, Activity, Network } from 'lucide-react';

const NginxView = () => {
  const mockVHosts = [
    { domain: 'api.projeto.com', upstream: '127.0.0.1:8080', status: 'healthy', reqs: '124 req/s' },
    { domain: 'sistema.io', upstream: '127.0.0.1:3000', status: 'healthy', reqs: '42 req/s' },
    { domain: 'blog.empresa.net', upstream: '127.0.0.1:8000', status: 'degraded', reqs: '12 req/s' },
  ];

  return (
    <div className="p-4 md:p-8 h-full flex flex-col overflow-hidden">
      <div className="mb-6 flex flex-col md:flex-row md:items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-light text-white mb-2 flex items-center gap-3">
            <Globe className="text-[#10b981]" /> Nginx & <span className="font-bold">Tráfego</span>
          </h1>
          <p className="text-[#737373] text-sm">Monitoramento de tráfego de rede e roteamento reverso.</p>
        </div>
      </div>

      {/* Resumo Tráfego */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mb-6">
        <div className="glass-panel rounded-xl p-6 border border-white/5 bg-white/[0.02] flex items-center gap-6">
          <div className="w-14 h-14 rounded-full bg-emerald-500/10 flex items-center justify-center text-emerald-400 border border-emerald-500/20">
            <ArrowDownRight size={24} />
          </div>
          <div>
            <h3 className="text-gray-400 font-medium text-sm mb-1">Download (RX)</h3>
            <div className="text-3xl font-bold text-white">45.2 <span className="text-lg text-gray-500 font-normal">MB/s</span></div>
          </div>
        </div>
        <div className="glass-panel rounded-xl p-6 border border-white/5 bg-white/[0.02] flex items-center gap-6">
          <div className="w-14 h-14 rounded-full bg-indigo-500/10 flex items-center justify-center text-indigo-400 border border-indigo-500/20">
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
        <div className="p-4 border-b border-white/5 bg-black/20 flex items-center gap-2">
          <Network className="text-[#10b981]" size={18} />
          <h2 className="text-white font-medium">Virtual Hosts (Nginx)</h2>
        </div>

        {/* Table */}
        <div className="flex-1 overflow-auto custom-scrollbar p-4">
          <table className="w-full text-left text-sm whitespace-nowrap">
            <thead className="text-gray-500">
              <tr>
                <th className="py-2 px-4 font-medium">Domínio / Host</th>
                <th className="py-2 px-4 font-medium">Upstream (Proxy Pass)</th>
                <th className="py-2 px-4 font-medium">Requisições</th>
                <th className="py-2 px-4 font-medium">Status Health</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-white/5">
              {mockVHosts.map((host, idx) => (
                <tr key={idx} className="hover:bg-white/[0.02] transition-colors">
                  <td className="py-3 px-4">
                    <div className="flex items-center gap-2">
                      <Globe size={14} className="text-gray-500" />
                      <span className="text-gray-200 font-medium">{host.domain}</span>
                    </div>
                  </td>
                  <td className="py-3 px-4 font-mono text-gray-400">{host.upstream}</td>
                  <td className="py-3 px-4">
                    <span className="text-gray-300 bg-white/5 px-2 py-1 rounded text-xs">{host.reqs}</span>
                  </td>
                  <td className="py-3 px-4">
                    {host.status === 'healthy' ? (
                      <span className="text-emerald-400 text-xs bg-emerald-400/10 px-2 py-1 rounded border border-emerald-400/20">Saudável</span>
                    ) : (
                      <span className="text-amber-400 text-xs bg-amber-400/10 px-2 py-1 rounded border border-amber-400/20">Degradado</span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
};

export default NginxView;
