import { forwardRef, useId } from 'react';
import type { InputHTMLAttributes } from 'react';

const cx = (...c: Array<string | false | null | undefined>) => c.filter(Boolean).join(' ');

interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  label: string;
  error?: string;
  hint?: string;
}

// Campo com label acima (sentence case, cinza). Erro vira borda vermelha +
// mensagem, e liga `aria-invalid`/`aria-describedby` para leitores de tela.
export const Input = forwardRef<HTMLInputElement, InputProps>(function Input(
  { label, error, hint, className, id, ...rest },
  ref,
) {
  const autoId = useId();
  const fieldId = id ?? autoId;
  const errorId = `${autoId}-error`;
  const hintId = `${autoId}-hint`;
  // Erro esconde o hint, então só o referenciamos quando ele aparece de fato.
  const describedBy = cx(hint && !error && hintId, error && errorId) || undefined;

  return (
    <div className="flex flex-col gap-1.5">
      <label htmlFor={fieldId} className="text-xs font-bold text-muted">
        {label}
      </label>
      <input
        ref={ref}
        id={fieldId}
        aria-invalid={error ? true : undefined}
        aria-describedby={describedBy}
        className={cx(
          'h-12 rounded-sm border bg-white px-3.5 text-[15px] text-ink outline-none transition',
          'placeholder:text-muted',
          'focus:border-primary-300',
          error ? 'border-error' : 'border-primary-200',
          className,
        )}
        {...rest}
      />
      {hint && !error && (
        <span id={hintId} className="text-[12.5px] text-muted">
          {hint}
        </span>
      )}
      {error && (
        <span id={errorId} className="text-[12.5px] text-error">
          {error}
        </span>
      )}
    </div>
  );
});
