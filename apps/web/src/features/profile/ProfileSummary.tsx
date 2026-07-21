import { useNavigate } from 'react-router-dom';

import type { EnrollmentStatus, JourneyEnrollment } from '../../shared/api';
import { Avatar } from '../../shared/ui/Avatar';
import { Card } from '../../shared/ui/Card';
import { IconLogout } from '../../shared/ui/icons';
import { useLogout, useSession } from '../auth/useSession';
import { useJourney } from '../journey/useJourney';

const STATUS_LABEL: Record<EnrollmentStatus, string> = {
  ativa: 'Ativa',
  pausada: 'Pausada',
  concluida: 'Concluída',
  encerrada: 'Encerrada',
  expirada: 'Expirada',
};

const ANCORAS = [
  { href: '#dados', label: 'Dados pessoais' },
  { href: '#plano', label: 'Plano e cobertura' },
  { href: '#privacidade', label: 'Privacidade e segurança' },
];

/**
 * Resumo sticky do perfil (aside): identidade, o selo do plano e a navegação por
 * âncoras para as seções. O selo é verde SÓ quando há uma matrícula `ativa` —
 * senão é neutro e diz o estado real (nunca finge um plano ativo). As âncoras são
 * links `#`, e as seções compensam o header sticky com `scroll-mt-24`. A saída é
 * ação real de conta: `useLogout` limpa todo o cache derivado e leva ao /entrar.
 */
export function ProfileSummary() {
  const session = useSession();
  const journey = useJourney();
  const logout = useLogout();
  const navigate = useNavigate();

  const nome = session.data?.full_name ?? '';
  const email = session.data?.email ?? '';
  const enrollments = journey.data?.enrollments ?? [];

  return (
    <aside className="flex flex-col gap-5 lg:sticky lg:top-[102px]">
      <Card padding="lg" className="flex flex-col items-center gap-3 text-center">
        <Avatar name={nome} size="lg" />
        <div className="flex min-w-0 flex-col gap-0.5">
          <h2 className="truncate text-xl font-bold text-primary-300">{nome}</h2>
          <p className="truncate text-[13.5px] text-muted">{email}</p>
        </div>
        <PlanPill enrollments={enrollments} />
      </Card>

      <nav className="flex flex-col gap-0.5 rounded-lg border border-primary-100 bg-white p-2 shadow-card">
        {ANCORAS.map((a) => (
          <a
            key={a.href}
            href={a.href}
            className="flex items-center gap-3 rounded-md px-3.5 py-3 text-[14.5px] font-bold text-primary-300 transition hover:bg-page focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-300"
          >
            <span className="h-2 w-2 shrink-0 rounded-full bg-primary-200" aria-hidden="true" />
            {a.label}
          </a>
        ))}
      </nav>

      <button
        type="button"
        onClick={() => logout.mutate(undefined, { onSuccess: () => navigate('/entrar') })}
        disabled={logout.isPending}
        className="inline-flex items-center justify-center gap-2 rounded-md border border-primary-100 bg-white p-3.5 text-sm font-bold text-error transition hover:bg-page focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-300 disabled:cursor-not-allowed disabled:opacity-60"
      >
        <IconLogout size={17} />
        Sair da conta
      </button>
    </aside>
  );
}

function PlanPill({ enrollments }: { enrollments: JourneyEnrollment[] }) {
  const ativa = enrollments.find((e) => e.enrollment.status === 'ativa');

  if (ativa) {
    return (
      <span className="inline-flex items-center gap-[7px] rounded-pill bg-[rgba(41,176,29,0.12)] px-3 py-1.5 text-[12.5px] font-bold text-success">
        <span className="h-[7px] w-[7px] rounded-full bg-success" aria-hidden="true" />
        Plano ativo · {ativa.care_line_name}
      </span>
    );
  }

  const primeira = enrollments[0];
  const texto = primeira
    ? `${primeira.care_line_name} · ${STATUS_LABEL[primeira.enrollment.status]}`
    : 'Sem plano ativo';

  return (
    <span className="inline-flex items-center gap-[7px] rounded-pill bg-primary-100 px-3 py-1.5 text-[12.5px] font-bold text-primary-300">
      <span className="h-[7px] w-[7px] rounded-full bg-primary-200" aria-hidden="true" />
      {texto}
    </span>
  );
}
