import type { ComponentType, ReactNode } from 'react';
import { Link, NavLink } from 'react-router-dom';

import { useIsDesktop } from '../viewport';
import logoBlue from '../../assets/logos/logo-blue.svg';
import { Avatar } from './Avatar';
import { IconAppointments, IconHome, IconProfile } from './icons';
import { TabBar } from './TabBar';

const cx = (...c: Array<string | false | null | undefined>) => c.filter(Boolean).join(' ');

interface AppShellProps {
  userName: string;
  /** Slot da afordância "Pedir ajuda" — o shell não conhece a lógica clínica. */
  help: ReactNode;
  children: ReactNode;
  /**
   * Chrome mobile (< lg): `tabs` = tela raiz (faixa de logo rolável + TabBar
   * fixa); `flow` = fluxo empilhado (a página traz seu FlowHeader; sem logo, sem
   * TabBar). Ignorado no desktop. ADR-041.
   */
  mobileVariant?: 'tabs' | 'flow';
}

/**
 * O chrome do app (ADR-041). No DESKTOP (≥ lg): header sticky de 70px + container
 * `max-w-shell` (DESIGN-SYSTEM §6). No MOBILE (< lg): sem header sticky — faixa de
 * logo rolável + TabBar (`tabs`) ou fluxo empilhado sem tab bar (`flow`). É
 * PRESENTACIONAL PURO — nenhum hook de dados aqui. Quem sabe o nome e monta o
 * "Pedir ajuda" é a AppLayout; o shell só posiciona.
 */
export function AppShell({ userName, help, children, mobileVariant = 'tabs' }: AppShellProps) {
  const isDesktop = useIsDesktop();

  if (!isDesktop) {
    // No mobile o header desktop some. `userName`/`help` NÃO são renderizados: o
    // HelpPill mobile vive no header de cada página e o Perfil é acessado pela aba.
    return (
      <div className="min-h-screen bg-page">
        {/* Mesmo skip-link do desktop: primeiro foco tabulável, salta ao <main>. */}
        <a
          href="#conteudo"
          className="sr-only focus:not-sr-only focus:absolute focus:left-4 focus:top-4 focus:z-50 focus:rounded-md focus:bg-primary-300 focus:px-4 focus:py-2 focus:text-sm focus:font-bold focus:text-white focus:shadow-raised focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-300 focus-visible:ring-offset-2"
        >
          Pular para o conteúdo
        </a>

        <main
          id="conteudo"
          className={cx(
            'mx-auto w-full max-w-[430px] px-5 pt-5',
            // `pb-[110px]` é o clearance da TabBar (mock); no fluxo, só respiro.
            mobileVariant === 'flow' ? 'pb-10' : 'pb-[110px]',
          )}
        >
          {mobileVariant === 'tabs' && (
            <Link
              to="/jornada"
              className="mb-5 inline-block rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-300"
            >
              <img src={logoBlue} alt="Renovi Saúde" className="h-6 w-auto" />
            </Link>
          )}
          {children}
        </main>

        {mobileVariant === 'tabs' && <TabBar />}
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-page">
      {/* Primeiro foco tabulável da página: salta a navegação repetida do topo e
          leva direto ao <main>. Visually-hidden até receber foco pelo teclado. */}
      <a
        href="#conteudo"
        className="sr-only focus:not-sr-only focus:absolute focus:left-4 focus:top-4 focus:z-50 focus:rounded-md focus:bg-primary-300 focus:px-4 focus:py-2 focus:text-sm focus:font-bold focus:text-white focus:shadow-raised focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-300 focus-visible:ring-offset-2"
      >
        Pular para o conteúdo
      </a>
      <header className="sticky top-0 z-30 border-b border-primary-100 bg-white">
        <div className="mx-auto flex h-[70px] max-w-shell items-center gap-9 px-10">
          <Link
            to="/jornada"
            className="shrink-0 rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-300"
          >
            <img src={logoBlue} alt="Renovi Saúde" className="h-[26px] w-auto" />
          </Link>

          <nav className="flex items-center gap-1">
            <NavItem to="/jornada" icon={IconHome}>
              Jornada
            </NavItem>
            <NavItem to="/consultas" icon={IconAppointments}>
              Consultas
            </NavItem>
            <NavItem to="/perfil" icon={IconProfile}>
              Perfil
            </NavItem>
          </nav>

          <div className="ml-auto flex items-center gap-[18px]">
            {help}
            <Link
              to="/perfil"
              aria-label="Perfil"
              className="shrink-0 rounded-full focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-300"
            >
              <Avatar name={userName} />
            </Link>
          </div>
        </div>
      </header>

      <main id="conteudo" className="mx-auto max-w-shell px-10 pb-16 pt-9">
        {children}
      </main>
    </div>
  );
}

/**
 * Pill de navegação. Ativa: fundo `primary-100`, texto navy bold — o ícone herda
 * a cor navy via `currentColor`. Inativa: cinza, hover em `bg-page`. O
 * `aria-current="page"` sai de graça do `NavLink` quando a rota casa.
 */
function NavItem({
  to,
  icon: Icon,
  children,
}: {
  to: string;
  icon: ComponentType<{ size?: number }>;
  children: ReactNode;
}) {
  return (
    <NavLink
      to={to}
      className={({ isActive }) =>
        cx(
          'inline-flex items-center gap-2 rounded-pill px-4 py-[9px] text-sm transition',
          'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-300',
          isActive
            ? 'bg-primary-100 font-bold text-primary-300'
            : 'font-semibold text-muted hover:bg-page',
        )
      }
    >
      <Icon size={20} />
      {children}
    </NavLink>
  );
}
