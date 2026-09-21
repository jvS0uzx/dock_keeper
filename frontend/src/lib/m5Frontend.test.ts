import { describe, expect, it } from 'vitest';
import { detalheDoCaminho } from './rotas';
import { UPSTREAM_LOCAL, deriveUpstreams, splitUpstreams } from './upstream';

describe('rota de detalhe com URL malformada', () => {
  it('não lança: cai como caminho sem detalhe', () => {
    expect(() => detalheDoCaminho('/maquinas/%E0%A4%A')).not.toThrow();
    expect(detalheDoCaminho('/maquinas/%E0%A4%A')).toBeNull();
  });

  it('continua decodificando o id válido', () => {
    expect(detalheDoCaminho('/maquinas/est%C3%A7%C3%A3o-01')).toEqual({
      kind: 'machine',
      id: 'estção-01',
    });
  });
});

describe('resposta local do nginx não é upstream', () => {
  it('o literal gravado pelo backend sai da lista de endereços', () => {
    expect(UPSTREAM_LOCAL).toBe('Local (Nginx/Cache)');
    expect(splitUpstreams('Local (Nginx/Cache)')).toEqual([]);
    expect(splitUpstreams('203.0.113.25:80, Local (Nginx/Cache)')).toEqual(['203.0.113.25:80']);
  });

  it('a malha não ganha nó para a resposta local', () => {
    const nos = deriveUpstreams([
      { upstream_addr: 'Local (Nginx/Cache)', server_name: 'x', status: '200', requests_count: 9 },
      { upstream_addr: '203.0.113.25:80', server_name: 'x', status: '200', requests_count: 4 },
    ]);
    expect(nos.map((n) => n.addr)).toEqual(['203.0.113.25:80']);
  });
});
