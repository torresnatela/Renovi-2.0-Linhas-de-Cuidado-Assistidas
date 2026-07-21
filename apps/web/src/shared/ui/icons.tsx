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
 * Ícones *filled* (fill em `currentColor`, sem stroke): exceção documentada
 * do DS, usada só no estado ATIVO da tab bar mobile (Etapa 1) — o par
 * outline/filled sinaliza a aba selecionada sem depender só de cor.
 */
function IconFilled({ size = 20, children }: { size?: number; children: ReactNode }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="currentColor"
      aria-hidden="true"
    >
      {children}
    </svg>
  );
}

// Mesma silhueta de IconHome (roof + walls), mas sólida: o triângulo do
// telhado e o corpo da casa preenchidos em vez de contornados.
export function IconHomeFilled({ size }: IconProps) {
  return (
    <IconFilled size={size}>
      <path d="M3 10.5 12 3l9 7.5Z" />
      <path d="M5.5 9.5V19a1 1 0 0 0 1 1H10v-5h4v5h3.5a1 1 0 0 0 1-1V9.5Z" />
    </IconFilled>
  );
}

// Corpo do calendário sólido + os dois "aneis" do topo (mesma composição de
// IconAppointments, sem a grade/check interna — o preenchimento já basta
// para distinguir do outline ao lado dele na tab bar).
export function IconAppointmentsFilled({ size }: IconProps) {
  return (
    <IconFilled size={size}>
      <rect x="4" y="6" width="16" height="15" rx="2" />
      <rect x="7" y="3" width="1.8" height="4.5" rx="0.9" />
      <rect x="15.2" y="3" width="1.8" height="4.5" rx="0.9" />
    </IconFilled>
  );
}

// Anel (mesmo raio 9 de IconProfile, via evenodd círculo-menos-círculo) +
// cabeça e ombros sólidos dentro — mesma leitura de "perfil", só que cheia.
export function IconProfileFilled({ size }: IconProps) {
  return (
    <IconFilled size={size}>
      <path
        fillRule="evenodd"
        clipRule="evenodd"
        d="M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18Zm0-1.6a7.4 7.4 0 1 1 0-14.8 7.4 7.4 0 0 1 0 14.8Z"
      />
      <circle cx="12" cy="10" r="3" />
      <path d="M6.5 18.3a6 6 0 0 1 11 0Z" />
    </IconFilled>
  );
}
