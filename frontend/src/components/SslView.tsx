import { useState, useEffect, useMemo, useRef } from 'react';
import { Lock, Plus, Search, ShieldCheck, ShieldAlert, AlertTriangle, RefreshCw, Trash2, Globe, Clock, Download } from 'lucide-react';
import { api, type DiscoveredDomain, type DomainRecord as DomainItem } from '../lib/api';
import { relativeTime } from '../lib/format';
import { useDialog } from './ui/dialog-context';
import { useRole } from './ui/session-context';

type Status = 'valid' | 'warning' | 'expired' | 'pending';

const statusOf = (d: DomainItem): Status => {
  if (!d.last_check) return 'pending';
  if (!d.valid) return 'expired';
  if (d.days_left <= 14) return 'warning';
  return 'valid';
};

// Rótulos dos motivos que o backend classifica em internal/network/ssl.go. O
// backend manda o código estável; o texto vive aqui, para ser reescrito sem
// mexer na comparação que outro código faz.
const INVALID_REASON_LABELS: Record<string, string> = {
  expirado: 'Expirado',
  ainda_nao_valido: 'Ainda não válido',
  hostname_divergente: 'Hostname divergente',
  autoassinado: 'Autoassinado',
  cadeia_nao_confiavel: 'Cadeia não confiável',
  sem_certificado: 'Sem certificado',
  handshake: 'Falha no handshake',
};

// Domínio verificado antes de a coluna invalid_reason existir não tem motivo
// classificado, e continuar mostrando só "Falha" é melhor que inventar um.
const invalidReasonLabel = (reason: string): string =>
  INVALID_REASON_LABELS[reason] ?? 'Inválido';

const invalidReasonOf = (d: DomainItem): string => d.invalid_reason ?? '';

// Janela que o backend leva para refazer os handshakes antes de persistir.
const RECHECK_SETTLE_MS = 2500;

const SslView = () => {
  const dialog = useDialog();
  const { canOperate } = useRole();
  const timeouts = useRef<number[]>([]);
  const [searchTerm, setSearchTerm] = useState('');
  const [domains, setDomains] = useState<DomainItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState<Record<number, boolean>>({});
  const [checkingAll, setCheckingAll] = useState(false);
  // Domínios que o Nginx atendeu e ainda não estão sob monitoramento.
  const [discovered, setDiscovered] = useState<DiscoveredDomain[]>([]);
  const [importing, setImporting] = useState(false);

  const fetchDomains = async () => {
    try {
      setDomains(await api.domains());
    } catch (e) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  };

  const loadDiscovered = async () => {
    try {
      setDiscovered((await api.discoverDomains()).filter(d => !d.monitored));
    } catch (e) {
      console.error(e);
    }
  };

  // Polling ao vivo: reflete o que o worker vai persistindo em background.
  useEffect(() => {
    const pending = timeouts.current;
    fetchDomains();
    loadDiscovered();
    const interval = setInterval(fetchDomains, 10000);
    return () => {
      clearInterval(interval);
      pending.forEach(clearTimeout);
      pending.length = 0;
    };
  }, []);

  // Agenda um refetch e mantém o handle para cancelar se a tela desmontar.
  const scheduleRefetch = (delay: number, task: () => void = fetchDomains) => {
    timeouts.current.push(window.setTimeout(task, delay));
  };

  const recheckOne = async (id: number) => {
    setBusy(prev => ({ ...prev, [id]: true }));
    try {
      const updated = await api.recheckDomain(id);
      setDomains(prev => prev.map(d => (d.id === id ? updated : d)));
    } catch (e) {
      console.error(e);
    } finally {
      setBusy(prev => ({ ...prev, [id]: false }));
    }
  };

  const recheckAll = async () => {
    setCheckingAll(true);
    try {
      await api.recheckAllDomains();
    } catch (e) {
      console.error(e);
      dialog.notify('Falha ao disparar a revalidação em lote.', 'error');
    } finally {
      scheduleRefetch(RECHECK_SETTLE_MS);
      scheduleRefetch(RECHECK_SETTLE_MS, () => setCheckingAll(false));
    }
  };

  const addDomain = async () => {
    const domainName = await dialog.prompt({
      title: 'Monitorar novo domínio',
      message: 'O certificado TLS passa a ser revalidado automaticamente.',
      placeholder: 'app.empresa.com',
      confirmLabel: 'Monitorar',
    });
    if (!domainName) return;
    try {
      await api.createDomain(domainName);
      scheduleRefetch(1500); // backend checa na hora
      dialog.notify(`${domainName} entrou no monitoramento.`, 'success');
    } catch (e) {
      console.error(e);
      dialog.notify('Erro ao adicionar domínio.', 'error');
    }
  };

  const deleteDomain = async (id: number) => {
    const confirmed = await dialog.confirm({
      title: 'Remover este domínio do monitoramento?',
      message: 'O histórico de checagens deixa de ser atualizado.',
      confirmLabel: 'Remover',
      danger: true,
    });
    if (!confirmed) return;
    try {
      await api.deleteDomain(id);
      setDomains(prev => prev.filter(d => d.id !== id));
    } catch (e) {
      console.error(e);
      dialog.notify('Erro ao remover o domínio.', 'error');
    }
  };

  const importAll = async () => {
    const nomes = discovered.map(d => d.domain);
    const confirmed = await dialog.confirm({
      title: `Monitorar ${nomes.length} domínio(s) do Nginx?`,
      message: nomes.slice(0, 8).join(', ') + (nomes.length > 8 ? '…' : ''),
      confirmLabel: 'Monitorar',
    });
    if (!confirmed) return;

    setImporting(true);
    try {
      const { imported } = await api.importDomains(nomes);
      dialog.notify(`${imported} domínio(s) entraram no monitoramento.`, 'success');
      // O handshake roda em background no backend; dá tempo de aparecer.
      scheduleRefetch(RECHECK_SETTLE_MS);
      await loadDiscovered();
    } catch (e) {
      dialog.notify((e as Error).message || 'Falha ao importar os domínios.', 'error');
    } finally {
      setImporting(false);
    }
  };

  const summary = useMemo(() => {
    const acc = { valid: 0, warning: 0, expired: 0, pending: 0 };
    domains.forEach(d => { acc[statusOf(d)]++; });
    return acc;
  }, [domains]);

  const filteredDomains = useMemo(
    () => domains.filter(d => d.domain.toLowerCase().includes(searchTerm.toLowerCase())),
    [domains, searchTerm],
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
        {canOperate && (
          <div className="flex items-center gap-2">
            {discovered.length > 0 && (
              <button
                onClick={importAll}
                disabled={importing}
                className="flex items-center gap-2 border border-[#10b981]/50 bg-[#10b981]/10 hover:bg-[#10b981]/20 text-[#10b981] px-4 py-2 rounded-lg font-medium transition-colors disabled:opacity-40"
              >
                <Download size={18} />
                <span>Importar {discovered.length} do Nginx</span>
              </button>
            )}
            <button onClick={addDomain} className="flex items-center gap-2 bg-[#10b981] hover:bg-[#059669] text-white px-4 py-2 rounded-lg font-medium transition-colors shadow-[0_0_15px_rgba(16,185,129,0.2)]">
              <Plus size={18} />
              <span>Monitorar Domínio</span>
            </button>
          </div>
        )}
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
            {canOperate && (
              <button onClick={recheckAll} disabled={checkingAll} className="flex items-center gap-2 text-gray-400 hover:text-white transition-colors text-sm px-3 py-1.5 rounded-lg hover:bg-white/5 border border-white/10 disabled:opacity-40">
                <RefreshCw size={16} className={checkingAll ? 'animate-spin' : ''} />
                <span>Forçar Handshake SSL</span>
              </button>
            )}
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
                {canOperate && <th className="py-3 px-4 font-medium text-gray-500 text-right">Ações</th>}
              </tr>
            </thead>
            <tbody className="divide-y divide-white/5">
              {loading && <tr><td colSpan={canOperate ? 6 : 5} className="py-8 text-center text-gray-500">Carregando domínios...</td></tr>}
              {!loading && filteredDomains.length === 0 && (
                <tr>
                  <td colSpan={canOperate ? 6 : 5} className="py-8 text-center text-gray-500">
                    Nenhum domínio monitorado.
                    {discovered.length > 0
                      ? ` O Nginx atendeu ${discovered.length} domínio(s) — use "Importar do Nginx" acima.`
                      : ' Cadastre um domínio ou aguarde o balanceador registrar tráfego.'}
                  </td>
                </tr>
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
                        <div className="w-40 space-y-1">
                          <span className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-md text-[11px] font-medium border border-rose-400/20 bg-rose-400/10 text-rose-300">
                            <ShieldAlert size={12} />
                            {invalidReasonLabel(invalidReasonOf(domain))}
                          </span>
                          {/* O badge mostra só o motivo mais grave; error_msg lista
                              todos quando o certificado tem mais de um problema. */}
                          {domain.error_msg && (
                            <p className="text-rose-400/70 text-[11px] leading-snug" title={domain.error_msg}>
                              {domain.error_msg}
                            </p>
                          )}
                        </div>
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
                    {canOperate && (
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
                    )}
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
