import { createContext, useContext } from 'react';

export interface ConfirmOptions {
  title: string;
  message?: string;
  confirmLabel?: string;
  cancelLabel?: string;
  danger?: boolean;
}

export interface PromptOptions {
  title: string;
  message?: string;
  placeholder?: string;
  initialValue?: string;
  confirmLabel?: string;
}

export type NoticeTone = 'success' | 'error' | 'info';

export interface DialogApi {
  confirm: (options: ConfirmOptions) => Promise<boolean>;
  prompt: (options: PromptOptions) => Promise<string | null>;
  notify: (message: string, tone?: NoticeTone) => void;
}

export const DialogContext = createContext<DialogApi | null>(null);

export const useDialog = (): DialogApi => {
  const ctx = useContext(DialogContext);
  if (!ctx) throw new Error('useDialog precisa estar dentro de <DialogProvider>');
  return ctx;
};
