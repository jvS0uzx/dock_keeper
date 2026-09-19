let apiUrl = import.meta.env.VITE_API_URL || '';

export async function loadRuntimeConfig(): Promise<void> {
  try {
    const resp = await fetch('/config.json', { cache: 'no-store' });
    if (!resp.ok) return;

    const cfg: unknown = await resp.json();
    if (cfg && typeof cfg === 'object' && 'apiUrl' in cfg) {
      const valor = (cfg as { apiUrl?: unknown }).apiUrl;
      if (typeof valor === 'string' && valor.trim() !== '') {
        apiUrl = valor.trim();
      }
    }
  } catch {
  }
}

export function apiBase(): string {
  return apiUrl;
}

export const API_TOKEN: string = import.meta.env.DEV ? import.meta.env.VITE_API_TOKEN || '' : '';
