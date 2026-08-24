// Vocabulário único do painel para métrica que a fonte de coleta não fornece.
//
// Cada fonte preenche um subconjunto das métricas: o agente das estações lê
// temperatura mas não abre sessão SSH; o stream SSH das VPS mede o handshake
// mas não lê sensor. Antes disso o backend gravava zero nos dois casos e a tela
// exibia "0 °C" e "0 ms" como se fossem leituras reais — o gráfico de handshake
// de uma estação era uma reta em zero. Agora o valor chega nulo, e o texto
// abaixo precisa ser o mesmo em toda tela para o operador não achar que são
// situações diferentes.

export const NO_TEMPERATURE = 'sem sensor';
export const NO_TEMPERATURE_HINT =
  'Esta fonte de coleta não informa temperatura para esta máquina.';

export const NO_HANDSHAKE = 'não disponível';
export const NO_HANDSHAKE_HINT =
  'Só máquinas coletadas por SSH têm tempo de handshake. Estações com agente não abrem sessão.';

// O que a tela chamava de "Latência". O nome mentia: o valor é o handshake SSH
// completo, tipicamente 1000-1400 ms nas VPS, contra RTT na casa das dezenas.
export const HANDSHAKE_LABEL = 'Handshake SSH';

// Zero medido continua sendo exibido como zero; só o nulo vira texto.
export const formatTemperature = (celsius: number | null): string =>
  celsius === null ? NO_TEMPERATURE : `${celsius.toFixed(0)}°C`;

export const formatHandshake = (ms: number | null): string =>
  ms === null ? NO_HANDSHAKE : `${ms.toFixed(0)} ms`;

// Comparação de limiar precisa ignorar o nulo em vez de tratá-lo como zero:
// máquina sem sensor não é máquina fria, é máquina sem leitura.
export const isAbove = (value: number | null, threshold: number): boolean =>
  value !== null && value >= threshold;
