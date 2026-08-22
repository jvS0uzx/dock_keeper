import React, { useState, useEffect, useMemo } from 'react';
import { API_URL } from '../config';
import { Lock, Plus, Search, ShieldCheck, ShieldAlert, AlertTriangle, RefreshCw, Trash2, Globe, Clock } from 'lucide-react';

// Espelha o model Domain do backend (estado persistido pelo worker de SSL).
interface DomainItem {
  id: number;
  domain: string;
  server_id: string;
  valid: boolean;
  issuer: string;
  days_left: number;
  error_msg: string;
  last_check: string | null;
}

type Status = 'valid' | 'warning' | 'expired' | 'pending';

const statusOf = (d: DomainItem): Status => {
  if (!d.last_check) return 'pending';
  if (!d.valid) return 'expired';
  if (d.days_left <= 14) return 'warning';
  return 'valid';
};

const relativeTime = (iso: string | null): string => {
  if (!iso) return 'nunca';
  const diff = Date.now() - new Date(iso).getTime();
  const s = Math.floor(diff / 1000);
  if (s < 60) return `há ${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `há ${m}min`;
  const h = Math.floor(m / 60);
  if (h < 24) return `há ${h}h`;
  return `há ${Math.floor(h / 24)}d`;
};

const SslView = () => {
  const [searchTerm, setSearchTerm] = useState('');
  const [domains, setDomains] = useState<DomainItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState<Record<number, boolean>>({});
  const [checkingAll, setCheckingAll] = useState(false);

  const fetchDomains = async () => {
    try {
      const res = await fetch(API_URL + '/api/ssl/domains');
      const data = await res.json();
      if (Array.isArray(data)) setDomains(data);
    } catch (e) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  };

  // Polling ao vivo: reflete o que o worker vai persistindo em background.
  useEffect(() => {
    fetchDomains();
    const interval = setInterval(fetchDomains, 10000);
    return () => clearInterval(interval);
  }, []);

  const recheckOne = async (id: number) => {
    setBusy(prev => ({ ...prev, [id]: true }));
    try {
      const res = await fetch(`${API_URL}/api/ssl/recheck?id=${id}`, { method: 'POST' });
      if (res.ok) {
        const updated: DomainItem = await res.json();
        setDomains(prev => prev.map(d => (d.id === id ? updated : d)));
      }
    } catch (e) {
      console.error(e);
    } finally {
      setBusy(prev => ({ ...prev, [id]: false }));
    }
  };

  const recheckAll = async () => {
    setCheckingAll(true);
    try {
      await fetch(`${API_URL}/api/ssl/recheck-all`, { method: 'POST' });
      // Dá tempo do backend fazer os handshakes e busca o resultado persistido.
      setTimeout(fetchDomains, 2500);
    } catch (e) {
      console.error(e);
    } finally {
      setTimeout(() => setCheckingAll(false), 2500);
    }
  };

  const addDomain = async () => {
    const domainName = window.prompt('Domínio a monitorar (ex: app.empresa.com):');
    if (!domainName) return;
    try {
      const res = await fetch(API_URL + '/api/ssl/domains', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ domain: domainName.trim(), server_id: '' }),
      });
      if (res.ok) setTimeout(fetchDomains, 1500); // backend checa na hora
    } catch (e) {
      alert('Erro ao adicionar domínio');
    }
  };

  const deleteDomain = async (id: number) => {
    if (!window.confirm('Remover este domínio do monitoramento?')) return;
    try {
      await fetch(`${API_URL}/api/ssl/domains?id=${id}`, { method: 'DELETE' });
      setDomains(prev => prev.filter(d => d.id !== id));
    } catch (e) {
      alert('Erro ao deletar');
    }
  };

  const summary = useMemo(() => {
    const acc = { valid: 0, warning: 0, expired: 0, pending: 0 };
    domains.forEach(d => { acc[statusOf(d)]++; });
    return acc;
  }, [domains]);

  const filteredDomains = domains.filter(d =>
    d.domain.toLowerCase().includes(searchTerm.toLowerCase())
  );

  const statusStyle = (s: Status) => ({
    valid: 'text-emerald-400 bg-emerald-400/10 border-emerald-400/20',
    warning: 'text-amber-400 bg-amber-400/10 border-amber-400/20',
    expired: 'text-rose-400 bg-rose-400/10 border-rose-400/20',
    pending: 'text-blue-400 bg-blue-400/10 border-blue-400/20',
  }[s]);

  const statusIcon = (s: Status) => ({
    valid: <ShieldCheck size={16} />,
    warning: <AlertTriangle size={16} />,
    expired: <ShieldAlert size={16} />,
    pending: <RefreshCw size={16} className="animate-spin" />,
  }[s]);

  const statusLabel = (s: Status) => ({
    valid: 'Válido', warning: 'Expira Breve', expired: 'Inválido/Expirado', pending: 'Checando...',
  }[s]);

  return (
    <div className="p-4 md:p-8 h-full flex flex-col overflow-hidden">
      <div className="mb-6 flex flex-col md:flex-row md:items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-light text-white mb-2 flex items-center gap-3">
            <Lock className="text-[#10b981]" /> SSL & <span className="font-bold">Domínios (Live)</span>
          </h1>
          <p className="text-[#737373] text-sm">Monitoramento contínuo de certificados TLS — revalidação automática a cada 30min.</p>
        </div>
        <button onClick={addDomain} className="flex items-center gap-2 bg-[#10b981] hover:bg-[#059669] text-white px-4 py-2 rounded-lg font-medium transition-colors shadow-[0_0_15px_rgba(16,185,129,0.2)]">
          <Plus size={18} />
          <span>Monitorar Domínio</span>
        </button>
      </div>

      {/* Resumo */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-3 mb-6">
        {([
          ['valid', 'Válidos', summary.valid, 'text-emerald-400 border-emerald-400/20'],
          ['warning', 'Expirando', summary.warning, 'text-amber-400 border-amber-400/20'],
          ['expired', 'Inválidos', summary.expired, 'text-rose-400 border-rose-400/20'],
          ['pending', 'Pendentes', summary.pending, 'text-blue-400 border-blue-400/20'],
        ] as const).map(([key, label, count, cls]) => (
          <div key={key} className={`glass-panel rounded-xl p-4 border bg-white/[0.02] ${cls}`}>
            <div className="text-3xl font-bold">{count}</div>
            <div className="text-xs text-[#737373] uppercase tracking-widest mt-1">{label}</div>
          </div>
        ))}
      </div>

      <div className="glass-panel rounded-xl border border-white/5 bg-white/[0.02] flex flex-col flex-1 min-h-0 overflow-hidden">
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
          <div className="flex items-center gap-3">
            <span className="flex items-center gap-2 text-xs text-emerald-400 uppercase tracking-wider">
              <span className="relative flex h-2 w-2">
                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
                <span className="relative inline-flex rounded-full h-2 w-2 bg-emerald-500"></span>
              </span>
              Live
            </span>
            <button onClick={recheckAll} disabled={checkingAll} className="flex items-center gap-2 text-gray-400 hover:text-white transition-colors text-sm px-3 py-1.5 rounded-lg hover:bg-white/5 border border-white/10 disabled:opacity-40">
              <RefreshCw size={16} className={checkingAll ? 'animate-spin' : ''} />
              <span>Forçar Handshake SSL</span>
            </button>
          </div>
        </div>

        <div className="flex-1 overflow-auto custom-scrollbar">
          <table className="w-full text-left text-sm whitespace-nowrap">
            <thead className="bg-[#0c0c0e]/80 sticky top-0 z-10">
              <tr>
                <th className="py-3 px-4 font-medium text-gray-500">Domínio</th>
                <th className="py-3 px-4 font-medium text-gray-500">Emissor</th>
                <th className="py-3 px-4 font-medium text-gray-500">Validade</th>
                <th className="py-3 px-4 font-medium text-gray-500">Status</th>
                <th className="py-3 px-4 font-medium text-gray-500">Última checagem</th>
                <th className="py-3 px-4 font-medium text-gray-500 text-right">Ações</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-white/5">
              {loading && <tr><td colSpan={6} className="py-8 text-center text-gray-500">Carregando domínios...</td></tr>}
              {!loading && filteredDomains.length === 0 && (
                <tr><td colSpan={6} className="py-8 text-center text-gray-500">Nenhum domínio monitorado. Clique em "Monitorar Domínio".</td></tr>
              )}
              {filteredDomains.map(domain => {
                const st = statusOf(domain);
                const pct = Math.max(0, Math.min(100, (domain.days_left / 90) * 100));
                const barColor = domain.days_left > 30 ? 'bg-emerald-400' : domain.days_left > 0 ? 'bg-amber-400' : 'bg-rose-400';
                return (
                  <tr key={domain.id} className="hover:bg-white/[0.02] transition-colors group">
                    <td className="py-3 px-4">
                      <div className="flex items-center gap-3">
                        <div className="w-8 h-8 rounded-full bg-white/5 flex items-center justify-center text-gray-400 border border-white/5">
                          <Globe size={14} />
                        </div>
                        <span className="text-gray-200 font-medium">{domain.domain}</span>
                      </div>
                    </td>
                    <td className="py-3 px-4 text-gray-400">{domain.issuer || '—'}</td>
                    <td className="py-3 px-4">
                      {st === 'pending' ? (
                        <span className="text-blue-400">Checando...</span>
                      ) : domain.valid ? (
                        <div className="w-32">
                          <span className={`font-mono font-medium ${domain.days_left > 30 ? 'text-emerald-400' : domain.days_left > 0 ? 'text-amber-400' : 'text-rose-400'}`}>
                            {domain.days_left} dias
                          </span>
                          <div className="w-full h-1 bg-white/10 rounded-full mt-1 overflow-hidden">
                            <div className={`h-full ${barColor}`} style={{ width: `${pct}%` }}></div>
                          </div>
                        </div>
                      ) : (
                        <span className="text-rose-400 text-xs" title={domain.error_msg}>{domain.error_msg || 'Falha'}</span>
                      )}
                    </td>
                    <td className="py-3 px-4">
                      <span className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-xs font-medium border ${statusStyle(st)}`}>
                        {statusIcon(st)}
                        {statusLabel(st)}
                      </span>
                    </td>
                    <td className="py-3 px-4 text-gray-500 text-xs">
                      <span className="flex items-center gap-1.5"><Clock size={12} /> {relativeTime(domain.last_check)}</span>
                    </td>
                    <td className="py-3 px-4 text-right">
                      <div className="flex items-center justify-end gap-2">
                        <button onClick={() => recheckOne(domain.id)} disabled={busy[domain.id]} className="p-1.5 text-gray-400 hover:text-emerald-400 hover:bg-emerald-400/10 rounded transition-colors disabled:opacity-40" title="Rechecar agora">
                          <RefreshCw size={16} className={busy[domain.id] ? 'animate-spin' : ''} />
                        </button>
                        <button onClick={() => deleteDomain(domain.id)} className="p-1.5 text-gray-400 hover:text-rose-400 hover:bg-rose-400/10 rounded transition-colors" title="Remover">
                          <Trash2 size={16} />
                        </button>
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
};

export default SslView;
