import { useState, useEffect, useCallback, type FormEvent } from 'react';
import { Building2, Trash2, Plus, MonitorSmartphone, Network } from 'lucide-react';
import { api, type Site, type ServerLiveStat, type NetworkHostView } from '../lib/api';
import { useDialog } from './ui/dialog-context';
import { useRole } from './ui/session-context';
import { useSiteScope } from './ui/site-scope-context';
import { useNavigation } from './ui/navigation-context';

const emptyForm = { name: '', code: '', address: '' };

const SitesView = () => {
  const dialog = useDialog();
  const { canOperate } = useRole();
  // O seletor da barra lateral lê a mesma lista: cadastrar ou remover aqui
  // precisa refletir lá na hora.
  const { reloadSites } = useSiteScope();
  const { openSite } = useNavigation();
  const [sites, setSites] = useState<Site[]>([]);
  const [stations, setStations] = useState<ServerLiveStat[]>([]);
  const [hosts, setHosts] = useState<NetworkHostView[]>([]);
  const [form, setForm] = useState({ ...emptyForm });
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    try {
      const [siteList, live, inventory] = await Promise.all([
        api.sites(),
        api.liveMetrics(),
        api.networkHosts(),
      ]);
      setSites(siteList);
      setStations(live.servers);
      setHosts(inventory.hosts);
    } catch (err) {
      console.error(err);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const handleCreate = async (e: FormEvent) => {
    e.preventDefault();
    if (!form.name.trim() || !form.code.trim()) return;
    try {
      await api.createSite(form);
      setForm({ ...emptyForm });
      load();
      reloadSites();
      dialog.notify(`Unidade "${form.name}" criada.`, 'success');
    } catch (err) {
      dialog.notify((err as Error).message || 'Falha ao criar a unidade.', 'error');
    }
  };

  const handleDelete = async (site: Site) => {
    const confirmed = await dialog.confirm({
      title: `Remover a unidade "${site.name}"?`,
      message: 'As máquinas continuam no inventário, apenas ficam sem unidade.',
      confirmLabel: 'Remover',
      danger: true,
    });
    if (!confirmed) return;
    try {
      await api.deleteSite(site.id);
      load();
      reloadSites();
    } catch (err) {
      dialog.notify((err as Error).message || 'Falha ao remover a unidade.', 'error');
    }
  };

  const countFor = (siteId: number) => ({
    stations: stations.filter(s => s.site_id === siteId).length,
    hosts: hosts.filter(h => h.site_id === siteId).length,
  });

  const inputClass =
    'w-full bg-black/40 border border-white/10 rounded-lg px-4 py-2 text-sm text-white focus:outline-none focus:border-[#10b981] transition-colors';

  return (
    <div className="p-8">
      <div className="flex items-center gap-3 mb-2">
        <Building2 size={22} className="text-[#10b981]" />
        <h1 className="text-2xl font-light text-white">
          Unidades <span className="font-bold">Monitoradas</span>
        </h1>
      </div>
      <p className="text-[#737373] text-sm mb-8">
        Cada unidade agrupa as máquinas de um local. O agente informa a unidade pelo código,
        em <code className="text-amber-300">AGENT_SITE</code>, e a estação se classifica sozinha.
      </p>

      <div className={`grid grid-cols-1 gap-6 ${canOperate ? 'lg:grid-cols-3' : ''}`}>
        {/* Cadastro só para Suporte TI; Visualizador vê as unidades. */}
        {canOperate && (
        <div className="glass-panel p-6 rounded-xl border border-white/5 bg-white/[0.02] col-span-1 h-fit">
          <h2 className="text-sm font-bold tracking-widest text-[#737373] uppercase mb-6">Nova Unidade</h2>
          <form onSubmit={handleCreate} className="flex flex-col gap-4">
            <div>
              <label htmlFor="site-name" className="text-xs text-[#737373] block mb-1">Nome</label>
              <input
                id="site-name"
                type="text"
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
                className={inputClass}
                placeholder="Ex: Filial Norte"
                required
              />
            </div>
            <div>
              <label htmlFor="site-code" className="text-xs text-[#737373] block mb-1">Código</label>
              <input
                id="site-code"
                type="text"
                value={form.code}
                onChange={(e) => setForm({ ...form, code: e.target.value })}
                className={inputClass}
                placeholder="Ex: norte"
                required
              />
              <p className="text-[10px] text-[#737373] mt-1">
                Valor de AGENT_SITE nas máquinas desta unidade.
              </p>
            </div>
            <div>
              <label htmlFor="site-address" className="text-xs text-[#737373] block mb-1">Endereço</label>
              <input
                id="site-address"
                type="text"
                value={form.address}
                onChange={(e) => setForm({ ...form, address: e.target.value })}
                className={inputClass}
                placeholder="Opcional"
              />
            </div>
            <button
              type="submit"
              className="mt-2 flex items-center justify-center gap-2 bg-[#10b981]/20 hover:bg-[#10b981]/30 border border-[#10b981]/50 text-[#10b981] font-bold text-xs uppercase tracking-widest py-3 rounded-lg transition-all"
            >
              <Plus size={14} />
              Criar Unidade
            </button>
          </form>
        </div>
        )}

        <div className={`glass-panel p-6 rounded-xl border border-white/5 bg-white/[0.02] ${canOperate ? 'col-span-2' : ''}`}>
          <h2 className="text-sm font-bold tracking-widest text-[#737373] uppercase mb-6">
            Unidades Cadastradas {sites.length > 0 && <span className="text-[#10b981]">({sites.length})</span>}
          </h2>

          {loading ? (
            <p className="text-sm text-[#737373]">Carregando...</p>
          ) : sites.length === 0 ? (
            <p className="text-sm text-[#737373]">
              Nenhuma unidade cadastrada. Crie a primeira para agrupar as máquinas por local.
            </p>
          ) : (
            <div className="overflow-x-auto custom-scrollbar">
              <table className="w-full text-sm text-left border-collapse">
                <thead className="text-[10px] text-[#737373] uppercase tracking-widest bg-[#0c0c0e]">
                  <tr>
                    <th className="py-3 px-4 rounded-l">Nome</th>
                    <th className="py-3 px-4">Código</th>
                    <th className="py-3 px-4">Endereço</th>
                    <th className="py-3 px-4 text-right">Estações</th>
                    <th className="py-3 px-4 text-right">Hosts na rede</th>
                    {canOperate && <th className="py-3 px-4 text-right rounded-r">Ação</th>}
                  </tr>
                </thead>
                <tbody>
                  {sites.map(site => {
                    const counts = countFor(site.id);
                    return (
                      <tr
                        key={site.id}
                        onClick={() => openSite(site.id)}
                        className="border-b border-white/[0.03] hover:bg-white/[0.05] transition-all cursor-pointer"
                      >
                        <td className="py-4 px-4 font-medium text-white/90">{site.name}</td>
                        <td className="py-4 px-4">
                          <code className="text-xs text-amber-300 bg-black/30 px-2 py-0.5 rounded border border-white/5">
                            {site.code}
                          </code>
                        </td>
                        <td className="py-4 px-4 text-[#737373] text-xs">{site.address || '—'}</td>
                        <td className="py-4 px-4 text-right">
                          <span className="inline-flex items-center gap-1 text-white/80">
                            <MonitorSmartphone size={12} className="text-[#737373]" />
                            {counts.stations}
                          </span>
                        </td>
                        <td className="py-4 px-4 text-right">
                          <span className="inline-flex items-center gap-1 text-white/80">
                            <Network size={12} className="text-[#737373]" />
                            {counts.hosts}
                          </span>
                        </td>
                        {canOperate && (
                          <td className="py-4 px-4 text-right">
                            <button
                              onClick={(e) => {
                              e.stopPropagation();
                              handleDelete(site);
                            }}
                              className="inline-flex items-center gap-1 text-xs text-red-400/80 hover:text-red-400 tracking-wider"
                            >
                              <Trash2 size={14} />
                              Remover
                            </button>
                          </td>
                        )}
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};

export default SitesView;
