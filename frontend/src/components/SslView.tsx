import React, { useState, useEffect } from 'react';
import { Lock, Plus, Search, ShieldCheck, ShieldAlert, AlertTriangle, RefreshCw, Trash2, Globe } from 'lucide-react';

interface DomainItem {
  id: number;
  domain: string;
  server: string;
  status: string;
  daysLeft: number;
  issuer: string;
}

const SslView = () => {
  const [searchTerm, setSearchTerm] = useState('');
  const [domains, setDomains] = useState<DomainItem[]>([]);
  const [loading, setLoading] = useState(true);

  const fetchDomains = async () => {
    try {
      const res = await fetch('http://localhost:8080/api/ssl/domains');
      const data = await res.json();
      if (data && Array.isArray(data)) {
        const mapped = data.map((d: any) => ({
          id: d.id,
          domain: d.domain,
          server: d.server_id ? 'Vinculado' : 'Externo', // Simple mock for server name
          status: 'loading',
          daysLeft: 0,
          issuer: '...',
        }));
        setDomains(mapped);
        mapped.forEach(d => checkDomain(d.id, d.domain));
      }
    } catch (e) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchDomains();
  }, []);

  const checkDomain = async (id: number, domainName: string) => {
    try {
      const res = await fetch(`http://localhost:8080/api/ssl/check?domain=${domainName}`);
      const data = await res.json();
      
      setDomains(prev => prev.map(d => {
        if (d.id === id) {
          return {
            ...d,
            daysLeft: data.days_left,
            issuer: data.issuer || 'Desconhecido',
            status: data.valid ? (data.days_left < 15 ? 'warning' : 'valid') : 'expired'
          };
        }
        return d;
      }));
    } catch (e) {
      setDomains(prev => prev.map(d => d.id === id ? { ...d, status: 'error', issuer: 'Erro' } : d));
    }
  };

  const checkAll = () => {
    setDomains(prev => prev.map(d => ({ ...d, status: 'loading' })));
    domains.forEach(d => checkDomain(d.id, d.domain));
  };

  const addDomain = async () => {
    const domainName = window.prompt("Digite o domínio que deseja monitorar (ex: app.empresa.com):");
    if (!domainName) return;
    
    try {
      const res = await fetch('http://localhost:8080/api/ssl/domains', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ domain: domainName, server_id: '' })
      });
      if (res.ok) {
        fetchDomains();
      }
    } catch (e) {
      alert("Erro ao adicionar domínio");
    }
  };

  const deleteDomain = async (id: number) => {
    if (!window.confirm("Deseja remover este domínio do monitoramento?")) return;
    try {
      await fetch(`http://localhost:8080/api/ssl/domains?id=${id}`, { method: 'DELETE' });
      fetchDomains();
    } catch (e) {
      alert("Erro ao deletar");
    }
  };

  const getStatusStyle = (status: string) => {
    switch (status) {
      case 'valid': return 'text-emerald-400 bg-emerald-400/10 border-emerald-400/20';
      case 'warning': return 'text-amber-400 bg-amber-400/10 border-amber-400/20';
      case 'expired': return 'text-rose-400 bg-rose-400/10 border-rose-400/20';
      case 'loading': return 'text-blue-400 bg-blue-400/10 border-blue-400/20';
      default: return 'text-gray-400 bg-gray-400/10 border-gray-400/20';
    }
  };

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'valid': return <ShieldCheck size={16} />;
      case 'warning': return <AlertTriangle size={16} />;
      case 'expired': return <ShieldAlert size={16} />;
      case 'loading': return <RefreshCw size={16} className="animate-spin" />;
      default: return <Lock size={16} />;
    }
  };

  const filteredDomains = domains.filter(d => 
    d.domain.toLowerCase().includes(searchTerm.toLowerCase())
  );

  return (
    <div className="p-4 md:p-8 h-full flex flex-col overflow-hidden">
      <div className="mb-6 flex flex-col md:flex-row md:items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-light text-white mb-2 flex items-center gap-3">
            <Lock className="text-[#10b981]" /> SSL & <span className="font-bold">Domínios (Live)</span>
          </h1>
          <p className="text-[#737373] text-sm">Monitoramento de certificados TLS/SSL dinâmico em tempo real.</p>
        </div>
        
        <button onClick={addDomain} className="flex items-center gap-2 bg-[#10b981] hover:bg-[#059669] text-white px-4 py-2 rounded-lg font-medium transition-colors shadow-[0_0_15px_rgba(16,185,129,0.2)]">
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
              placeholder="Buscar domínio..." 
              value={searchTerm}
              onChange={(e) => setSearchTerm(e.target.value)}
              className="w-full bg-[#0c0c0e] border border-white/10 rounded-lg pl-10 pr-4 py-2 text-sm text-gray-300 focus:outline-none focus:border-[#10b981]/50 focus:ring-1 focus:ring-[#10b981]/50 transition-all placeholder:text-gray-600"
            />
          </div>
          <button onClick={checkAll} className="flex items-center gap-2 text-gray-400 hover:text-white transition-colors text-sm px-3 py-1.5 rounded-lg hover:bg-white/5 border border-white/10">
            <RefreshCw size={16} />
            <span>Forçar Handshake SSL</span>
          </button>
        </div>

        {/* Table */}
        <div className="flex-1 overflow-auto custom-scrollbar">
          <table className="w-full text-left text-sm whitespace-nowrap">
            <thead className="bg-[#0c0c0e]/80 sticky top-0 z-10">
              <tr>
                <th className="py-3 px-4 font-medium text-gray-500">Domínio</th>
                <th className="py-3 px-4 font-medium text-gray-500">Emissor</th>
                <th className="py-3 px-4 font-medium text-gray-500">Validade</th>
                <th className="py-3 px-4 font-medium text-gray-500">Status</th>
                <th className="py-3 px-4 font-medium text-gray-500 text-right">Ações</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-white/5">
              {loading && <tr><td colSpan={5} className="py-8 text-center text-gray-500">Carregando domínios...</td></tr>}
              {!loading && filteredDomains.length === 0 && (
                <tr><td colSpan={5} className="py-8 text-center text-gray-500">Nenhum domínio cadastrado. Clique em "Monitorar Domínio" para adicionar um domínio real.</td></tr>
              )}
              {filteredDomains.map(domain => (
                <tr key={domain.id} className="hover:bg-white/[0.02] transition-colors group">
                  <td className="py-3 px-4">
                    <div className="flex items-center gap-3">
                      <div className="w-8 h-8 rounded-full bg-white/5 flex items-center justify-center text-gray-400 border border-white/5 group-hover:border-white/10 transition-colors">
                        <Globe size={14} />
                      </div>
                      <span className="text-gray-200 font-medium">{domain.domain}</span>
                    </div>
                  </td>
                  <td className="py-3 px-4 text-gray-400">{domain.issuer}</td>
                  <td className="py-3 px-4">
                    <div className="flex items-center gap-2">
                      <span className={`font-mono font-medium ${
                        domain.daysLeft > 30 ? 'text-emerald-400' : 
                        domain.daysLeft > 0 ? 'text-amber-400' : 'text-rose-400'
                      }`}>
                        {domain.status === 'loading' ? 'Checando...' : `${domain.daysLeft} dias`}
                      </span>
                    </div>
                  </td>
                  <td className="py-3 px-4">
                    <span className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-xs font-medium border ${getStatusStyle(domain.status)}`}>
                      {getStatusIcon(domain.status)}
                      {domain.status === 'valid' ? 'Válido' : domain.status === 'warning' ? 'Expira Breve' : domain.status === 'loading' ? 'Handshake...' : 'Expirado/Erro'}
                    </span>
                  </td>
                  <td className="py-3 px-4 text-right">
                    <div className="flex items-center justify-end gap-2 opacity-0 group-hover:opacity-100 transition-opacity">
                      <button onClick={() => {
                        setDomains(prev => prev.map(d => d.id === domain.id ? { ...d, status: 'loading' } : d));
                        checkDomain(domain.id, domain.domain);
                      }} className="p-1.5 text-gray-400 hover:text-emerald-400 hover:bg-emerald-400/10 rounded transition-colors" title="Verificar SSL">
                        <RefreshCw size={16} />
                      </button>
                      <button onClick={() => deleteDomain(domain.id)} className="p-1.5 text-gray-400 hover:text-rose-400 hover:bg-rose-400/10 rounded transition-colors" title="Remover monitoramento">
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
