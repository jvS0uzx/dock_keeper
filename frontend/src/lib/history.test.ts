import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { api } from './api';
import { responder } from '../test/recharts';

let urls: string[];

beforeEach(() => {
  urls = [];
  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
    urls.push(String(input));
    return responder(200, []);
  }));
});

afterEach(() => vi.unstubAllGlobals());

const params = (i = 0) => new URL(urls[i], 'http://x').searchParams;

describe('api.history — janela', () => {
  it('janela pronta vai como range', async () => {
    await api.history('srv-1', 'cpu', '30d');
    expect(params().get('range')).toBe('30d');
    expect(params().has('from')).toBe(false);
  });

  it('período personalizado vai como from/to, sem range', async () => {
    await api.history('srv-1', 'net_rx', { from: '2026-09-01T00:00:00.000Z', to: '2026-09-02T00:00:00.000Z' });
    expect(params().get('from')).toBe('2026-09-01T00:00:00.000Z');
    expect(params().get('to')).toBe('2026-09-02T00:00:00.000Z');
    expect(params().has('range')).toBe(false);
    expect(params().get('metric')).toBe('net_rx');
  });

  it('to é opcional', async () => {
    await api.history('srv-1', 'cpu', { from: '2026-09-01T00:00:00.000Z' });
    expect(params().has('to')).toBe(false);
    expect(params().has('range')).toBe(false);
  });
});
