const BYTE_UNITS = ['B', 'KB', 'MB', 'GB', 'TB'];

export const formatBytes = (bytes: number): string => {
  if (!bytes || bytes <= 0) return '0 B';
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), BYTE_UNITS.length - 1);
  return `${parseFloat((bytes / 1024 ** i).toFixed(2))} ${BYTE_UNITS[i]}`;
};

export const formatGB = (bytes: number): string => (bytes / 1024 ** 3).toFixed(1);

export const relativeTime = (iso: string | null): string => {
  if (!iso) return 'nunca';
  const seconds = Math.floor((Date.now() - new Date(iso).getTime()) / 1000);
  if (seconds < 60) return `há ${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `há ${minutes}min`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `há ${hours}h`;
  return `há ${Math.floor(hours / 24)}d`;
};

export const formatDateTime = (iso: string): string => {
  const date = new Date(iso);
  return isNaN(date.getTime()) ? iso : date.toLocaleString('pt-BR');
};

export const formatRate = (bytesPerSecond: number | null): string =>
  bytesPerSecond === null ? '—' : `${formatBytes(bytesPerSecond)}/s`;

export const formatPercent = (valor: number | null, casas = 0): string =>
  valor === null ? '—' : `${valor.toFixed(casas)}%`;

export const formatLoad = (valor: number | null): string =>
  valor === null ? '—' : valor.toFixed(2);

export const formatLatency = (ms: number | null): string => {
  if (ms === null) return '—';
  if (ms < 1000) return `${Math.round(ms)} ms`;
  return `${(ms / 1000).toFixed(1).replace('.', ',')} s`;
};

const UNIDADES_DE_BIT = ['bps', 'kbps', 'Mbps', 'Gbps', 'Tbps'];

const numeroCurto = (valor: number): string =>
  valor.toLocaleString('pt-BR', { maximumFractionDigits: 1 });

export const formatBps = (bps: number | null | undefined): string => {
  if (bps === null || bps === undefined) return '—';
  if (bps < 1000) return `${Math.round(bps)} bps`;
  const escala = Math.min(Math.floor(Math.log10(bps) / 3), UNIDADES_DE_BIT.length - 1);
  return `${numeroCurto(bps / 1000 ** escala)} ${UNIDADES_DE_BIT[escala]}`;
};

export const formatVelocidade = (mbps: number | null | undefined): string => {
  if (mbps === null || mbps === undefined) return '—';
  if (mbps >= 1000) return `${numeroCurto(mbps / 1000)} Gbps`;
  return `${numeroCurto(mbps)} Mbps`;
};

export const formatUptime = (segundos: number | null | undefined): string => {
  if (segundos === null || segundos === undefined) return '—';
  const dias = Math.floor(segundos / 86400);
  const horas = Math.floor((segundos % 86400) / 3600);
  const minutos = Math.floor((segundos % 3600) / 60);
  if (dias > 0) return `${dias}d ${horas}h`;
  if (horas > 0) return `${horas}h ${minutos}min`;
  return `${minutos}min`;
};

export const formatContador = (valor: number | null | undefined): string =>
  valor === null || valor === undefined ? '—' : valor.toLocaleString('pt-BR');
