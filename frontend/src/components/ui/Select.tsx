import { useCallback, useEffect, useId, useLayoutEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { ChevronDown } from 'lucide-react';

const MAX_LIST_HEIGHT = 256;
const GAP = 8;

export interface SelectOption {
  value: string;
  label: string;
}

interface SelectProps {
  options: SelectOption[];
  value: string;
  onChange: (value: string) => void;
  id?: string;
  ariaLabel?: string;
  placeholder?: string;
  disabled?: boolean;
  className?: string;
}

const Select = ({
  options,
  value,
  onChange,
  id,
  ariaLabel,
  placeholder = 'Selecione...',
  disabled = false,
  className = '',
}: SelectProps) => {
  const [isOpen, setIsOpen] = useState(false);
  const [activeIndex, setActiveIndex] = useState(-1);

  const containerRef = useRef<HTMLDivElement>(null);
  const buttonRef = useRef<HTMLButtonElement>(null);
  const listRef = useRef<HTMLUListElement>(null);

  const [rect, setRect] = useState<{ top: number; left: number; width: number } | null>(null);
  const generatedId = useId();
  const listboxId = `${id ?? generatedId}-listbox`;

  const selectedIndex = options.findIndex((o) => o.value === value);
  const selected = selectedIndex >= 0 ? options[selectedIndex] : undefined;

  const measure = useCallback(() => {
    const button = buttonRef.current;
    if (!button) return;

    const box = button.getBoundingClientRect();
    const below = window.innerHeight - box.bottom;
    const openUp = below < MAX_LIST_HEIGHT + GAP && box.top > below;

    setRect({
      top: openUp ? box.top - GAP : box.bottom + GAP,
      left: box.left,
      width: box.width,
    });
  }, []);

  useLayoutEffect(() => {
    if (isOpen) measure();
  }, [isOpen, measure]);

  useEffect(() => {
    if (!isOpen) return;

    const onPointerDown = (event: PointerEvent) => {
      const target = event.target as Node;
      if (!containerRef.current?.contains(target) && !listRef.current?.contains(target)) {
        setIsOpen(false);
      }
    };
    document.addEventListener('pointerdown', onPointerDown);
    window.addEventListener('scroll', measure, true);
    window.addEventListener('resize', measure);
    return () => {
      document.removeEventListener('pointerdown', onPointerDown);
      window.removeEventListener('scroll', measure, true);
      window.removeEventListener('resize', measure);
    };
  }, [isOpen, measure]);

  useEffect(() => {
    if (!isOpen || activeIndex < 0) return;
    const item = listRef.current?.children[activeIndex] as HTMLElement | undefined;
    item?.scrollIntoView({ block: 'nearest' });
  }, [isOpen, activeIndex]);

  const open = () => {
    if (disabled) return;
    setActiveIndex(selectedIndex >= 0 ? selectedIndex : 0);
    setIsOpen(true);
  };

  const commit = (index: number) => {
    const option = options[index];
    if (option) onChange(option.value);
    setIsOpen(false);
  };

  const handleKeyDown = (event: React.KeyboardEvent) => {
    if (disabled) return;

    switch (event.key) {
      case 'Enter':
      case ' ':
        event.preventDefault();
        if (isOpen) commit(activeIndex);
        else open();
        break;
      case 'Escape':
        if (isOpen) {
          event.preventDefault();
          setIsOpen(false);
        }
        break;
      case 'ArrowDown':
        event.preventDefault();
        if (!isOpen) open();
        else setActiveIndex((i) => Math.min(i + 1, options.length - 1));
        break;
      case 'ArrowUp':
        event.preventDefault();
        if (!isOpen) open();
        else setActiveIndex((i) => Math.max(i - 1, 0));
        break;
      case 'Home':
        if (isOpen) {
          event.preventDefault();
          setActiveIndex(0);
        }
        break;
      case 'End':
        if (isOpen) {
          event.preventDefault();
          setActiveIndex(options.length - 1);
        }
        break;
      case 'Tab':
        setIsOpen(false);
        break;
    }
  };

  return (
    <div ref={containerRef} className={`relative ${className}`}>
      <button
        ref={buttonRef}
        id={id}
        type="button"
        role="combobox"
        aria-haspopup="listbox"
        aria-expanded={isOpen}
        aria-controls={isOpen ? listboxId : undefined}
        aria-label={ariaLabel}
        disabled={disabled}
        onClick={() => (isOpen ? setIsOpen(false) : open())}
        onKeyDown={handleKeyDown}
        className="input-base flex w-full items-center justify-between gap-2 text-left hover:border-line-hi disabled:opacity-40 disabled:hover:border-line"
      >
        <span className={`truncate ${selected ? 'text-text-hi' : 'text-text-faint'}`}>
          {selected?.label ?? placeholder}
        </span>
        <ChevronDown
          size={16}
          strokeWidth={1.75}
          aria-hidden="true"
          className={`shrink-0 text-text-faint transition-transform ${isOpen ? 'rotate-180' : ''}`}
        />
      </button>

      {isOpen && rect && createPortal(
        <ul
          ref={listRef}
          id={listboxId}
          role="listbox"
          aria-label={ariaLabel}
          tabIndex={-1}
          style={{
            position: 'fixed',
            top: rect.top,
            left: rect.left,
            width: rect.width,
            maxHeight: MAX_LIST_HEIGHT,
            transform: rect.top < (buttonRef.current?.getBoundingClientRect().top ?? 0)
              ? 'translateY(-100%)'
              : undefined,
          }}
          className="overflow-y-auto custom-scrollbar rounded-ctrl border border-line-hi bg-ink-800 shadow-pop z-[200]"
        >
          {options.length === 0 && (
            <li className="px-3.5 py-2.5 text-sm text-text-faint">Nenhuma opção</li>
          )}
          {options.map((option, index) => {
            const isSelected = option.value === value;
            const isActive = index === activeIndex;
            return (
              <li
                key={option.value}
                role="option"
                aria-selected={isSelected}
                onPointerEnter={() => setActiveIndex(index)}
                onClick={() => commit(index)}
                className={`cursor-pointer px-3.5 py-2.5 text-sm transition-colors ${
                  isSelected
                    ? 'font-medium text-accent'
                    : isActive
                      ? 'bg-ink-750 text-text-hi'
                      : 'text-text'
                }`}
              >
                {option.label}
              </li>
            );
          })}
        </ul>,
        document.body,
      )}
    </div>
  );
};

export default Select;
