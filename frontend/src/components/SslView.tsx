import React, { useState } from 'react';
import { Lock, Plus, Search, ShieldCheck, ShieldAlert, AlertTriangle, RefreshCw, Trash2, Globe } from 'lucide-react';

const SslView = () => {
  const [searchTerm, setSearchTerm] = useState('');

  const mockDomains = [
    { id: 1, domain: 'api.projeto.com', server: 'VPS Produção', status: 'valid', daysLeft: 68, issuer: 'Let\'s Encrypt' },
    { id: 2, domain: 'meusite.com.br', server: 'VPS Produção', status: 'warning', daysLeft: 12, issuer: 'Let\'s Encrypt' },
    { id: 3, domain: 'dev.sistema.io', server: 'Worker Node', status: 'expired', daysLeft: 0, issuer: 'Let\'s Encrypt' },
    { id: 4, domain: 'grafana.empresa.net', server: 'VPS Produção', status: 'valid', daysLeft: 84, issuer: 'Cloudflare' },
  ];

  const getStatusStyle = (status: string) => {
    switch (status) {
      case 'valid': return 'text-emerald-400 bg-emerald-400/10 border-emerald-400/20';
      case 'warning': return 'text-amber-400 bg-amber-400/10 border-amber-400/20';
      case 'expired': return 'text-rose-400 bg-rose-400/10 border-rose-400/20';
      default: return 'text-gray-400 bg-gray-400/10 border-gray-400/20';
    }
  };

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'valid': return <ShieldCheck size={16} />;
      case 'warning': return <AlertTriangle size={16} />;
      case 'expired': return <ShieldAlert size={16} />;
      default: return <Lock size={16} />;
    }
  };

  return (
    <div className="p-4 md:p-8 h-full flex flex-col overflow-hidden">
      <div className="mb-6 flex flex-col md:flex-row md:items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-light text-white mb-2 flex items-center gap-3">
            <Lock className="text-[#10b981]" /> SSL & <span className="font-bold">Domínios</span>
          </h1>
          <p className="text-[#737373] text-sm">Monitoramento de certificados TLS/SSL e validade de domínios.</p>
        </div>
        
        <button className="flex items-center gap-2 bg-[#10b981] hover:bg-[#059669] text-white px-4 py-2 rounded-lg font-medium transition-colors shadow-[0_0_15px_rgba(16,185,129,0.2)]">
          <Plus size={18} />
          <span>Monitorar Domínio</span>
        </button>
      </div>

      <div className="glass-panel rounded-xl border border-white/5 bg-white/[0.02] flex flex-col flex-1 min-h-0 overflow-hidden">
        {/* Toolbar */}
        <div className="p-4 border-b border-white/5 flex flex-col md:flex-row gap-4 justify-between items-center bg-black/20">
          <div className="relative w-full md:w-96">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500" size={18} />
            <input 
              type="text" 
              placeholder="Buscar domínio ou servidor..." 
              value={searchTerm}
              onChange={(e) => setSearchTerm(e.target.value)}
              className="w-full bg-[#0c0c0e] border border-white/10 rounded-lg pl-10 pr-4 py-2 text-sm text-gray-300 focus:outline-none focus:border-[#10b981]/50 focus:ring-1 focus:ring-[#10b981]/50 transition-all placeholder:text-gray-600"
            />
          </div>
          <button className="flex items-center gap-2 text-gray-400 hover:text-white transition-colors text-sm px-3 py-1.5 rounded-lg hover:bg-white/5">
            <RefreshCw size={16} />
            <span>Checar Todos Agora</span>
          </button>
        </div>

        {/* Table */}
        <div className="flex-1 overflow-auto custom-scrollbar">
          <table className="w-full text-left text-sm whitespace-nowrap">
            <thead className="bg-[#0c0c0e]/80 sticky top-0 z-10">
              <tr>
                <th className="py-3 px-4 font-medium text-gray-500">Domínio</th>
                <th className="py-3 px-4 font-medium text-gray-500">Servidor</th>
                <th className="py-3 px-4 font-medium text-gray-500">Emissor</th>
                <th className="py-3 px-4 font-medium text-gray-500">Validade</th>
                <th className="py-3 px-4 font-medium text-gray-500">Status</th>
                <th className="py-3 px-4 font-medium text-gray-500 text-right">Ações</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-white/5">
              {mockDomains.map(domain => (
                <tr key={domain.id} className="hover:bg-white/[0.02] transition-colors group">
                  <td className="py-3 px-4">
                    <div className="flex items-center gap-3">
                      <div className="w-8 h-8 rounded-full bg-white/5 flex items-center justify-center text-gray-400 border border-white/5 group-hover:border-white/10 transition-colors">
                        <Globe size={14} />
                      </div>
                      <span className="text-gray-200 font-medium">{domain.domain}</span>
                    </div>
                  </td>
                  <td className="py-3 px-4 text-gray-400">{domain.server}</td>
                  <td className="py-3 px-4 text-gray-400">{domain.issuer}</td>
                  <td className="py-3 px-4">
                    <div className="flex items-center gap-2">
                      <span className={`font-mono font-medium ${
                        domain.daysLeft > 30 ? 'text-emerald-400' : 
                        domain.daysLeft > 0 ? 'text-amber-400' : 'text-rose-400'
                      }`}>
                        {domain.daysLeft} dias
                      </span>
                    </div>
                  </td>
                  <td className="py-3 px-4">
                    <span className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-xs font-medium border ${getStatusStyle(domain.status)}`}>
                      {getStatusIcon(domain.status)}
                      {domain.status === 'valid' ? 'Válido' : domain.status === 'warning' ? 'Expira Breve' : 'Expirado'}
                    </span>
                  </td>
                  <td className="py-3 px-4 text-right">
                    <div className="flex items-center justify-end gap-2 opacity-0 group-hover:opacity-100 transition-opacity">
                      <button className="p-1.5 text-gray-400 hover:text-emerald-400 hover:bg-emerald-400/10 rounded transition-colors" title="Verificar SSL">
                        <RefreshCw size={16} />
                      </button>
                      <button className="p-1.5 text-gray-400 hover:text-rose-400 hover:bg-rose-400/10 rounded transition-colors" title="Remover monitoramento">
                        <Trash2 size={16} />
                      </button>
                    </div>
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

export default SslView;
