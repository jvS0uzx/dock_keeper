import { useCallback, useEffect, useState, type FormEvent } from 'react';
import {
  ChevronLeft, ChevronRight, CircleCheck, CircleSlash, Loader2, Search,
  TriangleAlert,
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
  ok: { label: 'Aceito', className: 'badge-ok', Icon: CircleCheck },
  denied: { label: 'Recusado', className: 'badge-crit', Icon: CircleSlash },
  error: { label: 'Erro', className: 'badge-warn', Icon: TriangleAlert },
  pending: { label: 'Em execução', className: 'badge-info', Icon: Loader2 },
};

const ResultBadge = ({ result }: { result: string }) => {
  const style = RESULT_STYLES[result] ?? {
    label: result || 'desconhecido',
    className: 'badge-muted',
    Icon: TriangleAlert,
  };
  const { Icon } = style;
  return (
    <span className={`badge ${style.className}`}>
      <Icon size={12} strokeWidth={1.75} />
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
    return <span className="text-text-mut break-all">{detail}</span>;
  }

  const entries = Object.entries(parsed);
  if (entries.length === 0) return <span className="text-text-faint">—</span>;

  return (
    <div className="flex flex-wrap gap-x-3 gap-y-1">
      {entries.map(([key, value]) => (
        <span key={key} className="whitespace-nowrap">
          <span className="text-text-faint">{key}</span>
          <span className="text-text ml-1">{String(value)}</span>
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
    <div className="p-8 anim-rise">
      <div className="page-header">
        <div>
          <h1 className="page-title">Log de auditoria</h1>
          <p className="page-desc">
            Quem fez o quê, em qual alvo e com qual resultado. Registra toda escrita do painel e toda
            leitura de log remoto feita por SSH.
          </p>
        </div>
      </div>

      <form onSubmit={aplicar} className="panel p-5 mb-6">
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <div>
            <label htmlFor="audit-actor" className="eyebrow block mb-1.5">Usuário</label>
            <input
              id="audit-actor"
              type="text"
              value={actor}
              onChange={(e) => setActor(e.target.value)}
              className="input-base w-full"
              placeholder="Nome exato de quem agiu"
            />
          </div>
          <div>
            <label htmlFor="audit-action" className="eyebrow block mb-1.5">Ação</label>
            <Select id="audit-action" value={action} onChange={setAction} options={ACTIONS} />
          </div>
          <div>
            <label htmlFor="audit-result" className="eyebrow block mb-1.5">Resultado</label>
            <Select id="audit-result" value={result} onChange={setResult} options={RESULTS} />
          </div>
          <div>
            <label htmlFor="audit-site" className="eyebrow block mb-1.5">Unidade</label>
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
            <label htmlFor="audit-from" className="eyebrow block mb-1.5">De</label>
            <input
              id="audit-from"
              type="datetime-local"
              value={from}
              onChange={(e) => setFrom(e.target.value)}
              className="input-base w-full"
            />
          </div>
          <div>
            <label htmlFor="audit-to" className="eyebrow block mb-1.5">Até</label>
            <input
              id="audit-to"
              type="datetime-local"
              value={to}
              onChange={(e) => setTo(e.target.value)}
              className="input-base w-full"
            />
          </div>
        </div>
        <button type="submit" disabled={loading} className="btn btn-primary mt-4 disabled:opacity-50">
          {loading ? <Loader2 size={16} strokeWidth={1.75} className="animate-spin" /> : <Search size={16} strokeWidth={1.75} />}
          Filtrar
        </button>
      </form>

      <div className="panel p-5">
        <div className="flex items-center justify-between mb-4 gap-4 flex-wrap">
          <h2 className="eyebrow">
            Registros{total > 0 && <span className="mono-data text-text-mut normal-case tracking-normal"> · {total}</span>}
          </h2>
          {total > 0 && (
            <div className="flex items-center gap-3">
              <span className="text-xs text-text-mut tabular">
                {primeiro}–{ultimo} de {total}
              </span>
              <button
                type="button"
                onClick={() => setOffset((o) => Math.max(0, o - PAGE_SIZE))}
                disabled={!temAnterior || loading}
                className="btn btn-ghost min-h-0 h-8 px-2 disabled:opacity-30"
              >
                <ChevronLeft size={16} strokeWidth={1.75} />
                <span className="sr-only">Página anterior</span>
              </button>
              <button
                type="button"
                onClick={() => setOffset((o) => o + PAGE_SIZE)}
                disabled={!temProxima || loading}
                className="btn btn-ghost min-h-0 h-8 px-2 disabled:opacity-30"
              >
                <ChevronRight size={16} strokeWidth={1.75} />
                <span className="sr-only">Próxima página</span>
              </button>
            </div>
          )}
        </div>

        {erro ? (
          <p className="text-sm text-crit">{erro}</p>
        ) : loading ? (
          <p className="text-sm text-text-mut">Carregando...</p>
        ) : items.length === 0 ? (
          <p className="text-sm text-text-mut">Nenhum registro para os filtros informados.</p>
        ) : (
          <div className="overflow-x-auto custom-scrollbar">
            <table className="table-base min-w-[880px] text-xs">
              <thead>
                <tr>
                  <th className="whitespace-nowrap">Quando</th>
                  <th className="whitespace-nowrap">Quem</th>
                  <th className="whitespace-nowrap">Ação</th>
                  <th className="whitespace-nowrap">Alvo</th>
                  <th className="whitespace-nowrap">Unidade</th>
                  <th className="whitespace-nowrap">Resultado</th>
                  <th>Detalhe</th>
                </tr>
              </thead>
              <tbody>
                {items.map((e) => (
                  <tr key={e.id} className="align-top">
                    <td className="mono-data text-text-faint whitespace-nowrap">
                      {formatDateTime(e.at)}
                    </td>
                    <td className="whitespace-nowrap">
                      <span className="text-text-hi font-medium">{e.actor_username || 'sem sessão'}</span>
                      {e.source_ip && (
                        <span className="block mono-data text-text-faint text-[11px]">{e.source_ip}</span>
                      )}
                    </td>
                    <td className="mono-data text-text whitespace-nowrap">{e.action}</td>
                    <td className="text-text-mut">
                      <span className="block">{e.target_label || e.target_id || '—'}</span>
                      {e.target_label && e.target_id && (
                        <span className="block mono-data text-text-faint text-[11px] break-all">
                          {e.target_id}
                        </span>
                      )}
                    </td>
                    <td className="text-text-mut whitespace-nowrap">
                      {e.site_id === null ? '—' : siteName(e.site_id)}
                    </td>
                    <td className="whitespace-nowrap">
                      <ResultBadge result={e.result} />
                    </td>
                    <td className="mono-data">
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
