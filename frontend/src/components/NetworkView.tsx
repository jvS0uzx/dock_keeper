import LoadNotice from './ui/LoadNotice';
import { useLoadStatus } from './ui/load-status';
import { useState, useEffect, useCallback, useMemo, type FormEvent } from 'react';
import {
  RefreshCw, Search, ShieldQuestion, Router, Printer, Monitor,
  HardDrive, Pencil, X, Globe, Lock,
} from 'lucide-react';
import { api, type HostInventoryPatch, type NetworkHostView, type NetworkInventory, type Site } from '../lib/api';
import { formatDateTime, relativeTime } from '../lib/format';
import { useDialog } from './ui/dialog-context';
import { useRole } from './ui/session-context';
import { useSiteScope } from './ui/site-scope-context';
import Select from './ui/Select';
import { POLL } from '../lib/polling';
import { AbaDeInterfaces } from './InterfacesDeRede';
import { temSNMP } from '../lib/snmp';


const SCAN_SETTLE_MS = 8000;

const FINGERPRINTS: { port: string; label: string; Icon: typeof Monitor }[] = [
  { port: '9100', label: 'Impressora', Icon: Printer },
  { port: '3389', label: 'Estação Windows', Icon: Monitor },
  { port: '445', label: 'Windows / SMB', Icon: Monitor },
  { port: '135', label: 'Windows / RPC', Icon: Monitor },
  { port: '5000', label: 'NAS', Icon: HardDrive },
  { port: '22', label: 'Linux / SSH', Icon: Router },
];

const DEVICE_TYPES: { value: string; label: string }[] = [
  { value: 'printer', label: 'Impressora' },
  { value: 'windows', label: 'Estação Windows' },
  { value: 'nas', label: 'NAS' },
  { value: 'linux', label: 'Linux' },
  { value: 'web-device', label: 'Dispositivo Web' },
  { value: 'unknown', label: 'Desconhecido' },
];

const AUTO_TYPE_OPTION = { value: '', label: 'Automático (detectado pelas portas)' };
const AUTO_SITE_OPTION = { value: '', label: 'Automático (definido pelo coletor)' };

const typeLabel = (value: string) =>
  DEVICE_TYPES.find((t) => t.value === value)?.label ?? 'Desconhecido';

const identify = (host: NetworkHostView) => {
  const match = FINGERPRINTS.find(f => host.open_ports.includes(f.port));
  return match ?? { label: 'Desconhecido', Icon: ShieldQuestion };
};

const detectedTypeLabel = (host: NetworkHostView) =>
  host.device_type_locked ? identify(host).label : typeLabel(host.device_type);

type Filter = 'all' | 'unmonitored' | 'online';

type AbaDoHost = 'resumo' | 'interfaces';

const PainelDoHost = ({ host, local, onClose }: { host: NetworkHostView; local: string; onClose: () => void }) => {
  const comSNMP = temSNMP(host);
  const [aba, setAba] = useState<AbaDoHost>('resumo');

  useEffect(() => {
    const aoTeclar = (evento: KeyboardEvent) => {
      if (evento.key === 'Escape') onClose();
    };
    document.addEventListener('keydown', aoTeclar);
    return () => document.removeEventListener('keydown', aoTeclar);
  }, [onClose]);

  const pares: [string, string][] = [
    ['Endereço', host.ip],
    ['Nome', host.hostname || '—'],
    ['MAC', host.mac || '—'],
    ['Tipo provável', detectedTypeLabel(host)],
    ['Local', local || '—'],
    ['Portas abertas', host.open_ports.length > 0 ? host.open_ports.join(', ') : '—'],
    ['Primeira vez', formatDateTime(host.first_seen)],
    ['Última vez', formatDateTime(host.last_seen)],
  ];

  const abas: { chave: AbaDoHost; rotulo: string }[] = [
    { chave: 'resumo', rotulo: 'Resumo' },
    ...(comSNMP ? [{ chave: 'interfaces' as const, rotulo: 'Interfaces' }] : []),
  ];

  return (
    <div className="fixed inset-0 z-50 flex justify-end bg-black/60 backdrop-blur-sm" onClick={onClose}>
      <aside
        role="dialog"
        aria-modal="true"
        aria-label={`Detalhe do host ${host.ip}`}
        data-testid="painel-do-host"
        onClick={(evento) => evento.stopPropagation()}
        className="h-full w-full max-w-3xl overflow-y-auto custom-scrollbar border-l border-line bg-ink-900 shadow-pop"
      >
        <div className="sticky top-0 z-10 border-b border-line bg-ink-850">
          <div className="flex items-start justify-between gap-4 p-5 pb-3">
            <div className="min-w-0">
              <h2 className="mono-data truncate text-base font-semibold text-text-hi">{host.ip}</h2>
              <p className="mt-1 truncate text-xs text-text-faint">{host.hostname || '—'}</p>
            </div>
            <button
              type="button"
              onClick={onClose}
              aria-label="Fechar detalhe"
              className="shrink-0 text-text-faint transition-colors hover:text-text-hi"
            >
              <X size={18} strokeWidth={1.75} />
            </button>
          </div>
          <div role="tablist" aria-label="Seções do host" className="flex gap-1 px-5 pb-3">
            {abas.map(({ chave, rotulo }) => (
              <button
                key={chave}
                type="button"
                role="tab"
                aria-selected={aba === chave}
                onClick={() => setAba(chave)}
                className={`btn text-xs ${
                  aba === chave ? 'border border-accent/50 bg-accent/10 text-accent' : 'btn-ghost text-text-mut'
                }`}
              >
                {rotulo}
              </button>
            ))}
          </div>
        </div>

        {aba === 'resumo' && (
          <section className="p-5">
            <dl className="grid grid-cols-2 gap-x-6 gap-y-3">
              {pares.map(([rotulo, valor]) => (
                <div key={rotulo} className="flex min-w-0 flex-col gap-0.5">
                  <dt className="eyebrow">{rotulo}</dt>
                  <dd className="mono-data truncate text-sm text-text-hi" title={valor}>{valor}</dd>
                </div>
              ))}
            </dl>
          </section>
        )}
        {aba === 'interfaces' && comSNMP && <AbaDeInterfaces hostId={host.id} />}
      </aside>
    </div>
  );
};

const StatCard = ({ label, value, accent }: { label: string; value: number | string; accent: string }) => (
  <div className="stat-card">
    <div className={`stat-value ${accent}`}>{value}</div>
    <div className="eyebrow mt-1.5">{label}</div>
  </div>
);

interface HostEditModalProps {
  host: NetworkHostView;
  sites: Site[];
  onClose: () => void;
  onSaved: (updated: NetworkHostView) => void;
}

const TEXT_FIELDS = ['floor', 'sector', 'room', 'rack', 'asset_tag', 'owner', 'notes'] as const;

const buildForm = (host: NetworkHostView) => ({
  site_id: host.site_locked ? host.site_id : null,
  device_type: host.device_type_locked ? host.device_type : '',
  floor: host.floor,
  sector: host.sector,
  room: host.room,
  rack: host.rack,
  asset_tag: host.asset_tag,
  owner: host.owner,
  notes: host.notes,
});

const HostEditModal = ({ host, sites, onClose, onSaved }: HostEditModalProps) => {
  const dialog = useDialog();
  const [saving, setSaving] = useState(false);
  const initial = useMemo(() => buildForm(host), [host]);
  const [form, setForm] = useState(initial);

  const currentSiteName = sites.find((s) => s.id === host.site_id)?.name ?? '';

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    if (saving) return;

    const patch: HostInventoryPatch = {};
    for (const field of TEXT_FIELDS) {
      if (form[field] !== initial[field]) patch[field] = form[field];
    }
    if (form.device_type !== initial.device_type) patch.device_type = form.device_type;
    if (form.site_id !== initial.site_id) patch.site_id = form.site_id;

    if (Object.keys(patch).length === 0) {
      onClose();
      return;
    }

    setSaving(true);
    try {
      const updated = await api.updateHost(host.ip, patch);
      dialog.notify(`Cadastro de ${host.ip} gravado.`, 'success');
      onSaved(updated);
    } catch (err) {
      dialog.notify((err as Error).message || 'Falha ao gravar o cadastro.', 'error');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm" onClick={onClose}>
      <div
        role="dialog"
        aria-modal="true"
        aria-label={`Cadastro de ${host.ip}`}
        className="w-full max-w-lg bg-ink-900 border border-line rounded-card shadow-pop overflow-hidden"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between p-4 border-b border-line bg-ink-850">
          <div className="flex items-center gap-3">
            <Pencil className="text-accent" size={15} strokeWidth={1.75} />
            <h3 className="text-text-hi font-semibold mono-data selectable">{host.ip}</h3>
            {host.hostname && <span className="text-xs text-text-faint">{host.hostname}</span>}
          </div>
          <button onClick={onClose} aria-label="Fechar" className="text-text-faint hover:text-text-hi transition-colors">
            <X size={18} strokeWidth={1.75} />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="p-4 grid grid-cols-2 gap-3 max-h-[70vh] overflow-y-auto custom-scrollbar">
          <div className="col-span-2">
            <label htmlFor="host-site" className="eyebrow block mb-1.5">Unidade</label>
            <Select
              id="host-site"
              value={form.site_id === null ? '' : String(form.site_id)}
              onChange={(v) => setForm({ ...form, site_id: v === '' ? null : Number(v) })}
              options={[AUTO_SITE_OPTION, ...sites.map((s) => ({ value: String(s.id), label: s.name }))]}
            />
            {form.site_id === null && (
              <p className="text-[10px] text-text-faint mt-1">
                {currentSiteName
                  ? `Definida agora: ${currentSiteName}. Escolher uma unidade fixa o valor e o coletor deixa de alterá-lo.`
                  : 'Nenhuma unidade definida no momento. Escolher uma fixa o valor.'}
              </p>
            )}
          </div>

          <div className="col-span-2">
            <label htmlFor="host-type" className="eyebrow block mb-1.5">Tipo de equipamento</label>
            <Select
              id="host-type"
              value={form.device_type}
              onChange={(v) => setForm({ ...form, device_type: v })}
              options={[AUTO_TYPE_OPTION, ...DEVICE_TYPES]}
            />
            {form.device_type === '' && (
              <p className="text-[10px] text-text-faint mt-1">
                Detectado agora: {detectedTypeLabel(host)}. Escolher um tipo fixa o valor e a varredura deixa de alterá-lo.
              </p>
            )}
          </div>

          {([
            ['host-floor', 'Andar', 'floor'],
            ['host-sector', 'Setor', 'sector'],
            ['host-room', 'Sala', 'room'],
            ['host-rack', 'Rack', 'rack'],
            ['host-asset', 'Patrimônio', 'asset_tag'],
            ['host-owner', 'Responsável', 'owner'],
          ] as const).map(([id, label, field]) => (
            <div key={id}>
              <label htmlFor={id} className="eyebrow block mb-1.5">{label}</label>
              <input
                id={id}
                type="text"
                value={form[field]}
                onChange={(e) => setForm({ ...form, [field]: e.target.value })}
                className="input-base w-full"
              />
            </div>
          ))}

          <div className="col-span-2">
            <label htmlFor="host-notes" className="eyebrow block mb-1.5">Observações</label>
            <textarea
              id="host-notes"
              value={form.notes}
              onChange={(e) => setForm({ ...form, notes: e.target.value })}
              rows={2}
              className="input-base w-full py-2"
            />
          </div>

          <div className="col-span-2 flex justify-end gap-2 pt-2">
            <button type="button" onClick={onClose} className="btn btn-ghost">
              Cancelar
            </button>
            <button type="submit" disabled={saving} className="btn btn-primary disabled:opacity-40">
              {saving ? 'Gravando...' : 'Gravar'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};

const NetworkView = () => {
  const dialog = useDialog();
  const { canOperate } = useRole();
  const { numericSiteId } = useSiteScope();
  const [inventory, setInventory] = useState<NetworkInventory | null>(null);
  const [sites, setSites] = useState<Site[]>([]);
  const [editing, setEditing] = useState<NetworkHostView | null>(null);
  const [detalheId, setDetalheId] = useState<number | null>(null);
  const [loading, setLoading] = useState(true);
  const carga = useLoadStatus();
  const { ok: cargaOk, fail: cargaFail } = carga;
  const unidades = useLoadStatus();
  const { ok: unidadesOk, fail: unidadesFail } = unidades;
  const [scanning, setScanning] = useState(false);
  const [search, setSearch] = useState('');
  const [filter, setFilter] = useState<Filter>('all');

  const fetchInventory = useCallback(
    (signal?: AbortSignal) =>
      api.networkHosts(signal)
        .then((dados) => {
          setInventory(dados);
          cargaOk();
        })
        .catch((err: unknown) => {
          if (!signal?.aborted) cargaFail(err, 'Falha ao ler o inventário de rede.');
        })
        .finally(() => setLoading(false)),
    [cargaOk, cargaFail],
  );

  useEffect(() => {
    const controller = new AbortController();
    fetchInventory(controller.signal);
    api.sites()
      .then((list) => {
        setSites(list);
        unidadesOk();
      })
      .catch((err) => {
        if (!controller.signal.aborted) unidadesFail(err, 'Falha ao listar as unidades.');
      });
    const interval = setInterval(() => fetchInventory(controller.signal), POLL.inventarioDeRede);
    return () => {
      clearInterval(interval);
      controller.abort();
    };
  }, [fetchInventory, unidadesOk, unidadesFail]);

  const runScan = async () => {
    setScanning(true);
    try {
      await api.scanNetwork();
      dialog.notify('Varredura disparada. O inventário atualiza em alguns segundos.', 'info');
      setTimeout(() => {
        fetchInventory();
        setScanning(false);
      }, SCAN_SETTLE_MS);
    } catch (err) {
      setScanning(false);
      dialog.notify((err as Error).message || 'Falha ao disparar a varredura.', 'error');
    }
  };

  const handleSaved = (updated: NetworkHostView) => {
    setInventory((prev) =>
      prev
        ? { ...prev, hosts: prev.hosts.map((h) => (h.ip === updated.ip ? { ...h, ...updated } : h)) }
        : prev,
    );
    setEditing(null);
  };

  const scoped = useMemo(
    () => (inventory?.hosts ?? []).filter(h => numericSiteId === null || h.site_id === numericSiteId),
    [inventory, numericSiteId],
  );

  const hosts = useMemo(() => {
    const term = search.toLowerCase();
    return scoped.filter(h => {
      if (filter === 'unmonitored' && h.monitored) return false;
      if (filter === 'online' && !h.online) return false;
      if (!term) return true;
      return h.ip.includes(term) || h.hostname.toLowerCase().includes(term) || h.mac.includes(term);
    });
  }, [scoped, search, filter]);

  const monitoredCount = scoped.filter(h => h.monitored).length;
  const onlineCount = scoped.filter(h => h.online).length;
  const unmonitored = scoped.length - monitoredCount;
  const siteName = (id: number | null) => sites.find((s) => s.id === id)?.name ?? '';
  const localDe = (host: NetworkHostView) =>
    [siteName(host.site_id), host.sector, host.room].filter(Boolean).join(' · ');
  const detalhe = detalheId === null ? null : (inventory?.hosts.find((h) => h.id === detalheId) ?? null);
  const fecharDetalhe = useCallback(() => setDetalheId(null), []);
  const columns = canOperate ? 9 : 8;

  return (
    <div className="p-4 md:p-8 h-full flex flex-col overflow-hidden anim-rise">
      <LoadNotice error={carga.error} lastOk={carga.lastOk} className="mb-4" />
      <LoadNotice error={unidades.error} lastOk={unidades.lastOk} className="mb-4" />
      <div className="page-header flex-col md:flex-row md:items-end items-start">
        <div>
          <h1 className="page-title">Inventário de Rede</h1>
          <p className="page-desc">
            Máquinas e ativos vistos na rede da seção, cruzados com o que já é monitorado.
          </p>
        </div>
        {canOperate && (
          <button onClick={runScan} disabled={scanning} className="btn btn-primary disabled:opacity-40">
            <RefreshCw size={15} strokeWidth={1.75} className={scanning ? 'animate-spin' : ''} />
            {scanning ? 'Varrendo...' : 'Varrer agora'}
          </button>
        )}
      </div>

      <div className="grid grid-cols-2 md:grid-cols-4 gap-3 mb-6 stagger">
        <StatCard label="Hosts no inventário" value={scoped.length} accent="" />
        <StatCard label="Online agora" value={onlineCount} accent="text-ok" />
        <StatCard label="Monitorados" value={monitoredCount} accent="text-info" />
        <StatCard label="Sem agente" value={unmonitored} accent={unmonitored > 0 ? 'text-warn' : 'text-text-faint'} />
      </div>

      <div className="panel flex flex-col flex-1 min-h-0 overflow-hidden">
        <div className="p-4 border-b border-line flex flex-col md:flex-row gap-4 justify-between items-center">
          <div className="relative w-full md:w-96">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 text-text-faint" size={16} strokeWidth={1.75} />
            <input
              type="text"
              placeholder="Buscar IP, nome ou MAC..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="input-base w-full pl-9"
            />
          </div>
          <div className="flex items-center gap-2">
            {([
              ['all', 'Todos'],
              ['online', 'Online'],
              ['unmonitored', 'Sem agente'],
            ] as const).map(([key, label]) => (
              <button
                key={key}
                onClick={() => setFilter(key)}
                className={`btn text-xs ${
                  filter === key
                    ? 'border border-accent/50 bg-accent/10 text-accent'
                    : 'btn-ghost text-text-mut'
                }`}
              >
                {label}
              </button>
            ))}
          </div>
        </div>

        <div className="flex-1 overflow-auto custom-scrollbar">
          <table className="table-base whitespace-nowrap">
            <thead className="bg-ink-900 sticky top-0 z-10">
              <tr>
                <th>Estado</th>
                <th>Endereço</th>
                <th>Nome</th>
                <th>Tipo provável</th>
                <th>Local</th>
                <th>MAC</th>
                <th>Portas abertas</th>
                <th>Monitorado</th>
                {canOperate && <th className="text-right">Ações</th>}
              </tr>
            </thead>
            <tbody>
              {loading && (
                <tr><td colSpan={columns} className="py-8 text-center text-text-faint">Carregando inventário...</td></tr>
              )}
              {!loading && inventory?.scan_active === false && (
                <tr>
                  <td colSpan={columns} className="py-8 text-center text-text-faint">
                    Inventário desligado. Defina <code className="text-warn">DISCOVERY_CIDRS</code> no .env do backend.
                  </td>
                </tr>
              )}
              {!loading && inventory?.scan_active && hosts.length === 0 && (
                <tr><td colSpan={columns} className="py-8 text-center text-text-faint">Nenhum host para os filtros informados.</td></tr>
              )}
              {hosts.map(host => {
                const { label, Icon } = identify(host);
                const local = localDe(host);
                return (
                  <tr key={host.id}>
                    <td>
                      <span className={`badge ${host.online ? 'badge-ok' : 'badge-muted'}`}>
                        {host.online ? 'Online' : 'Offline'}
                      </span>
                    </td>
                    <td>
                      <button
                        type="button"
                        onClick={() => setDetalheId(host.id)}
                        aria-label={`Ver detalhe de ${host.ip}`}
                        className="mono-data text-text-hi hover:text-accent transition-colors"
                      >
                        {host.ip}
                      </button>
                    </td>
                    <td className="text-text">
                      <span className="inline-flex items-center gap-2">
                        {host.hostname || <span className="text-text-faint">—</span>}
                        {temSNMP(host) && (
                          <span
                            className={`badge ${host.snmp_erro ? 'badge-warn' : 'badge-info'}`}
                            title={host.snmp_erro ? `SNMP com falha: ${host.snmp_erro}` : 'Telemetria SNMP ativa'}
                            data-testid="marca-snmp"
                          >
                            SNMP
                          </span>
                        )}
                      </span>
                    </td>
                    <td>
                      <span className="inline-flex items-center gap-2 text-xs text-text-mut">
                        <Icon size={14} strokeWidth={1.75} className="text-text-faint" />
                        {DEVICE_TYPES.find((t) => t.value === host.device_type)?.label ?? label}
                        {host.device_type_locked && (
                          <span
                            title="Tipo fixado manualmente; a varredura não altera"
                            className="inline-flex text-accent/80"
                          >
                            <Lock size={11} aria-label="Tipo fixado manualmente" />
                          </span>
                        )}
                      </span>
                    </td>
                    <td className="text-xs text-text-mut">
                      {local || <span className="text-text-faint">—</span>}
                    </td>
                    <td className="mono-data text-xs text-text-faint selectable">{host.mac || '—'}</td>
                    <td>
                      <div className="flex gap-1 flex-wrap">
                        {host.open_ports.map(port => (
                          <span key={port} className="mono-data bg-ink-800 text-text-mut px-1.5 py-0.5 rounded text-[10px] border border-line">
                            {port}
                          </span>
                        ))}
                      </div>
                    </td>
                    <td>
                      {host.monitored ? (
                        <span className="badge badge-ok">
                          {host.kind === 'agent' ? 'Agente' : 'SSH'}
                        </span>
                      ) : (
                        <span className="badge badge-warn">
                          Sem agente
                        </span>
                      )}
                    </td>
                    {canOperate && (
                      <td className="text-right">
                        <button
                          onClick={() => setEditing(host)}
                          title="Editar cadastro"
                          className="p-1.5 text-text-mut hover:text-accent hover:bg-accent/10 rounded-ctrl transition-colors"
                        >
                          <Pencil size={14} strokeWidth={1.75} />
                        </button>
                      </td>
                    )}
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>

        <div className="px-4 py-2.5 border-t border-line flex items-center gap-4">
          {inventory?.last_scan && <span className="eyebrow">Última varredura {relativeTime(inventory.last_scan)}</span>}
          {sites.length > 0 && (
            <span className="eyebrow flex items-center gap-1.5">
              <Globe size={10} />
              {sites.length} unidade(s)
            </span>
          )}
        </div>
      </div>

      {detalhe !== null && (
        <PainelDoHost key={detalhe.id} host={detalhe} local={localDe(detalhe)} onClose={fecharDetalhe} />
      )}

      {editing && (
        <HostEditModal
          key={editing.ip}
          host={editing}
          sites={sites}
          onClose={() => setEditing(null)}
          onSaved={handleSaved}
        />
      )}
    </div>
  );
};

export default NetworkView;
