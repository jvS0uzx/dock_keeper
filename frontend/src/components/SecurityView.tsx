import React, { useState } from 'react';
import { ShieldAlert, Activity, Server, AlertOctagon, Terminal, Shield, RefreshCw } from 'lucide-react';

const SecurityView = () => {
  const [activeTab, setActiveTab] = useState<'radar' | 'auth'>('radar');

  const mockPorts = [
    { port: 22, protocol: 'tcp', service: 'sshd', process: 'systemd', state: 'LISTEN' },
    { port: 80, protocol: 'tcp', service: 'nginx', process: 'nginx', state: 'LISTEN' },
    { port: 443, protocol: 'tcp', service: 'nginx', process: 'nginx', state: 'LISTEN' },
    { port: 5432, protocol: 'tcp', service: 'postgres', process: 'postgres', state: 'LISTEN' },
    { port: 6379, protocol: 'tcp', service: 'redis-server', process: 'redis', state: 'LISTEN' },
  ];

  const mockAuthLogs = [
    { time: '10:42:15', ip: '192.168.1.100', user: 'root', status: 'Failed password', type: 'error' },
    { time: '10:41:20', ip: '203.0.113.5', user: 'admin', status: 'Failed password', type: 'error' },
    { time: '10:30:05', ip: '203.0.113.22', user: 'joao', status: 'Accepted publickey', type: 'success' },
    { time: '09:15:00', ip: '10.0.0.5', user: 'root', status: 'Connection closed', type: 'info' },
  ];

  return (
    <div className="p-4 md:p-8 h-full flex flex-col overflow-hidden">
      <div className="mb-6 flex flex-col md:flex-row md:items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-light text-white mb-2 flex items-center gap-3">
            <ShieldAlert className="text-[#10b981]" /> Segurança & <span className="font-bold">Auditoria</span>
          </h1>
          <p className="text-[#737373] text-sm">Monitoramento de portas expostas (ss -tuln) e tentativas de intrusão.</p>
        </div>
      </div>

      {/* Resumo */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
        <div className="glass-panel rounded-xl p-5 border border-white/5 bg-white/[0.02]">
          <div className="flex items-center justify-between mb-4">
            <h3 className="text-gray-400 font-medium text-sm">Portas Expostas</h3>
            <div className="p-2 bg-indigo-500/10 rounded-lg text-indigo-400">
              <Activity size={18} />
            </div>
          </div>
          <div className="text-3xl font-bold text-white">5</div>
          <p className="text-xs text-gray-500 mt-2">Abertas para 0.0.0.0</p>
        </div>
        <div className="glass-panel rounded-xl p-5 border border-white/5 bg-white/[0.02]">
          <div className="flex items-center justify-between mb-4">
            <h3 className="text-gray-400 font-medium text-sm">Tentativas SSH (Hoje)</h3>
            <div className="p-2 bg-rose-500/10 rounded-lg text-rose-400">
              <AlertOctagon size={18} />
            </div>
          </div>
          <div className="text-3xl font-bold text-white">124</div>
          <p className="text-xs text-gray-500 mt-2">Falhas de autenticação detectadas</p>
        </div>
        <div className="glass-panel rounded-xl p-5 border border-white/5 bg-white/[0.02]">
          <div className="flex items-center justify-between mb-4">
            <h3 className="text-gray-400 font-medium text-sm">Status UFW/Fail2Ban</h3>
            <div className="p-2 bg-emerald-500/10 rounded-lg text-emerald-400">
              <Shield size={18} />
            </div>
          </div>
          <div className="text-xl font-bold text-emerald-400 flex items-center gap-2">
            Ativo
          </div>
          <p className="text-xs text-gray-500 mt-2">Proteção ativa e monitorando</p>
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
            <table className="w-full text-left text-sm whitespace-nowrap">
              <thead className="text-gray-500">
                <tr>
                  <th className="py-2 px-4 font-medium">Porta</th>
                  <th className="py-2 px-4 font-medium">Protocolo</th>
                  <th className="py-2 px-4 font-medium">Serviço / Processo</th>
                  <th className="py-2 px-4 font-medium">Estado</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/5">
                {mockPorts.map((port, idx) => (
                  <tr key={idx} className="hover:bg-white/[0.02] transition-colors">
                    <td className="py-3 px-4 font-mono text-gray-300">{port.port}</td>
                    <td className="py-3 px-4 text-gray-400 uppercase">{port.protocol}</td>
                    <td className="py-3 px-4 text-gray-300">
                      <span className="bg-white/10 px-2 py-1 rounded text-xs mr-2">{port.service}</span>
                      <span className="text-gray-500">{port.process}</span>
                    </td>
                    <td className="py-3 px-4">
                      <span className="text-emerald-400 text-xs bg-emerald-400/10 px-2 py-1 rounded border border-emerald-400/20">{port.state}</span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          ) : (
            <div className="font-mono text-sm bg-[#0c0c0e] rounded-lg p-4 border border-white/5">
              {mockAuthLogs.map((log, idx) => (
                <div key={idx} className="mb-2 flex gap-3">
                  <span className="text-gray-600">{log.time}</span>
                  <span className="text-blue-400 w-32">{log.ip}</span>
                  <span className="text-amber-300 w-24">user: {log.user}</span>
                  <span className={log.type === 'error' ? 'text-rose-400' : log.type === 'success' ? 'text-emerald-400' : 'text-gray-400'}>
                    {log.status}
                  </span>
                </div>
              ))}
              <div className="mt-4 flex items-center text-gray-500 gap-2">
                <RefreshCw size={14} className="animate-spin" /> Escutando novos eventos...
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};

export default SecurityView;
