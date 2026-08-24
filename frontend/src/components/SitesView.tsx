import { useState, useEffect, useCallback, type FormEvent } from 'react';
import { Trash2, Plus, MonitorSmartphone, Network } from 'lucide-react';
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

  return (
    <div className="p-8 anim-rise">
      <div className="page-header">
        <div>
          <h1 className="page-title">Unidades monitoradas</h1>
          <p className="page-desc">
            Cada unidade agrupa as máquinas de um local. O agente informa a unidade pelo código,
            em <code className="mono-data text-accent">AGENT_SITE</code>, e a estação se classifica sozinha.
          </p>
        </div>
      </div>

      <div className={`grid grid-cols-1 gap-6 ${canOperate ? 'lg:grid-cols-3' : ''}`}>
        {/* Cadastro só para Suporte TI; Visualizador vê as unidades. */}
        {canOperate && (
        <div className="panel p-5 col-span-1 h-fit">
          <h2 className="eyebrow mb-5">Nova unidade</h2>
          <form onSubmit={handleCreate} className="flex flex-col gap-4">
            <div>
              <label htmlFor="site-name" className="eyebrow block mb-1.5">Nome</label>
              <input
                id="site-name"
                type="text"
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
                className="input-base w-full"
                placeholder="Ex: Filial Norte"
                required
              />
            </div>
            <div>
              <label htmlFor="site-code" className="eyebrow block mb-1.5">Código</label>
              <input
                id="site-code"
                type="text"
                value={form.code}
                onChange={(e) => setForm({ ...form, code: e.target.value })}
                className="input-base w-full"
                placeholder="Ex: norte"
                required
              />
              <p className="text-[11px] text-text-faint mt-1.5">
                Valor de AGENT_SITE nas máquinas desta unidade.
              </p>
            </div>
            <div>
              <label htmlFor="site-address" className="eyebrow block mb-1.5">Endereço</label>
              <input
                id="site-address"
                type="text"
                value={form.address}
                onChange={(e) => setForm({ ...form, address: e.target.value })}
                className="input-base w-full"
                placeholder="Opcional"
              />
            </div>
            <button type="submit" className="btn btn-primary mt-1">
              <Plus size={16} strokeWidth={1.75} />
              Criar unidade
            </button>
          </form>
        </div>
        )}

        <div className={canOperate ? 'col-span-2' : ''}>
          <h2 className="eyebrow mb-4">
            Unidades cadastradas{sites.length > 0 && <span className="mono-data text-text-mut normal-case tracking-normal"> · {sites.length}</span>}
          </h2>

          {loading ? (
            <p className="text-sm text-text-mut">Carregando...</p>
          ) : sites.length === 0 ? (
            <p className="text-sm text-text-mut">
              Nenhuma unidade cadastrada. Crie a primeira para agrupar as máquinas por local.
            </p>
          ) : (
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 stagger">
              {sites.map(site => {
                const counts = countFor(site.id);
                return (
                  <div
                    key={site.id}
                    role="button"
                    tabIndex={0}
                    onClick={() => openSite(site.id)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' || e.key === ' ') {
                        e.preventDefault();
                        openSite(site.id);
                      }
                    }}
                    className="panel panel-hover p-5 cursor-pointer"
                  >
                    <div className="flex items-start justify-between gap-3">
                      <div className="min-w-0">
                        <div className="font-semibold text-text-hi truncate">{site.name}</div>
                        <div className="mt-1 flex items-center gap-2">
                          <code className="mono-data text-accent bg-ink-850 border border-line rounded px-1.5 py-0.5 text-[11px]">
                            {site.code}
                          </code>
                          {site.address && <span className="text-xs text-text-faint truncate">{site.address}</span>}
                        </div>
                      </div>
                      {canOperate && (
                        <button
                          onClick={(e) => {
                            e.stopPropagation();
                            handleDelete(site);
                          }}
                          className="btn btn-ghost btn-sm hover:text-crit shrink-0"
                          title={`Remover ${site.name}`}
                        >
                          <Trash2 size={14} strokeWidth={1.75} />
                        </button>
                      )}
                    </div>

                    <div className="mt-4 pt-4 border-t border-line grid grid-cols-2 gap-3">
                      <div>
                        <div className="eyebrow flex items-center gap-1.5">
                          <MonitorSmartphone size={12} strokeWidth={1.75} />
                          Estações
                        </div>
                        <div className="stat-value text-lg mt-1">{counts.stations}</div>
                      </div>
                      <div>
                        <div className="eyebrow flex items-center gap-1.5">
                          <Network size={12} strokeWidth={1.75} />
                          Hosts na rede
                        </div>
                        <div className="stat-value text-lg mt-1">{counts.hosts}</div>
                      </div>
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </div>
      </div>
    </div>
  );
};

export default SitesView;
