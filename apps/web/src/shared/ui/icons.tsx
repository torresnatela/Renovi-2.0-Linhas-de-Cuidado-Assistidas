import type { ReactNode } from 'react';

/**
 * Ícones de linha do design system: grid 24, `currentColor`, sem fill.
 * Cada um herda a cor do texto ao redor e é decorativo (`aria-hidden`) — quem
 * precisar de rótulo o dá no elemento clicável que embrulha o ícone.
 */
interface IconProps {
  size?: number;
}

function Icon({ size = 20, children }: { size?: number; children: ReactNode }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.8}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      {children}
    </svg>
  );
}

export function IconHome({ size }: IconProps) {
  return (
    <Icon size={size}>
      <path d="M3 10.5 12 3l9 7.5" />
      <path d="M5.5 9.5V19a1 1 0 0 0 1 1H10v-5h4v5h3.5a1 1 0 0 0 1-1V9.5" />
    </Icon>
  );
}

// Consultas: calendário com um check dentro (agenda confirmada).
export function IconAppointments({ size }: IconProps) {
  return (
    <Icon size={size}>
      <path d="M8 3v3M16 3v3" />
      <rect x="4" y="5" width="16" height="16" rx="2" />
      <path d="M4 9.5h16" />
      <path d="M9 14.5l2 2 4-4" />
    </Icon>
  );
}

export function IconProfile({ size }: IconProps) {
  return (
    <Icon size={size}>
      <circle cx="12" cy="12" r="9" />
      <circle cx="12" cy="10" r="3" />
      <path d="M6.5 18.3a6 6 0 0 1 11 0" />
    </Icon>
  );
}

export function IconCalendar({ size }: IconProps) {
  return (
    <Icon size={size}>
      <path d="M8 3v3M16 3v3" />
      <rect x="4" y="5" width="16" height="16" rx="2" />
      <path d="M4 9.5h16" />
    </Icon>
  );
}

export function IconCaretRight({ size }: IconProps) {
  return (
    <Icon size={size}>
      <path d="M9 6l6 6-6 6" />
    </Icon>
  );
}

export function IconCheck({ size }: IconProps) {
  return (
    <Icon size={size}>
      <path d="M5 13l4 4L19 7" />
    </Icon>
  );
}

export function IconClock({ size }: IconProps) {
  return (
    <Icon size={size}>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 7v5l3 2" />
    </Icon>
  );
}

export function IconArrowRight({ size }: IconProps) {
  return (
    <Icon size={size}>
      <path d="M5 12h14M13 6l6 6-6 6" />
    </Icon>
  );
}

// Alvo do "Pedir ajuda": dois círculos concêntricos + 4 traços diagonais.
export function IconHelpTarget({ size }: IconProps) {
  return (
    <Icon size={size}>
      <circle cx="12" cy="12" r="9" />
      <circle cx="12" cy="12" r="3.5" />
      <path d="M6 6l3.5 3.5M18 6l-3.5 3.5M6 18l3.5-3.5M18 18l-3.5-3.5" />
    </Icon>
  );
}

export function IconLogout({ size }: IconProps) {
  return (
    <Icon size={size}>
      <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4M16 17l5-5-5-5M21 12H9" />
    </Icon>
  );
}

export function IconBack({ size }: IconProps) {
  return (
    <Icon size={size}>
      <path d="M15 19l-7-7 7-7" />
    </Icon>
  );
}

/**
 * Ícones *filled* (fill em `currentColor`, sem stroke no contorno externo):
 * exceção documentada do DS, usada só no estado ATIVO da tab bar mobile
 * (Etapa 1) — o par outline/filled sinaliza a aba selecionada sem depender
 * só de cor.
 *
 * Paths transcritos verbatim do handoff de design (`design_files/assets/
 * icons/{home,appointment,avatar}-active-icon.svg`) — viewBox nativo do
 * handoff (21×21 para os três), sem reescalar para o grid 24 dos ícones
 * outline. Mapeamento de cor: o preenchimento navy do handoff vira
 * `currentColor` (herdado do svg); o detalhe branco desenhado por cima
 * (linhas/pontos) vira o token `var(--color-white)` — nunca o hex `#fff` do
 * arquivo original.
 */
function IconFilled({ size = 20, children }: { size?: number; children: ReactNode }) {
  return (
    <svg width={size} height={size} viewBox="0 0 21 21" fill="currentColor" aria-hidden="true">
      {children}
    </svg>
  );
}

export function IconHomeFilled({ size }: IconProps) {
  return (
    <IconFilled size={size}>
      <path
        d="M1.5 10.5L10.5 1.5L19.5 10.5"
        stroke="currentColor"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <path
        d="M3.5 8.5V15.5C3.5 16.6046 4.39543 17.5 5.5 17.5H15.5C16.6046 17.5 17.5 16.6046 17.5 15.5V8.5"
        stroke="currentColor"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </IconFilled>
  );
}

export function IconAppointmentsFilled({ size }: IconProps) {
  return (
    <IconFilled size={size}>
      <path
        fillRule="evenodd"
        clipRule="evenodd"
        d="M4.5 2.5H16.5C17.6046 2.5 18.5 3.39543 18.5 4.5V16.5C18.5 17.6046 17.6046 18.5 16.5 18.5H4.5C3.39543 18.5 2.5 17.6046 2.5 16.5V4.5C2.5 3.39543 3.39543 2.5 4.5 2.5Z"
        stroke="var(--color-white)"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <path d="M2.5 6.5H18.5" stroke="var(--color-white)" strokeLinecap="round" strokeLinejoin="round" />
      <path
        d="M10.5 11.5C11.0523 11.5 11.5 11.0523 11.5 10.5C11.5 9.94772 11.0523 9.5 10.5 9.5C9.94772 9.5 9.5 9.94772 9.5 10.5C9.5 11.0523 9.94772 11.5 10.5 11.5Z"
        fill="var(--color-white)"
      />
      <path
        d="M6.5 11.5C7.05228 11.5 7.5 11.0523 7.5 10.5C7.5 9.94772 7.05228 9.5 6.5 9.5C5.94772 9.5 5.5 9.94772 5.5 10.5C5.5 11.0523 5.94772 11.5 6.5 11.5Z"
        fill="var(--color-white)"
      />
      <path
        d="M6.5 15.5C7.05228 15.5 7.5 15.0523 7.5 14.5C7.5 13.9477 7.05228 13.5 6.5 13.5C5.94772 13.5 5.5 13.9477 5.5 14.5C5.5 15.0523 5.94772 15.5 6.5 15.5Z"
        fill="var(--color-white)"
      />
    </IconFilled>
  );
}

export function IconProfileFilled({ size }: IconProps) {
  return (
    <IconFilled size={size}>
      <path
        d="M10.5 18.5C14.9183 18.5 18.5 14.9183 18.5 10.5C18.5 6.08172 14.9183 2.5 10.5 2.5C6.08172 2.5 2.5 6.08172 2.5 10.5C2.5 14.9183 6.08172 18.5 10.5 18.5Z"
        stroke="var(--color-white)"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <path
        d="M16.5 15.5002C15.8385 13.2267 13.3004 12.4751 10.5 12.4751C7.77251 12.4751 5.22927 13.3439 4.5 15.5002"
        stroke="var(--color-white)"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <path
        fillRule="evenodd"
        clipRule="evenodd"
        d="M10.5 4.5C12.1569 4.5 13.5 5.84315 13.5 7.5V9.5C13.5 11.1569 12.1569 12.5 10.5 12.5C8.84315 12.5 7.5 11.1569 7.5 9.5V7.5C7.5 5.84315 8.84315 4.5 10.5 4.5Z"
        stroke="var(--color-white)"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </IconFilled>
  );
}
