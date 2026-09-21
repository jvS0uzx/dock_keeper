import { describe, expect, it } from 'vitest';
import { unidadeEfetiva } from './panels';

describe('filtro de unidade só vale onde o seletor aparece', () => {
  it('no painel Suporte TI, a unidade escolhida filtra', () => {
    expect(unidadeEfetiva('suporte', '3')).toBe(3);
    expect(unidadeEfetiva('suporte', 'all')).toBeNull();
  });

  it('no painel Infra / Dev não há seletor, então unidade esquecida não filtra às escondidas', () => {
    expect(unidadeEfetiva('dev', '3')).toBeNull();
  });
});
