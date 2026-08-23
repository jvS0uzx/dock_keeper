import { useState, useEffect, useCallback, useRef } from 'react';
import { Map, Upload, Trash2, Save, Pencil, Eye, X, Plus, Building2 } from 'lucide-react';
import {
  api,
  type FloorPlan,
  type FloorPlanPin,
  type FloorPlanPinInput,
  type NetworkHostView,
} from '../lib/api';
import { useDialog } from './ui/dialog-context';
import { useRole } from './ui/session-context';
import { useNavigation } from './ui/navigation-context';
import { useSiteScope } from './ui/site-scope-context';
import Select from './ui/Select';

const LIVE_POLL_MS = 20000;

type Mode = 'view' | 'edit';

const pinColor = (pin: FloorPlanPin) => {
  if (pin.target_plan_id) return 'bg-blue-400 border-blue-200';
  if (!pin.known) return 'bg-[#737373] border-white/40';
  if (!pin.online) return 'bg-[#737373] border-white/40';
  if (!pin.monitored) return 'bg-amber-400 border-amber-200';
  return 'bg-[#10b981] border-emerald-200';
};

/** Nome curto exibido junto ao ponto: hostname quando existe, senão o IP. */
const pinLabel = (pin: FloorPlanPin) => {
  if (pin.target_plan_id) return pin.label || 'Abrir planta';
  const name = pin.label || pin.hostname;
  if (!name) return pin.host_ip;
  // FQDN vira só o rótulo curto: "pc-rh.empresa.local" ocuparia a planta toda.
  return name.split('.')[0];
};

const pinTitle = (pin: FloorPlanPin) => {
  if (pin.target_plan_id) return `${pin.label || 'Planta'} (abrir)`;
  if (!pin.known) return `${pin.host_ip} — fora do inventário`;
  const estado = pin.online ? 'online' : 'offline';
  const agente = pin.monitored ? 'monitorado' : 'sem agente';
  return `${pin.hostname || pin.host_ip} — ${estado}, ${agente}`;
};

const FloorPlanView = () => {
  const dialog = useDialog();
  const { canOperate } = useRole();
  const { openMachine } = useNavigation();
  const { numericSiteId, siteName, sites, setSiteId, reloadSites } = useSiteScope();
  const [plans, setPlans] = useState<FloorPlan[]>([]);
  const [current, setCurrent] = useState<FloorPlan | null>(null);
  const [imageUrl, setImageUrl] = useState<string>('');
  const [pins, setPins] = useState<FloorPlanPin[]>([]);
  const [hosts, setHosts] = useState<NetworkHostView[]>([]);
  const [mode, setMode] = useState<Mode>('view');
  const [pendingHost, setPendingHost] = useState('');
  const [draggingHost, setDraggingHost] = useState('');
  const [dirty, setDirty] = useState(false);
  const [loading, setLoading] = useState(true);

  const imageRef = useRef<HTMLImageElement>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  const loadPlans = useCallback(async () => {
    try {
      const all = await api.floorPlans();
      // A planta pertence a uma unidade, então a tela só mostra as da unidade
      // em escopo. Sem unidade escolhida não há mapa a exibir — misturar
      // andares de filiais diferentes não significaria nada.
      const list = numericSiteId === null ? [] : all.filter(p => p.site_id === numericSiteId);
      setPlans(list);
      setCurrent(prev => (prev && list.some(p => p.id === prev.id) ? prev : list[0] ?? null));
    } catch (err) {
      console.error(err);
    } finally {
      setLoading(false);
    }
  }, [numericSiteId]);

  useEffect(() => {
    loadPlans();
  }, [loadPlans]);

  // A paleta de máquinas arrastáveis segue a unidade da planta, e não o parque
  // inteiro. Desde que o marcador passou a resolver o host pela chave
  // (unidade, ip), host de outra unidade nunca resolveria: a pessoa arrastava a
  // máquina que via na lista e a planta escrevia "fora do inventário".
  //
  // O recorte vai na consulta, não no filtro do componente: trazer o parque
  // para descartar no navegador é o mesmo desperdício com uma camada a mais.
  useEffect(() => {
    if (numericSiteId === null) {
      setHosts([]);
      return;
    }
    const controller = new AbortController();
    api.networkHosts(controller.signal, numericSiteId)
      .then(inv => setHosts(inv.hosts))
      .catch(() => {});
    return () => controller.abort();
  }, [numericSiteId]);

  // Imagem da planta: object URL precisa ser revogado ao trocar de planta,
  // senão o blob fica retido no browser.
  useEffect(() => {
    if (!current) {
      setImageUrl('');
      return;
    }
    const controller = new AbortController();
    let url = '';

    api.floorPlanImageUrl(current.id, controller.signal)
      .then(objectUrl => {
        url = objectUrl;
        setImageUrl(objectUrl);
      })
      .catch(() => {
        if (!controller.signal.aborted) setImageUrl('');
      });

    return () => {
      controller.abort();
      if (url) URL.revokeObjectURL(url);
    };
  }, [current]);

  const refreshPins = useCallback(async (signal?: AbortSignal) => {
    if (!current) return;
    try {
      const plan = await api.floorPlan(current.id, signal);
      setPins(plan.pins);
    } catch (err) {
      if (!signal?.aborted) console.error(err);
    }
  }, [current]);

  // Só faz polling em modo visualização: em edição isso sobrescreveria o
  // posicionamento que o operador ainda não gravou.
  useEffect(() => {
    if (!current) return;
    const controller = new AbortController();
    refreshPins(controller.signal);

    if (mode === 'edit') return () => controller.abort();

    const interval = setInterval(() => refreshPins(controller.signal), LIVE_POLL_MS);
    return () => {
      clearInterval(interval);
      controller.abort();
    };
  }, [current, mode, refreshPins]);

  // Criar a unidade aqui evita o vaivém: sem unidade cadastrada a tela pedia
  // para escolher uma na barra lateral, onde não havia nenhuma para escolher.
  const createSiteInline = async () => {
    const name = await dialog.prompt({
      title: 'Nova unidade',
      message: 'A planta pertence a uma unidade. Informe o nome do local.',
      placeholder: 'Ex: Matriz',
      confirmLabel: 'Continuar',
    });
    if (!name) return;

    const code = await dialog.prompt({
      title: `Código de ${name}`,
      message: 'Identificador curto, usado no AGENT_SITE das máquinas desta unidade.',
      placeholder: 'Ex: matriz',
      confirmLabel: 'Criar unidade',
    });
    if (!code) return;

    try {
      const site = await api.createSite({ name, code });
      reloadSites();
      setSiteId(String(site.id));
      dialog.notify(`Unidade "${site.name}" criada. Agora envie a planta de um andar.`, 'success');
    } catch (err) {
      dialog.notify((err as Error).message || 'Falha ao criar a unidade.', 'error');
    }
  };

  const handleUpload = async (file: File) => {
    // A planta pertence a uma unidade; várias plantas na mesma unidade são os
    // andares. Sem unidade escolhida não dá para saber onde ela entra.
    if (numericSiteId === null) {
      // Sem unidade não há onde pendurar a planta. Em vez de só recusar,
      // oferece o caminho: cadastrar a unidade agora.
      const criar = await dialog.confirm({
        title: 'A planta pertence a uma unidade',
        message:
          sites.length > 0
            ? 'Escolha a unidade na barra lateral e envie a planta de novo.'
            : 'Nenhuma unidade cadastrada. Quer criar uma agora?',
        confirmLabel: sites.length > 0 ? 'Entendi' : 'Criar unidade',
        cancelLabel: 'Cancelar',
      });
      if (fileRef.current) fileRef.current.value = '';
      if (criar && sites.length === 0) await createSiteInline();
      return;
    }

    const name = await dialog.prompt({
      title: `Nome da planta em ${siteName(numericSiteId)}`,
      message: 'Use o andar ou setor, já que a unidade vem do escopo. Ex: "2º andar" ou "Térreo — Recepção".',
      placeholder: 'Andar ou setor',
      confirmLabel: 'Enviar',
    });
    if (!name) return;

    try {
      const plan = await api.uploadFloorPlan(name, file, numericSiteId);
      await loadPlans();
      setCurrent(plan);
      setMode('edit');
      dialog.notify('Planta enviada. Arraste as máquinas da lista para o lugar delas.', 'success');
    } catch (err) {
      dialog.notify((err as Error).message || 'Falha ao enviar a planta.', 'error');
    } finally {
      if (fileRef.current) fileRef.current.value = '';
    }
  };

  const handleDeletePlan = async () => {
    if (!current) return;
    const confirmed = await dialog.confirm({
      title: `Remover a planta "${current.name}"?`,
      message: 'Os marcadores posicionados nela são perdidos. O inventário não é afetado.',
      confirmLabel: 'Remover',
      danger: true,
    });
    if (!confirmed) return;

    try {
      await api.deleteFloorPlan(current.id);
      setCurrent(null);
      setPins([]);
      await loadPlans();
    } catch (err) {
      dialog.notify((err as Error).message || 'Falha ao remover a planta.', 'error');
    }
  };

  /** Converte a posição do ponteiro em porcentagem da imagem. */
  const toPercent = (clientX: number, clientY: number) => {
    const rect = imageRef.current!.getBoundingClientRect();
    return {
      x: Math.max(0, Math.min(100, ((clientX - rect.left) / rect.width) * 100)),
      y: Math.max(0, Math.min(100, ((clientY - rect.top) / rect.height) * 100)),
    };
  };

  /** Cria ou mede de novo o marcador do host na coordenada informada. */
  const placeHost = (hostIP: string, clientX: number, clientY: number) => {
    if (!imageRef.current) return;
    const { x, y } = toPercent(clientX, clientY);

    setPins(prev => {
      const existing = prev.find(p => p.host_ip === hostIP);
      if (existing) {
        // Reposicionar preserva o estado já resolvido pelo backend.
        return prev.map(p => (p.host_ip === hostIP ? { ...p, x, y } : p));
      }
      const host = hosts.find(h => h.ip === hostIP);
      return [
        ...prev,
        {
          id: 0,
          host_ip: hostIP,
          label: host?.hostname || hostIP,
          x,
          y,
          target_plan_id: null,
          hostname: host?.hostname ?? '',
          device_type: host?.device_type ?? '',
          online: host?.online ?? false,
          monitored: host?.monitored ?? false,
          known: Boolean(host),
          server_id: '',
        },
      ];
    });
    setDirty(true);
  };

  // Clique na planta continua funcionando: em tela de toque o arrastar nativo
  // do HTML não dispara, então selecionar e tocar é o caminho que resta.
  const handlePlanClick = (e: React.MouseEvent<HTMLImageElement>) => {
    if (mode !== 'edit' || !pendingHost) return;
    placeHost(pendingHost, e.clientX, e.clientY);
    setPendingHost('');
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    if (mode !== 'edit') return;
    const hostIP = e.dataTransfer.getData('text/plain');
    if (hostIP) placeHost(hostIP, e.clientX, e.clientY);
    setDraggingHost('');
  };

  // Em edição o clique remove o marcador; em visualização abre a máquina —
  // é o caminho natural de "achei no mapa, quero ver como está".
  const handlePinClick = (pin: FloorPlanPin) => {
    if (mode === 'edit') {
      removePin(pin.host_ip);
      return;
    }
    if (pin.target_plan_id) {
      const target = plans.find(p => p.id === pin.target_plan_id);
      if (target) setCurrent(target);
      return;
    }
    if (pin.server_id) openMachine(pin.server_id);
  };

  const removePin = (hostIP: string) => {
    setPins(prev => prev.filter(p => p.host_ip !== hostIP));
    setDirty(true);
  };

  const savePins = async () => {
    if (!current) return;
    const payload: FloorPlanPinInput[] = pins.map(p => ({
      host_ip: p.host_ip,
      label: p.label,
      x: p.x,
      y: p.y,
      target_plan_id: p.target_plan_id,
    }));
    try {
      const saved = await api.savePins(current.id, payload);
      setPins(saved.pins);
      setDirty(false);
      dialog.notify('Posições gravadas.', 'success');
    } catch (err) {
      dialog.notify((err as Error).message || 'Falha ao gravar as posições.', 'error');
    }
  };

  const unplaced = hosts.filter(h => !pins.some(p => p.host_ip === h.ip));

  return (
    <div className="p-4 md:p-8 h-full flex flex-col overflow-hidden">
      <div className="mb-6 flex flex-col md:flex-row md:items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-light text-white mb-2 flex items-center gap-3">
            <Map className="text-[#10b981]" /> Planta <span className="font-bold">Baixa</span>
          </h1>
          <p className="text-[#737373] text-sm">
            {numericSiteId === null
              ? 'Cada unidade tem sua planta; escolha uma na barra lateral.'
              : `Andares de ${siteName(numericSiteId)}. Arraste cada máquina para o lugar dela.`}
          </p>
        </div>

        {canOperate && (
          <div className="flex items-center gap-2">
            <input
              ref={fileRef}
              type="file"
              accept="image/png,image/jpeg,image/gif"
              className="hidden"
              onChange={(e) => {
                const file = e.target.files?.[0];
                if (file) handleUpload(file);
              }}
            />
            <button
              onClick={() => fileRef.current?.click()}
              className="flex items-center gap-2 bg-[#10b981]/20 hover:bg-[#10b981]/30 border border-[#10b981]/50 text-[#10b981] font-bold text-xs uppercase tracking-widest px-4 py-3 rounded-lg transition-all"
            >
              <Upload size={14} />
              Nova planta
            </button>
            {current && (
              <>
                <button
                  onClick={() => setMode(mode === 'edit' ? 'view' : 'edit')}
                  className="flex items-center gap-2 border border-white/10 text-[#737373] hover:text-white font-bold text-xs uppercase tracking-widest px-4 py-3 rounded-lg transition-all"
                >
                  {mode === 'edit' ? <><Eye size={14} /> Visualizar</> : <><Pencil size={14} /> Editar</>}
                </button>
                <button
                  onClick={handleDeletePlan}
                  className="p-3 border border-white/10 text-[#737373] hover:text-rose-400 rounded-lg transition-colors"
                  title="Remover planta"
                >
                  <Trash2 size={14} />
                </button>
              </>
            )}
          </div>
        )}
      </div>

      {plans.length > 0 && (
        <div className="flex gap-2 flex-wrap mb-4 items-center">
          <span className="text-[10px] text-[#737373] uppercase tracking-widest mr-1">Andares</span>
          {plans.map(plan => (
            <button
              key={plan.id}
              onClick={() => { setCurrent(plan); setMode('view'); setDirty(false); }}
              className={`px-3 py-2 rounded-lg text-xs font-bold tracking-widest uppercase transition-all border ${
                current?.id === plan.id
                  ? 'bg-[#10b981]/20 border-[#10b981]/50 text-[#10b981]'
                  : 'bg-black/40 border-white/10 text-[#737373] hover:text-white'
              }`}
            >
              {plan.name}
            </button>
          ))}
        </div>
      )}

      <div className="flex-1 min-h-0 grid grid-cols-1 lg:grid-cols-4 gap-4">
        <div className="lg:col-span-3 glass-panel rounded-xl border border-white/5 bg-white/[0.02] overflow-auto custom-scrollbar p-4">
          {loading ? (
            <div className="h-full flex items-center justify-center text-sm text-[#737373]">Carregando...</div>
          ) : numericSiteId === null ? (
            <div className="h-full flex flex-col items-center justify-center text-sm text-[#737373] gap-3 text-center px-6">
              <Building2 size={32} className="opacity-30" />
              <span className="text-xs max-w-sm">
                Cada planta é o andar de uma unidade, então o mapa só faz sentido dentro de uma delas.
              </span>

              {sites.length > 0 ? (
                <div className="w-56">
                  <label htmlFor="plan-site" className="text-[10px] uppercase tracking-widest block mb-1.5">
                    Escolha a unidade
                  </label>
                  <Select
                    id="plan-site"
                    value=""
                    onChange={setSiteId}
                    placeholder="Selecione..."
                    options={sites.map((site) => ({ value: String(site.id), label: site.name }))}
                  />
                </div>
              ) : (
                <>
                  <span>Nenhuma unidade cadastrada ainda.</span>
                  {canOperate && (
                    <button
                      onClick={createSiteInline}
                      className="flex items-center gap-2 bg-[#10b981]/20 hover:bg-[#10b981]/30 border border-[#10b981]/50 text-[#10b981] font-bold text-xs uppercase tracking-widest px-4 py-3 rounded-lg transition-all"
                    >
                      <Plus size={14} />
                      Criar unidade
                    </button>
                  )}
                </>
              )}
            </div>
          ) : !current ? (
            <div className="h-full flex flex-col items-center justify-center text-sm text-[#737373] gap-2 text-center px-6">
              <Map size={32} className="opacity-30" />
              <span>Nenhuma planta em {siteName(numericSiteId)}.</span>
              <span className="text-xs">Envie a imagem de um andar para começar.</span>
            </div>
          ) : !imageUrl ? (
            <div className="h-full flex items-center justify-center text-sm text-[#737373]">Carregando a imagem...</div>
          ) : (
            <div
              className={`relative inline-block max-w-full rounded-lg transition-shadow ${
                draggingHost ? 'ring-2 ring-[#10b981]/60' : ''
              }`}
              onDragOver={(e) => {
                // Sem o preventDefault o navegador recusa a área como destino.
                if (mode === 'edit') e.preventDefault();
              }}
              onDrop={handleDrop}
            >
              <img
                ref={imageRef}
                src={imageUrl}
                alt={current.name}
                onClick={handlePlanClick}
                className={`max-w-full h-auto rounded-lg ${mode === 'edit' && pendingHost ? 'cursor-crosshair' : ''}`}
              />
              {pins.map(pin => (
                <button
                  key={`${pin.host_ip}-${pin.x}-${pin.y}`}
                  title={mode === 'edit' ? `${pinTitle(pin)} — arraste para mover, clique para remover` : pinTitle(pin)}
                  onClick={() => handlePinClick(pin)}
                  draggable={mode === 'edit'}
                  onDragStart={(e) => {
                    e.dataTransfer.setData('text/plain', pin.host_ip);
                    e.dataTransfer.effectAllowed = 'move';
                    setDraggingHost(pin.host_ip);
                  }}
                  onDragEnd={() => setDraggingHost('')}
                  style={{ left: `${pin.x}%`, top: `${pin.y}%` }}
                  className={`absolute -translate-x-1/2 -translate-y-1/2 flex flex-col items-center gap-1 group ${
                    mode === 'edit' || pin.server_id ? 'cursor-pointer' : 'cursor-default'
                  }`}
                >
                  <span
                    className={`w-4 h-4 rounded-full border-2 shadow-lg transition-transform group-hover:scale-125 ${pinColor(pin)}`}
                  />
                  {/* O nome ao lado do ponto é o que responde "qual máquina é
                      essa" sem passar o mouse em cada marcador. */}
                  <span className="px-1.5 py-0.5 rounded bg-[#0c0c0e]/90 border border-white/10 text-[10px] leading-none text-white/90 whitespace-nowrap shadow-md max-w-[140px] truncate">
                    {pinLabel(pin)}
                  </span>
                </button>
              ))}
            </div>
          )}
        </div>

        <div className="glass-panel rounded-xl border border-white/5 bg-white/[0.02] flex flex-col min-h-0">
          <div className="p-4 border-b border-white/5 flex items-center justify-between">
            <h2 className="text-xs font-bold tracking-widest text-[#737373] uppercase">
              {mode === 'edit' ? 'Posicionar' : 'Legenda'}
            </h2>
            {dirty && (
              <button
                onClick={savePins}
                className="flex items-center gap-1 text-[10px] font-bold uppercase tracking-widest text-[#10b981]"
              >
                <Save size={12} /> Gravar
              </button>
            )}
          </div>

          {mode === 'edit' ? (
            <div className="flex-1 overflow-y-auto custom-scrollbar p-3 flex flex-col gap-1">
              <p className="text-[10px] text-[#737373] px-2 pb-2 leading-relaxed">
                Arraste uma máquina para o lugar dela na planta. Marcador já
                posicionado também se arrasta; clique nele para remover.
              </p>
              {pendingHost && (
                <div className="mb-2 p-2 rounded border border-[#10b981]/30 bg-[#10b981]/10 text-[11px] text-[#10b981] flex items-center justify-between">
                  <span>Clique na planta para posicionar</span>
                  <button onClick={() => setPendingHost('')} aria-label="Cancelar"><X size={12} /></button>
                </div>
              )}
              {unplaced.length === 0 && (
                <p className="text-xs text-[#737373] p-2">Todos os hosts do inventário já estão na planta.</p>
              )}
              {unplaced.map(host => (
                <button
                  key={host.ip}
                  draggable
                  onDragStart={(e) => {
                    e.dataTransfer.setData('text/plain', host.ip);
                    e.dataTransfer.effectAllowed = 'copy';
                    setDraggingHost(host.ip);
                  }}
                  onDragEnd={() => setDraggingHost('')}
                  onClick={() => setPendingHost(host.ip)}
                  className={`text-left px-2 py-1.5 rounded text-xs transition-colors border ${
                    pendingHost === host.ip
                      ? 'border-[#10b981]/50 bg-[#10b981]/10 text-[#10b981]'
                      : 'border-transparent text-gray-400 hover:bg-white/5'
                  }`}
                >
                  <span className="flex items-center gap-2">
                    <Plus size={11} className="shrink-0" />
                    <span className="font-mono">{host.ip}</span>
                  </span>
                  {host.hostname && <span className="block ml-5 text-[10px] text-[#737373]">{host.hostname}</span>}
                </button>
              ))}
            </div>
          ) : (
            <div className="p-4 flex flex-col gap-3 text-xs text-[#737373]">
              {([
                ['bg-[#10b981]', 'Online e monitorado'],
                ['bg-amber-400', 'Online, sem agente'],
                ['bg-[#737373]', 'Offline ou fora do inventário'],
                ['bg-blue-400', 'Leva a outra planta'],
              ] as const).map(([color, label]) => (
                <span key={label} className="flex items-center gap-2">
                  <span className={`w-3 h-3 rounded-full border-2 border-white/30 ${color}`} />
                  {label}
                </span>
              ))}
              <p className="mt-2 pt-3 border-t border-white/5 leading-relaxed">
                {pins.length} marcador(es) nesta planta.
                {pins.length === 0 && canOperate && ' Entre em Editar para posicionar as máquinas.'}
              </p>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};

export default FloorPlanView;
