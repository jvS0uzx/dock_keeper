export const ALTURA_CARTAO = 72;
export const RESPIRO_ENTRE_CARTOES = 16;
export const PASSO_CARTAO = ALTURA_CARTAO + RESPIRO_ENTRE_CARTOES;

export const ALTURA_BALANCEADOR = 76;
export const RESPIRO_ENTRE_BALANCEADORES = 20;
export const PASSO_BALANCEADOR = ALTURA_BALANCEADOR + RESPIRO_ENTRE_BALANCEADORES;

export const ALTURA_MINIMA_MALHA = 208;
export const ALTURA_MAXIMA_MALHA = 720;
export const LARGURA_PADRAO_MALHA = 880;

export const alturaDoConteudo = (receptores: number, balanceadores: number): number =>
  Math.max(ALTURA_MINIMA_MALHA, receptores * PASSO_CARTAO, balanceadores * PASSO_BALANCEADOR);

export const alturaVisivel = (conteudo: number): number => Math.min(conteudo, ALTURA_MAXIMA_MALHA);

export const centroDaLinha = (indice: number, total: number, altura: number, passo: number): number =>
  (altura - total * passo) / 2 + passo * (indice + 0.5);
