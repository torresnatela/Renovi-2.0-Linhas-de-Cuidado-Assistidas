import type { ComponentType, ReactNode } from 'react';
import { NavLink } from 'react-router-dom';

import {
  IconAppointments,
  IconAppointmentsFilled,
  IconHome,
  IconHomeFilled,
  IconProfile,
  IconProfileFilled,
} from './icons';

const cx = (...c: Array<string | false | null | undefined>) => c.filter(Boolean).join(' ');

/**
 * Tab bar fixa das telas raiz no mobile (< lg) — mock `Jornada`. Presentacional
 * puro: três destinos, com o par outline→filled sinalizando a aba ativa sem
 * depender só de cor.
 *
 * z-index 30: mesma camada do header sticky do desktop. O popover do
 * `HelpNowMenu` fica ACIMA (z-40, definido lá) — não mexer nele aqui.
 */
export function TabBar() {
  return (
    <nav
      aria-label="Principal"
      className="fixed bottom-0 left-1/2 z-30 flex w-full max-w-[430px] -translate-x-1/2 justify-around border-t border-primary-100 bg-white pt-2.5 pb-[calc(18px+env(safe-area-inset-bottom))]"
    >
      <TabItem to="/jornada" icon={IconHome} activeIcon={IconHomeFilled}>
        Jornada
      </TabItem>
      <TabItem to="/consultas" icon={IconAppointments} activeIcon={IconAppointmentsFilled}>
        Consultas
      </TabItem>
      <TabItem to="/perfil" icon={IconProfile} activeIcon={IconProfileFilled}>
        Perfil
      </TabItem>
    </nav>
  );
}

/**
 * Uma aba. Ativa: ícone filled + navy bold. Inativa: ícone outline + cinza,
 * esmaecido (`opacity-55`, do mock). O `aria-current="page"` sai de graça do
 * `NavLink`; o render-prop `isActive` troca ícone e classes.
 */
function TabItem({
  to,
  icon: Icon,
  activeIcon: ActiveIcon,
  children,
}: {
  to: string;
  icon: ComponentType<{ size?: number }>;
  activeIcon: ComponentType<{ size?: number }>;
  children: ReactNode;
}) {
  return (
    <NavLink
      to={to}
      className={({ isActive }) =>
        cx(
          'flex min-w-16 flex-col items-center gap-[3px] rounded-sm text-[11px]',
          'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-300',
          isActive ? 'font-bold text-primary-300' : 'font-semibold text-muted opacity-55',
        )
      }
    >
      {({ isActive }) => (
        <>
          {isActive ? <ActiveIcon size={24} /> : <Icon size={24} />}
          {children}
        </>
      )}
    </NavLink>
  );
}
