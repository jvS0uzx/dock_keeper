import userEvent from '@testing-library/user-event';
import { act } from '@testing-library/react';
import { vi } from 'vitest';

export const semEspera = () => userEvent.setup({ delay: null });

export const comRelogioFalso = () => {
  vi.useFakeTimers({ shouldAdvanceTime: true });
  return userEvent.setup({ advanceTimers: vi.advanceTimersByTimeAsync });
};

export const avancar = (ms: number) => act(() => vi.advanceTimersByTimeAsync(ms));
