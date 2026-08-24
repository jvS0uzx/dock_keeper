import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { AlertTriangle, CheckCircle2, Info, X, XCircle } from 'lucide-react';
import {
  DialogContext,
  type ConfirmOptions,
  type DialogApi,
  type NoticeTone,
  type PromptOptions,
} from './dialog-context';

interface Notice {
  id: number;
  message: string;
  tone: NoticeTone;
}

type Pending =
  | { kind: 'confirm'; options: ConfirmOptions; resolve: (value: boolean) => void }
  | { kind: 'prompt'; options: PromptOptions; resolve: (value: string | null) => void };

const NOTICE_TTL_MS = 5000;

const toneStyle: Record<NoticeTone, { border: string; text: string; Icon: typeof Info }> = {
  success: { border: 'border-ok/30', text: 'text-ok', Icon: CheckCircle2 },
  error: { border: 'border-crit/30', text: 'text-crit', Icon: XCircle },
  info: { border: 'border-line-hi', text: 'text-text', Icon: Info },
};

/**
 * Substitui window.alert/confirm/prompt por UI própria: os diálogos nativos
 * travam a thread do browser e quebram o visual do painel.
 */
export const DialogProvider = ({ children }: { children: ReactNode }) => {
  const [pending, setPending] = useState<Pending | null>(null);
  const [draft, setDraft] = useState('');
  const [notices, setNotices] = useState<Notice[]>([]);
  const noticeId = useRef(0);
  const inputRef = useRef<HTMLInputElement>(null);

  const settle = useCallback((accepted: boolean) => {
    if (!pending) return;
    if (pending.kind === 'confirm') pending.resolve(accepted);
    else pending.resolve(accepted ? draft.trim() || null : null);
    setPending(null);
  }, [pending, draft]);

  const api = useMemo<DialogApi>(() => ({
    confirm: (options) =>
      new Promise<boolean>((resolve) => {
        setDraft('');
        setPending({ kind: 'confirm', options, resolve });
      }),
    prompt: (options) =>
      new Promise<string | null>((resolve) => {
        setDraft(options.initialValue ?? '');
        setPending({ kind: 'prompt', options, resolve });
      }),
    notify: (message, tone = 'info') => {
      const id = ++noticeId.current;
      setNotices((prev) => [...prev, { id, message, tone }]);
      setTimeout(() => setNotices((prev) => prev.filter((n) => n.id !== id)), NOTICE_TTL_MS);
    },
  }), []);

  useEffect(() => {
    if (!pending) return;
    inputRef.current?.focus();
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') settle(false);
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [pending, settle]);

  const isDanger = pending?.kind === 'confirm' && pending.options.danger;

  return (
    <DialogContext.Provider value={api}>
      {children}

      {pending && (
        <div
          className="fixed inset-0 z-[100] flex items-center justify-center bg-black/60 backdrop-blur-sm p-4"
          onClick={() => settle(false)}
        >
          <div
            role="dialog"
            aria-modal="true"
            aria-label={pending.options.title}
            className="panel anim-rise w-full max-w-md shadow-pop"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-start gap-3 border-b border-line p-5">
              {isDanger && (
                <AlertTriangle size={18} strokeWidth={1.75} className="mt-0.5 shrink-0 text-crit" />
              )}
              <div className="flex-1">
                <h2 className="font-semibold tracking-tight text-text-hi">{pending.options.title}</h2>
                {pending.options.message && (
                  <p className="mt-1 text-sm text-text-mut">{pending.options.message}</p>
                )}
              </div>
              <button
                onClick={() => settle(false)}
                aria-label="Fechar"
                className="text-text-faint transition-colors hover:text-text-hi"
              >
                <X size={18} strokeWidth={1.75} />
              </button>
            </div>

            {pending.kind === 'prompt' && (
              <div className="p-5 pb-0">
                <input
                  ref={inputRef}
                  type="text"
                  value={draft}
                  placeholder={pending.options.placeholder}
                  onChange={(e) => setDraft(e.target.value)}
                  onKeyDown={(e) => e.key === 'Enter' && settle(true)}
                  className="input-base w-full"
                />
              </div>
            )}

            <div className="flex justify-end gap-2 p-5">
              <button onClick={() => settle(false)} className="btn btn-ghost">
                {(pending.kind === 'confirm' && pending.options.cancelLabel) || 'Cancelar'}
              </button>
              <button
                onClick={() => settle(true)}
                disabled={pending.kind === 'prompt' && !draft.trim()}
                className={`btn disabled:opacity-40 ${isDanger ? 'btn-danger' : 'btn-primary'}`}
              >
                {pending.options.confirmLabel || 'Confirmar'}
              </button>
            </div>
          </div>
        </div>
      )}

      <div className="pointer-events-none fixed bottom-6 right-6 z-[110] flex flex-col gap-2">
        {notices.map(({ id, message, tone }) => {
          const { border, text, Icon } = toneStyle[tone];
          return (
            <div
              key={id}
              role="status"
              className={`pointer-events-auto flex max-w-sm items-start gap-2 rounded-ctrl border bg-ink-800/95 px-4 py-3 text-sm shadow-pop backdrop-blur ${border} ${text}`}
            >
              <Icon size={16} strokeWidth={1.75} className="mt-0.5 shrink-0" />
              <span className="selectable">{message}</span>
            </div>
          );
        })}
      </div>
    </DialogContext.Provider>
  );
};
