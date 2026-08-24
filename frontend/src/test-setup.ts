import { afterEach } from 'vitest';
import { cleanup } from '@testing-library/react';

// Desmonta o que o teste montou. Sem isso o componente do teste anterior
// continua no documento, e uma busca por texto acha o elemento errado — a falha
// aparece no teste seguinte, não no que a causou, e o rastro fica ilegível.
afterEach(() => {
  cleanup();
  localStorage.clear();
});
