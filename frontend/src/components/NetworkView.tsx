import { useState, useEffect, useCallback, useMemo, type FormEvent } from 'react';
import {
  Network, RefreshCw, Search, ShieldQuestion, Router, Printer, Monitor,
  HardDrive, Pencil, X, Globe, Lock,
} from 'lucide-react';
import { api, type HostInventoryPatch, type NetworkHostView, type NetworkInventory, type Site } from '../lib/api';
import { relativeTime } from '../lib/format';
import { useDialog } from './ui/dialog-context';
import { useRole } from './ui/session-context';
import { useSiteScope } from './ui/site-scope-context';
import Select from './ui/Select';

const POLL_MS = 20000;

// Tempo que a varredura costuma levar antes do inventário refletir o disparo.
const SCAN_SETTLE_MS = 8000;

// Portas que identificam o tipo de equipamento. A primeira que casar vence,
// então a ordem importa: 9100 (fila de impressão) antes de 80.
const FINGERPRINTS: { port: string; label: string; Icon: typeof Monitor }[] = [
  { port: '9100', label: 'Impressora', Icon: Printer },
  { port: '3389', label: 'Estação Windows', Icon: Monitor },
  { port: '445', label: 'Windows / SMB', Icon: Monitor },
  { port: '135', label: 'Windows / RPC', Icon: Monitor },
  { port: '5000', label: 'NAS', Icon: HardDrive },
  { port: '22', label: 'Linux / SSH', Icon: Router },
];

// Tipos que o backend infere pelas portas; o operador pode corrigir à mão.
const DEVICE_TYPES: { value: string; label: string }[] = [
  { value: 'printer', label: 'Impressora' },
  { value: 'windows', label: 'Estação Windows' },
  { value: 'nas', label: 'NAS' },
  { value: 'linux', label: 'Linux' },
  { value: 'web-device', label: 'Dispositivo Web' },
  { value: 'unknown', label: 'Desconhecido' },
];

// Valor vazio significa "não fixado": o backend recalcula pelo que a varredura
// detectar. Fica no topo porque é o estado natural de um host recém-descoberto.
const AUTO_TYPE_OPTION = { value: '', label: 'Automático (detectado pelas portas)' };
const AUTO_SITE_OPTION = { value: '', label: 'Automático (definido pelo coletor)' };

const typeLabel = (value: string) =>
  DEVICE_TYPES.find((t) => t.value === value)?.label ?? 'Desconhecido';

const identify = (host: NetworkHostView) => {
  const match = FINGERPRINTS.find(f => host.open_ports.includes(f.port));
  return match ?? { label: 'Desconhecido', Icon: ShieldQuestion };
};

/**
 * Tipo que a varredura enxerga agora, para o operador saber o que está aceitando
 * ao deixar o campo em automático.
 *
 * Sem lock, o valor gravado é o próprio detectado. Com lock, o backend só devolve
 * o valor fixado, então a inferência local pelas portas é a melhor aproximação.
 */
const detectedTypeLabel = (host: NetworkHostView) =>
  host.device_type_locked ? identify(host).label : typeLabel(host.device_type);

type Filter = 'all' | 'unmonitored' | 'online';

const StatCard = ({ label, value, accent }: { label: string; value: number | string; accent: string }) => (
  <div className="glass-panel rounded-xl p-4 border border-white/5 bg-white/[0.02]">
    <div className={`text-3xl font-bold ${accent}`}>{value}</div>
    <div className="text-xs text-[#737373] uppercase tracking-widest mt-1">{label}</div>
  </div>
);

interface HostEditModalProps {
  host: NetworkHostView;
  sites: Site[];
  onClose: () => void;
  onSaved: (updated: NetworkHostView) => void;
}

/** Campos de texto livre do cadastro, os únicos comparáveis um a um. */
const TEXT_FIELDS = ['floor', 'sector', 'room', 'rack', 'asset_tag', 'owner', 'notes'] as const;

/**
 * Estado inicial do formulário.
 *
 * Campo sem lock começa em "automático" mesmo tendo valor gravado: o valor veio
 * da varredura, não de uma escolha de alguém, e mostrá-lo como se fosse escolha
 * do operador faria qualquer gravação fixar o campo sem querer.
 */
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

/** Cadastro físico do equipamento: onde ele está e de quem é. */
const HostEditModal = ({ host, sites, onClose, onSaved }: HostEditModalProps) => {
  const dialog = useDialog();
  const [saving, setSaving] = useState(false);
  const initial = useMemo(() => buildForm(host), [host]);
  const [form, setForm] = useState(initial);

  const currentSiteName = sites.find((s) => s.id === host.site_id)?.name ?? '';

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    if (saving) return;

    // Só o que o operador mexeu entra no patch: mandar o formulário inteiro
    // ligaria os locks de tipo e unidade em toda gravação, mesmo que ele tenha
    // vindo aqui apenas para preencher a sala.
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

  const inputClass =
    'w-full bg-black/40 border border-white/10 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-[#10b981] transition-colors';

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm" onClick={onClose}>
      <div
        role="dialog"
        aria-modal="true"
        aria-label={`Cadastro de ${host.ip}`}
        className="w-full max-w-lg bg-[#0c0c0e] border border-white/10 rounded-xl shadow-2xl overflow-hidden"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between p-4 border-b border-white/5 bg-[#1a1c23]">
          <div className="flex items-center gap-3">
            <Pencil className="text-[#10b981] w-4 h-4" />
            <h3 className="text-white font-bold font-mono tracking-wider selectable">{host.ip}</h3>
            {host.hostname && <span className="text-xs text-[#737373]">{host.hostname}</span>}
          </div>
          <button onClick={onClose} aria-label="Fechar" className="text-[#737373] hover:text-white transition-colors">
            <X size={18} />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="p-4 grid grid-cols-2 gap-3 max-h-[70vh] overflow-y-auto custom-scrollbar">
          <div className="col-span-2">
            <label htmlFor="host-site" className="text-xs text-[#737373] block mb-1">Unidade</label>
            <Select
              id="host-site"
              value={form.site_id === null ? '' : String(form.site_id)}
              onChange={(v) => setForm({ ...form, site_id: v === '' ? null : Number(v) })}
              options={[AUTO_SITE_OPTION, ...sites.map((s) => ({ value: String(s.id), label: s.name }))]}
            />
            {form.site_id === null && (
              <p className="text-[10px] text-[#737373] mt-1">
                {currentSiteName
                  ? `Definida agora: ${currentSiteName}. Escolher uma unidade fixa o valor e o coletor deixa de alterá-lo.`
                  : 'Nenhuma unidade definida no momento. Escolher uma fixa o valor.'}
              </p>
            )}
          </div>

          <div className="col-span-2">
            <label htmlFor="host-type" className="text-xs text-[#737373] block mb-1">Tipo de equipamento</label>
            <Select
              id="host-type"
              value={form.device_type}
              onChange={(v) => setForm({ ...form, device_type: v })}
              options={[AUTO_TYPE_OPTION, ...DEVICE_TYPES]}
            />
            {form.device_type === '' && (
              <p className="text-[10px] text-[#737373] mt-1">
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
              <label htmlFor={id} className="text-xs text-[#737373] block mb-1">{label}</label>
              <input
                id={id}
                type="text"
                value={form[field]}
                onChange={(e) => setForm({ ...form, [field]: e.target.value })}
                className={inputClass}
              />
            </div>
          ))}

          <div className="col-span-2">
            <label htmlFor="host-notes" className="text-xs text-[#737373] block mb-1">Observações</label>
            <textarea
              id="host-notes"
              value={form.notes}
              onChange={(e) => setForm({ ...form, notes: e.target.value })}
              rows={2}
              className={inputClass}
            />
          </div>

          <div className="col-span-2 flex justify-end gap-2 pt-2">
            <button
              type="button"
              onClick={onClose}
              className="px-4 py-2 rounded-lg border border-white/10 text-xs font-bold uppercase tracking-widest text-[#737373] hover:text-white transition-colors"
            >
              Cancelar
            </button>
            <button
              type="submit"
              disabled={saving}
              className="px-4 py-2 rounded-lg border border-[#10b981]/50 bg-[#10b981]/20 hover:bg-[#10b981]/30 text-xs font-bold uppercase tracking-widest text-[#10b981] transition-colors disabled:opacity-40"
            >
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
  // Unidade escolhida na barra lateral: o inventário segue o mesmo escopo das
  // demais telas do painel de campo.
  const { numericSiteId } = useSiteScope();
  const [inventory, setInventory] = useState<NetworkInventory | null>(null);
  const [sites, setSites] = useState<Site[]>([]);
  const [editing, setEditing] = useState<NetworkHostView | null>(null);
  const [loading, setLoading] = useState(true);
  const [scanning, setScanning] = useState(false);
  const [search, setSearch] = useState('');
  const [filter, setFilter] = useState<Filter>('all');

  const fetchInventory = useCallback(async (signal?: AbortSignal) => {
    try {
      setInventory(await api.networkHosts(signal));
    } catch (err) {
      if (!signal?.aborted) console.error(err);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    fetchInventory(controller.signal);
    api.sites().then(setSites).catch(() => {});
    const interval = setInterval(() => fetchInventory(controller.signal), POLL_MS);
    return () => {
      clearInterval(interval);
      controller.abort();
    };
  }, [fetchInventory]);

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

  // Reflete a edição na tabela sem esperar o próximo polling.
  const handleSaved = (updated: NetworkHostView) => {
    setInventory((prev) =>
      prev
        ? { ...prev, hosts: prev.hosts.map((h) => (h.ip === updated.ip ? { ...h, ...updated } : h)) }
        : prev,
    );
    setEditing(null);
  };

  // Recorte da unidade antes de qualquer outro filtro: os cartões de resumo
  // contam sobre ele, não sobre o parque inteiro.
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
  const columns = canOperate ? 9 : 8;

  return (
    <div className="p-4 md:p-8 h-full flex flex-col overflow-hidden">
      <div className="mb-6 flex flex-col md:flex-row md:items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-light text-white mb-2 flex items-center gap-3">
            <Network className="text-[#10b981]" /> Inventário de <span className="font-bold">Rede</span>
          </h1>
          <p className="text-[#737373] text-sm">
            Máquinas e ativos vistos na rede da seção, cruzados com o que já é monitorado.
          </p>
        </div>
        {canOperate && (
          <button
            onClick={runScan}
            disabled={scanning}
            className="flex items-center gap-2 bg-[#10b981]/20 hover:bg-[#10b981]/30 border border-[#10b981]/50 text-[#10b981] font-bold text-xs uppercase tracking-widest px-4 py-3 rounded-lg transition-all disabled:opacity-40"
          >
            <RefreshCw size={14} className={scanning ? 'animate-spin' : ''} />
            {scanning ? 'Varrendo...' : 'Varrer agora'}
          </button>
        )}
      </div>

      <div className="grid grid-cols-2 md:grid-cols-4 gap-3 mb-6">
        <StatCard label="Hosts no inventário" value={scoped.length} accent="text-white" />
        <StatCard label="Online agora" value={onlineCount} accent="text-emerald-400" />
        <StatCard label="Monitorados" value={monitoredCount} accent="text-[#10b981]" />
        <StatCard label="Sem agente" value={unmonitored} accent={unmonitored > 0 ? 'text-amber-400' : 'text-[#737373]'} />
      </div>

      <div className="glass-panel rounded-xl border border-white/5 bg-white/[0.02] flex flex-col flex-1 min-h-0 overflow-hidden">
        <div className="p-4 border-b border-white/5 flex flex-col md:flex-row gap-4 justify-between items-center bg-black/20">
          <div className="relative w-full md:w-96">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500" size={18} />
            <input
              type="text"
              placeholder="Buscar IP, nome ou MAC..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="w-full bg-[#0c0c0e] border border-white/10 rounded-lg pl-10 pr-4 py-2 text-sm text-gray-300 focus:outline-none focus:border-[#10b981]/50 transition-all placeholder:text-gray-600"
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
                className={`px-3 py-2 rounded-lg text-xs font-bold tracking-widest uppercase transition-all border ${
                  filter === key
                    ? 'bg-[#10b981]/20 border-[#10b981]/50 text-[#10b981]'
                    : 'bg-black/40 border-white/10 text-[#737373] hover:text-white'
                }`}
              >
                {label}
              </button>
            ))}
          </div>
        </div>

        <div className="flex-1 overflow-auto custom-scrollbar">
          <table className="w-full text-left text-sm whitespace-nowrap">
            <thead className="bg-[#0c0c0e]/80 sticky top-0 z-10">
              <tr>
                <th className="py-3 px-4 font-medium text-gray-500">Estado</th>
                <th className="py-3 px-4 font-medium text-gray-500">Endereço</th>
                <th className="py-3 px-4 font-medium text-gray-500">Nome</th>
                <th className="py-3 px-4 font-medium text-gray-500">Tipo provável</th>
                <th className="py-3 px-4 font-medium text-gray-500">Local</th>
                <th className="py-3 px-4 font-medium text-gray-500">MAC</th>
                <th className="py-3 px-4 font-medium text-gray-500">Portas abertas</th>
                <th className="py-3 px-4 font-medium text-gray-500">Monitorado</th>
                {canOperate && <th className="py-3 px-4 font-medium text-gray-500 text-right">Ações</th>}
              </tr>
            </thead>
            <tbody className="divide-y divide-white/5">
              {loading && (
                <tr><td colSpan={columns} className="py-8 text-center text-gray-500">Carregando inventário...</td></tr>
              )}
              {!loading && inventory?.scan_active === false && (
                <tr>
                  <td colSpan={columns} className="py-8 text-center text-gray-500">
                    Inventário desligado. Defina <code className="text-amber-300">DISCOVERY_CIDRS</code> no .env do backend.
                  </td>
                </tr>
              )}
              {!loading && inventory?.scan_active && hosts.length === 0 && (
                <tr><td colSpan={columns} className="py-8 text-center text-gray-500">Nenhum host para os filtros informados.</td></tr>
              )}
              {hosts.map(host => {
                const { label, Icon } = identify(host);
                const local = [siteName(host.site_id), host.sector, host.room]
                  .filter(Boolean)
                  .join(' · ');
                return (
                  <tr key={host.ip} className="hover:bg-white/[0.02] transition-colors">
                    <td className="py-3 px-4">
                      <span className="flex items-center gap-2">
                        <span className={`w-2 h-2 rounded-full ${host.online ? 'bg-[#10b981] animate-pulse' : 'bg-[#737373]'}`} />
                        <span className={`text-[10px] font-bold tracking-widest uppercase ${host.online ? 'text-[#10b981]' : 'text-[#737373]'}`}>
                          {host.online ? 'Online' : 'Offline'}
                        </span>
                      </span>
                    </td>
                    <td className="py-3 px-4 font-mono text-gray-200 selectable">{host.ip}</td>
                    <td className="py-3 px-4 text-gray-300">{host.hostname || <span className="text-gray-600">—</span>}</td>
                    <td className="py-3 px-4">
                      <span className="inline-flex items-center gap-2 text-xs text-gray-400">
                        <Icon size={14} className="text-[#737373]" />
                        {DEVICE_TYPES.find((t) => t.value === host.device_type)?.label ?? label}
                        {host.device_type_locked && (
                          <span
                            title="Tipo fixado manualmente; a varredura não altera"
                            className="inline-flex text-[#10b981]/70"
                          >
                            <Lock size={11} aria-label="Tipo fixado manualmente" />
                          </span>
                        )}
                      </span>
                    </td>
                    <td className="py-3 px-4 text-xs text-gray-400">
                      {local || <span className="text-gray-600">—</span>}
                    </td>
                    <td className="py-3 px-4 font-mono text-xs text-gray-500 selectable">{host.mac || '—'}</td>
                    <td className="py-3 px-4">
                      <div className="flex gap-1 flex-wrap">
                        {host.open_ports.map(port => (
                          <span key={port} className="bg-white/5 text-gray-400 px-1.5 py-0.5 rounded text-[10px] font-mono border border-white/5">
                            {port}
                          </span>
                        ))}
                      </div>
                    </td>
                    <td className="py-3 px-4">
                      {host.monitored ? (
                        <span className="text-emerald-400 text-xs bg-emerald-400/10 px-2 py-1 rounded border border-emerald-400/20">
                          {host.kind === 'agent' ? 'Agente' : 'SSH'}
                        </span>
                      ) : (
                        <span className="text-amber-400/80 text-xs bg-amber-400/10 px-2 py-1 rounded border border-amber-400/20">
                          Sem agente
                        </span>
                      )}
                    </td>
                    {canOperate && (
                      <td className="py-3 px-4 text-right">
                        <button
                          onClick={() => setEditing(host)}
                          title="Editar cadastro"
                          className="p-1.5 text-gray-400 hover:text-[#10b981] hover:bg-[#10b981]/10 rounded transition-colors"
                        >
                          <Pencil size={14} />
                        </button>
                      </td>
                    )}
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>

        <div className="px-4 py-2 border-t border-white/5 text-[10px] text-[#737373] uppercase tracking-widest flex items-center gap-4">
          {inventory?.last_scan && <span>Última varredura {relativeTime(inventory.last_scan)}</span>}
          {sites.length > 0 && (
            <span className="flex items-center gap-1">
              <Globe size={10} />
              {sites.length} unidade(s)
            </span>
          )}
        </div>
      </div>

      {editing && (
        // A chave por IP garante que o formulário remonte ao trocar de host, em
        // vez de reaproveitar o estado do anterior.
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
