import type { InterfaceDeRede, NetworkHostView } from './api';

interface Marca {
  texto: string;
  classe: string;
}

const MARCA_DO_ESTADO: Record<string, Marca> = {
  up: { texto: 'Up', classe: 'badge-ok' },
  down: { texto: 'Down', classe: 'badge-crit' },
  lowerLayerDown: { texto: 'Down (camada inferior)', classe: 'badge-crit' },
  dormant: { texto: 'Dormente', classe: 'badge-warn' },
  testing: { texto: 'Em teste', classe: 'badge-warn' },
  notPresent: { texto: 'Ausente', classe: 'badge-muted' },
  unknown: { texto: 'Desconhecido', classe: 'badge-muted' },
};

export const marcaDaInterface = (itf: Pick<InterfaceDeRede, 'oper_status' | 'admin_status'>): Marca => {
  if (itf.admin_status === 'down') return { texto: 'Desativada', classe: 'badge-muted' };
  return MARCA_DO_ESTADO[itf.oper_status] ?? MARCA_DO_ESTADO.unknown;
};

export const nomeDaInterface = (itf: Pick<InterfaceDeRede, 'if_name' | 'if_descr' | 'if_index'>): string =>
  itf.if_name || itf.if_descr || `ifIndex ${itf.if_index}`;

export const temSNMP = (host: Pick<NetworkHostView, 'snmp_visto_em' | 'snmp_erro'>): boolean =>
  Boolean(host.snmp_visto_em) || Boolean(host.snmp_erro);
