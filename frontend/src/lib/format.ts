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
