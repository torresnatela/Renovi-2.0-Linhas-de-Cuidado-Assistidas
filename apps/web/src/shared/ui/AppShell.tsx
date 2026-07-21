import type { ComponentType, ReactNode } from 'react';
import { Link, NavLink } from 'react-router-dom';

import logoBlue from '../../assets/logos/logo-blue.svg';
import { Avatar } from './Avatar';
import { IconAppointments, IconHome, IconProfile } from './icons';

const cx = (...c: Array<string | false | null | undefined>) => c.filter(Boolean).join(' ');

interface AppShellProps {
  userName: string;
  /** Slot da afordância "Pedir ajuda" — o shell não conhece a lógica clínica. */
  help: ReactNode;
  children: ReactNode;
}

/**
 * O chrome do app desktop (DESIGN-SYSTEM §6): header sticky de 70px + container
 * `max-w-shell`. É PRESENTACIONAL PURO — nenhum hook de dados aqui. Quem sabe o
 * nome e monta o "Pedir ajuda" é a AppLayout; o shell só posiciona.
 */
export function AppShell({ userName, help, children }: AppShellProps) {
  return (
    <div className="min-h-screen bg-page">
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

      <main className="mx-auto max-w-shell px-10 pb-16 pt-9">{children}</main>
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
