import { useCallback, useEffect, useState, type FormEvent } from 'react';
import {
  ChevronLeft, ChevronRight, CircleCheck, CircleSlash, Loader2, Search,
  ShieldAlert, TriangleAlert,
} from 'lucide-react';
import { api, type AuditEntry, type AuditQuery } from '../lib/api';
import { formatDateTime } from '../lib/format';
import { useSiteScope } from './ui/site-scope-context';
import Select, { type SelectOption } from './ui/Select';

const PAGE_SIZE = 50;

const RESULTS: SelectOption[] = [
  { value: '', label: 'Todos' },
  { value: 'denied', label: 'Recusado' },
  { value: 'error', label: 'Erro' },
  { value: 'ok', label: 'Aceito' },
  { value: 'pending', label: 'Em execução' },
];

// Famílias de ação, pelo prefixo que o backend usa. O filtro casa a família
// inteira, então "container" traz start, stop e restart sem o operador precisar
// decorar o verbo.
const ACTIONS: SelectOption[] = [
  { value: '', label: 'Todas' },
  { value: 'auth', label: 'Autenticação' },
  { value: 'user', label: 'Usuários' },
  { value: 'server', label: 'Servidores' },
  { value: 'site', label: 'Unidades' },
  { value: 'container', label: 'Containers' },
  { value: 'container-logs', label: 'Logs de container' },
  { value: 'auth-log', label: 'Log de autenticação' },
  { value: 'alert-rule', label: 'Regras de alerta' },
  { value: 'ssl', label: 'Certificados' },
  { value: 'ssl-domain', label: 'Domínios' },
  { value: 'network-host', label: 'Inventário' },
  { value: 'floorplan', label: 'Plantas baixas' },
  { value: 'ingest', label: 'Ingestão' },
];

/**
 * O resultado é a coluna que o administrador varre primeiro: "recusado" é o que
 * ele procura quando desconfia de alguma coisa, e precisa saltar da tabela sem
 * ele ter que ler texto.
 */
const RESULT_STYLES: Record<string, { label: string; className: string; Icon: typeof CircleCheck }> = {
  ok: { label: 'Aceito', className: 'text-[#10b981] border-[#10b981]/30 bg-[#10b981]/10', Icon: CircleCheck },
  denied: { label: 'Recusado', className: 'text-rose-400 border-rose-400/40 bg-rose-400/10', Icon: CircleSlash },
  error: { label: 'Erro', className: 'text-amber-400 border-amber-400/30 bg-amber-400/10', Icon: TriangleAlert },
  pending: { label: 'Em execução', className: 'text-sky-400 border-sky-400/30 bg-sky-400/10', Icon: Loader2 },
};

const ResultBadge = ({ result }: { result: string }) => {
  const style = RESULT_STYLES[result] ?? {
    label: result || 'desconhecido',
    className: 'text-[#737373] border-white/10 bg-white/5',
    Icon: TriangleAlert,
  };
  const { Icon } = style;
  return (
    <span
      className={`inline-flex items-center gap-1.5 px-2 py-1 rounded-md border text-[10px] font-bold uppercase tracking-wider ${style.className}`}
    >
      <Icon className="w-3 h-3" />
      {style.label}
    </span>
  );
};

/**
 * O detalhe chega como JSON serializado. Imprimi-lo cru numa célula produz uma
 * linha ilegível que ninguém lê — e o detalhe é justamente o que explica a ação.
 */
const DetailCells = ({ detail }: { detail: string }) => {
  let parsed: Record<string, unknown>;
  try {
    parsed = JSON.parse(detail || '{}') as Record<string, unknown>;
  } catch {
    return <span className="text-[#737373] break-all">{detail}</span>;
  }

  const entries = Object.entries(parsed);
  if (entries.length === 0) return <span className="text-[#737373]">—</span>;

  return (
    <div className="flex flex-wrap gap-x-3 gap-y-1">
      {entries.map(([key, value]) => (
        <span key={key} className="whitespace-nowrap">
          <span className="text-[#737373]">{key}</span>
          <span className="text-white/80 ml-1">{String(value)}</span>
        </span>
      ))}
    </div>
  );
};

const AuditView = () => {
  const { sites, siteName } = useSiteScope();

  const [actor, setActor] = useState('');
  const [action, setAction] = useState('');
  const [result, setResult] = useState('');
  const [siteId, setSiteId] = useState('');
  const [from, setFrom] = useState('');
  const [to, setTo] = useState('');

  // Os filtros aplicados, separados dos campos do formulário: sem isso, mudar
  // um campo e paginar traria a página seguinte de uma busca que não é a que
  // está na tela.
  const [applied, setApplied] = useState<AuditQuery>({});
  const [offset, setOffset] = useState(0);

  const [items, setItems] = useState<AuditEntry[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [erro, setErro] = useState('');

  const carregar = useCallback((query: AuditQuery, nextOffset: number, signal: AbortSignal) => {
    setLoading(true);
    setErro('');
    api.audit({ ...query, limit: PAGE_SIZE, offset: nextOffset }, signal)
      .then((page) => {
        setItems(page.items);
        setTotal(page.total);
      })
      .catch((err: unknown) => {
        if (signal.aborted) return;
        setItems([]);
        setTotal(0);
        setErro(err instanceof Error ? err.message : 'falha ao consultar a auditoria');
      })
      .finally(() => {
        if (!signal.aborted) setLoading(false);
      });
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    carregar(applied, offset, controller.signal);
    return () => controller.abort();
  }, [applied, offset, carregar]);

  // O <input type="datetime-local"> devolve "2026-08-22T14:30", sem fuso; o
  // backend exige RFC3339 e recusa o resto com 400.
  const paraRFC3339 = (local: string): string | undefined => {
    if (!local) return undefined;
    const date = new Date(local);
    return isNaN(date.getTime()) ? undefined : date.toISOString();
  };

  const aplicar = (e: FormEvent) => {
    e.preventDefault();
    setOffset(0);
    setApplied({
      actor: actor.trim() || undefined,
      action: action || undefined,
      result: result || undefined,
      site_id: siteId || undefined,
      from: paraRFC3339(from),
      to: paraRFC3339(to),
    });
  };

  const primeiro = total === 0 ? 0 : offset + 1;
  const ultimo = Math.min(offset + items.length, total);
  const temAnterior = offset > 0;
  const temProxima = offset + items.length < total;

  return (
    <div className="p-8">
      <h1 className="text-2xl font-light text-white mb-2 flex items-center gap-3">
        <ShieldAlert className="w-6 h-6 text-[#10b981]" />
        Log de <span className="font-bold">Auditoria</span>
      </h1>
      <p className="text-[#737373] text-sm mb-8">
        Quem fez o quê, em qual alvo e com qual resultado. Registra toda escrita do painel e toda
        leitura de log remoto feita por SSH.
      </p>

      <form onSubmit={aplicar} className="glass-panel p-6 rounded-xl border border-white/5 bg-white/[0.02] mb-6">
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <div>
            <label htmlFor="audit-actor" className="text-xs text-[#737373] block mb-1">Usuário</label>
            <input
              id="audit-actor"
              type="text"
              value={actor}
              onChange={(e) => setActor(e.target.value)}
              className="w-full bg-black/40 border border-white/10 rounded-lg px-4 py-2 text-sm text-white focus:outline-none focus:border-[#10b981] transition-colors"
              placeholder="Nome exato de quem agiu"
            />
          </div>
          <div>
            <label htmlFor="audit-action" className="text-xs text-[#737373] block mb-1">Ação</label>
            <Select id="audit-action" value={action} onChange={setAction} options={ACTIONS} />
          </div>
          <div>
            <label htmlFor="audit-result" className="text-xs text-[#737373] block mb-1">Resultado</label>
            <Select id="audit-result" value={result} onChange={setResult} options={RESULTS} />
          </div>
          <div>
            <label htmlFor="audit-site" className="text-xs text-[#737373] block mb-1">Unidade</label>
            <Select
              id="audit-site"
              value={siteId}
              onChange={setSiteId}
              options={[
                { value: '', label: 'Todas' },
                ...sites.map((s) => ({ value: String(s.id), label: s.name })),
              ]}
            />
          </div>
          <div>
            <label htmlFor="audit-from" className="text-xs text-[#737373] block mb-1">De</label>
            <input
              id="audit-from"
              type="datetime-local"
              value={from}
              onChange={(e) => setFrom(e.target.value)}
              className="w-full bg-black/40 border border-white/10 rounded-lg px-4 py-2 text-sm text-white focus:outline-none focus:border-[#10b981] transition-colors"
            />
          </div>
          <div>
            <label htmlFor="audit-to" className="text-xs text-[#737373] block mb-1">Até</label>
            <input
              id="audit-to"
              type="datetime-local"
              value={to}
              onChange={(e) => setTo(e.target.value)}
              className="w-full bg-black/40 border border-white/10 rounded-lg px-4 py-2 text-sm text-white focus:outline-none focus:border-[#10b981] transition-colors"
            />
          </div>
        </div>
        <button
          type="submit"
          disabled={loading}
          className="mt-4 flex items-center gap-2 bg-[#10b981]/20 hover:bg-[#10b981]/30 border border-[#10b981]/50 text-[#10b981] font-bold text-xs uppercase tracking-widest py-3 px-6 rounded-lg transition-all disabled:opacity-50"
        >
          {loading ? <Loader2 className="w-4 h-4 animate-spin" /> : <Search className="w-4 h-4" />}
          Filtrar
        </button>
      </form>

      <div className="glass-panel p-6 rounded-xl border border-white/5 bg-white/[0.02]">
        <div className="flex items-center justify-between mb-6 gap-4 flex-wrap">
          <h2 className="text-sm font-bold tracking-widest text-[#737373] uppercase">
            Registros {total > 0 && <span className="text-[#10b981]">({total})</span>}
          </h2>
          {total > 0 && (
            <div className="flex items-center gap-3">
              <span className="text-xs text-[#737373] tabular-nums">
                {primeiro}–{ultimo} de {total}
              </span>
              <button
                type="button"
                onClick={() => setOffset((o) => Math.max(0, o - PAGE_SIZE))}
                disabled={!temAnterior || loading}
                className="p-2 rounded-lg border border-white/10 text-white/70 hover:text-white hover:bg-white/5 disabled:opacity-30 disabled:hover:bg-transparent transition-colors"
              >
                <ChevronLeft className="w-4 h-4" />
                <span className="sr-only">Página anterior</span>
              </button>
              <button
                type="button"
                onClick={() => setOffset((o) => o + PAGE_SIZE)}
                disabled={!temProxima || loading}
                className="p-2 rounded-lg border border-white/10 text-white/70 hover:text-white hover:bg-white/5 disabled:opacity-30 disabled:hover:bg-transparent transition-colors"
              >
                <ChevronRight className="w-4 h-4" />
                <span className="sr-only">Próxima página</span>
              </button>
            </div>
          )}
        </div>

        {erro ? (
          <p className="text-sm text-rose-400">{erro}</p>
        ) : loading ? (
          <p className="text-sm text-[#737373]">Carregando...</p>
        ) : items.length === 0 ? (
          <p className="text-sm text-[#737373]">Nenhum registro para os filtros informados.</p>
        ) : (
          <div className="overflow-x-auto custom-scrollbar">
            <table className="w-full text-xs text-left border-collapse">
              <thead className="text-[10px] text-[#737373] uppercase tracking-widest bg-[#0c0c0e]">
                <tr>
                  <th className="py-3 px-4 rounded-l whitespace-nowrap">Quando</th>
                  <th className="py-3 px-4 whitespace-nowrap">Quem</th>
                  <th className="py-3 px-4 whitespace-nowrap">Ação</th>
                  <th className="py-3 px-4 whitespace-nowrap">Alvo</th>
                  <th className="py-3 px-4 whitespace-nowrap">Unidade</th>
                  <th className="py-3 px-4 whitespace-nowrap">Resultado</th>
                  <th className="py-3 px-4 rounded-r">Detalhe</th>
                </tr>
              </thead>
              <tbody>
                {items.map((e) => (
                  <tr key={e.id} className="border-b border-white/[0.03] hover:bg-white/[0.05] transition-all align-top">
                    <td className="py-2 px-4 text-[#737373] whitespace-nowrap font-mono">
                      {formatDateTime(e.at)}
                    </td>
                    <td className="py-2 px-4 whitespace-nowrap">
                      <span className="text-white/90">{e.actor_username || 'sem sessão'}</span>
                      {e.source_ip && (
                        <span className="block text-[10px] text-[#737373] font-mono">{e.source_ip}</span>
                      )}
                    </td>
                    <td className="py-2 px-4 text-[#10b981] whitespace-nowrap font-mono">{e.action}</td>
                    <td className="py-2 px-4 text-white/80">
                      <span className="block">{e.target_label || e.target_id || '—'}</span>
                      {e.target_label && e.target_id && (
                        <span className="block text-[10px] text-[#737373] font-mono break-all">
                          {e.target_id}
                        </span>
                      )}
                    </td>
                    <td className="py-2 px-4 text-white/70 whitespace-nowrap">
                      {e.site_id === null ? '—' : siteName(e.site_id)}
                    </td>
                    <td className="py-2 px-4 whitespace-nowrap">
                      <ResultBadge result={e.result} />
                    </td>
                    <td className="py-2 px-4 font-mono">
                      <DetailCells detail={e.detail} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
};

export default AuditView;
