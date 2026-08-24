import { useState, useEffect, useMemo, useRef } from 'react';
import { Plus, Search, ShieldCheck, ShieldAlert, AlertTriangle, RefreshCw, Trash2, Globe, Clock, Download } from 'lucide-react';
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
  alvo_privado_bloqueado: 'Alvo privado bloqueado',
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

  const statusBadge = (s: Status) => ({
    valid: 'badge badge-ok',
    warning: 'badge badge-warn',
    expired: 'badge badge-crit',
    pending: 'badge badge-info',
  }[s]);

  const statusIcon = (s: Status) => ({
    valid: <ShieldCheck size={13} strokeWidth={1.75} />,
    warning: <AlertTriangle size={13} strokeWidth={1.75} />,
    expired: <ShieldAlert size={13} strokeWidth={1.75} />,
    pending: <RefreshCw size={13} className="animate-spin" strokeWidth={1.75} />,
  }[s]);

  const statusLabel = (s: Status) => ({
    valid: 'Válido', warning: 'Expira breve', expired: 'Inválido', pending: 'Checando...',
  }[s]);

  const daysClass = (days: number) =>
    days > 30 ? 'text-ok' : days > 0 ? 'text-warn' : 'text-crit';

  return (
    <div className="p-4 md:p-8 h-full flex flex-col overflow-hidden anim-rise">
      <div className="page-header flex-col md:flex-row md:items-end items-start">
        <div>
          <h1 className="page-title">SSL &amp; Domínios</h1>
          <p className="page-desc">Certificados TLS revalidados automaticamente; cadeia e hostname conferidos, não só a data.</p>
        </div>
        {canOperate && (
          <div className="flex items-center gap-2">
            {discovered.length > 0 && (
              <button onClick={importAll} disabled={importing} className="btn btn-ghost disabled:opacity-40">
                <Download size={16} strokeWidth={1.75} />
                <span>Importar {discovered.length} do Nginx</span>
              </button>
            )}
            <button onClick={addDomain} className="btn btn-primary">
              <Plus size={16} strokeWidth={1.75} />
              <span>Monitorar domínio</span>
            </button>
          </div>
        )}
      </div>

      {/* Resumo */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-3 mb-6 stagger">
        {([
          ['valid', 'Válidos', summary.valid, 'text-ok'],
          ['warning', 'Expirando', summary.warning, 'text-warn'],
          ['expired', 'Inválidos', summary.expired, 'text-crit'],
          ['pending', 'Pendentes', summary.pending, 'text-info'],
        ] as const).map(([key, label, count, cls]) => (
          <div key={key} className="stat-card">
            <div className={`stat-value ${count > 0 ? cls : ''}`}>{count}</div>
            <div className="eyebrow mt-1.5">{label}</div>
          </div>
        ))}
      </div>

      <div className="panel flex flex-col flex-1 min-h-0 overflow-hidden">
        <div className="p-4 border-b border-line flex flex-col md:flex-row gap-4 justify-between items-center">
          <div className="relative w-full md:w-96">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 text-text-faint" size={16} strokeWidth={1.75} />
            <input
              type="text"
              placeholder="Buscar domínio..."
              value={searchTerm}
              onChange={(e) => setSearchTerm(e.target.value)}
              className="input-base w-full pl-9"
            />
          </div>
          <div className="flex items-center gap-3">
            <span className="flex items-center gap-2 eyebrow text-ok">
              <span className="relative flex h-2 w-2">
                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-ok opacity-60"></span>
                <span className="relative inline-flex rounded-full h-2 w-2 bg-ok"></span>
              </span>
              Live
            </span>
            {canOperate && (
              <button onClick={recheckAll} disabled={checkingAll} className="btn btn-ghost disabled:opacity-40">
                <RefreshCw size={15} strokeWidth={1.75} className={checkingAll ? 'animate-spin' : ''} />
                <span>Forçar Handshake SSL</span>
              </button>
            )}
          </div>
        </div>

        <div className="flex-1 overflow-auto custom-scrollbar">
          <table className="table-base whitespace-nowrap">
            <thead className="bg-ink-900 sticky top-0 z-10">
              <tr>
                <th>Domínio</th>
                <th>Emissor</th>
                <th>Validade</th>
                <th>Status</th>
                <th>Última checagem</th>
                {canOperate && <th className="text-right">Ações</th>}
              </tr>
            </thead>
            <tbody>
              {loading && <tr><td colSpan={canOperate ? 6 : 5} className="py-8 text-center text-text-faint">Carregando domínios...</td></tr>}
              {!loading && filteredDomains.length === 0 && (
                <tr>
                  <td colSpan={canOperate ? 6 : 5} className="py-8 text-center text-text-faint">
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
                const barColor = domain.days_left > 30 ? 'bg-ok' : domain.days_left > 0 ? 'bg-warn' : 'bg-crit';
                return (
                  <tr key={domain.id}>
                    <td>
                      <div className="flex items-center gap-3">
                        <div className="w-7 h-7 rounded-full bg-ink-800 flex items-center justify-center text-text-faint border border-line">
                          <Globe size={13} strokeWidth={1.75} />
                        </div>
                        <span className="mono-data text-text-hi">{domain.domain}</span>
                      </div>
                    </td>
                    <td className="text-text-mut">{domain.issuer || '—'}</td>
                    <td>
                      {st === 'pending' ? (
                        <span className="text-info">Checando...</span>
                      ) : domain.valid ? (
                        <div className="w-32">
                          <span className={`mono-data font-medium ${daysClass(domain.days_left)}`}>
                            {domain.days_left} dias
                          </span>
                          <div className="w-full h-1 bg-ink-750 rounded-full mt-1 overflow-hidden">
                            <div className={`h-full ${barColor}`} style={{ width: `${pct}%` }}></div>
                          </div>
                        </div>
                      ) : (
                        <div className="w-44 space-y-1">
                          <span className="badge badge-crit">
                            <ShieldAlert size={12} strokeWidth={1.75} />
                            {invalidReasonLabel(invalidReasonOf(domain))}
                          </span>
                          {/* O badge mostra só o motivo mais grave; error_msg lista
                              todos quando o certificado tem mais de um problema. */}
                          {domain.error_msg && (
                            <p className="text-crit/70 text-[11px] leading-snug whitespace-normal" title={domain.error_msg}>
                              {domain.error_msg}
                            </p>
                          )}
                        </div>
                      )}
                    </td>
                    <td>
                      <span className={statusBadge(st)}>
                        {statusIcon(st)}
                        {statusLabel(st)}
                      </span>
                    </td>
                    <td className="text-text-faint text-xs">
                      <span className="flex items-center gap-1.5"><Clock size={12} strokeWidth={1.75} /> {relativeTime(domain.last_check)}</span>
                    </td>
                    {canOperate && (
                      <td className="text-right">
                        <div className="flex items-center justify-end gap-1">
                          <button onClick={() => recheckOne(domain.id)} disabled={busy[domain.id]} className="p-1.5 text-text-mut hover:text-accent hover:bg-accent/10 rounded-ctrl transition-colors disabled:opacity-40" title="Rechecar agora">
                            <RefreshCw size={15} strokeWidth={1.75} className={busy[domain.id] ? 'animate-spin' : ''} />
                          </button>
                          <button onClick={() => deleteDomain(domain.id)} className="p-1.5 text-text-mut hover:text-crit hover:bg-crit/10 rounded-ctrl transition-colors" title="Remover">
                            <Trash2 size={15} strokeWidth={1.75} />
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
