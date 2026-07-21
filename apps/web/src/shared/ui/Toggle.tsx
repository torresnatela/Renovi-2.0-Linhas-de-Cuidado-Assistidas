const cx = (...c: Array<string | false | null | undefined>) => c.filter(Boolean).join(' ');

interface ToggleProps {
  checked: boolean;
  onChange: (checked: boolean) => void;
  label: string;
}

// Switch on/off. `<button role="switch">` já dá Space/Enter e foco nativos; o
// `label` vira nome acessível (a tela costuma mostrar o texto ao lado).
export function Toggle({ checked, onChange, label }: ToggleProps) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      onClick={() => onChange(!checked)}
      className={cx(
        'inline-flex h-[27px] w-[46px] shrink-0 items-center rounded-pill p-[3px] transition',
        'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-300 focus-visible:ring-offset-2',
        checked ? 'bg-primary-300' : 'bg-primary-200',
      )}
    >
      <span
        className={cx(
          'h-[21px] w-[21px] rounded-full bg-white transition-transform',
          checked && 'translate-x-[19px]',
        )}
      />
    </button>
  );
}
