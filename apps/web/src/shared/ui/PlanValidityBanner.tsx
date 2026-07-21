import type { Enrollment, EnrollmentStatus } from '../api';
import { formatDate, FUSO_PADRAO } from '../datetime';
import { Badge } from './Badge';
import { Card } from './Card';

/**
 * A faixa de vigência do plano. Dois papéis:
 *  - normal: nome da linha + status (Badge) + barra de progresso da vigência +
 *    "Vigente até {data}";
 *  - nearExpiry: aviso informativo accent de que a vigência se aproxima do fim.
 *
 * A barra é MATEMÁTICA DE EXIBIÇÃO (posição de hoje no intervalo, clamp 0–100) —
 * não é regra de negócio: quem decide se o plano vale é o servidor (o `status` e a
 * elegibilidade). E nearExpiry NÃO tem botão de renovar: renovação é operação
 * administrativa, sem endpoint de paciente. O aviso tranquiliza (consultas
 * marcadas seguem valendo), não converte.
 */

interface PlanValidityBannerProps {
  enrollment: Enrollment;
  careLineName: string;
  nearExpiry?: boolean;
}

const STATUS_LABEL: Record<EnrollmentStatus, string> = {
  ativa: 'Ativa',
  pausada: 'Pausada',
  concluida: 'Concluída',
  encerrada: 'Encerrada',
  expirada: 'Expirada',
};

// Posição de hoje entre início e fim, em %. Clamp porque a barra nunca deve
// vazar do trilho, mesmo com um relógio adiantado ou datas degeneradas.
function progressPct(from: string, until: string): number {
  const start = new Date(from).getTime();
  const end = new Date(until).getTime();
  if (!(end > start)) return 0;
  const raw = ((Date.now() - start) / (end - start)) * 100;
  return Math.min(100, Math.max(0, raw));
}

export function PlanValidityBanner({
  enrollment,
  careLineName,
  nearExpiry = false,
}: PlanValidityBannerProps) {
  if (nearExpiry) {
    return (
      <div className="flex flex-col gap-2.5 rounded-lg border border-accent-200 bg-accent-100 px-5 py-[18px]">
        <span className="self-start rounded-pill bg-white px-2.5 py-1 text-[10.5px] font-bold uppercase tracking-[0.06em] text-primary-300">
          Seu plano
        </span>
        <span className="text-[15px] font-bold text-primary-300">
          Seu plano vai até <strong>{formatDate(enrollment.valid_until, FUSO_PADRAO)}</strong>.
        </span>
        <span className="text-[13px] text-ink">Suas consultas marcadas não são afetadas.</span>
      </div>
    );
  }

  const isActive = enrollment.status === 'ativa';
  const pct = progressPct(enrollment.valid_from, enrollment.valid_until);

  return (
    <Card className="flex flex-col gap-2.5">
      <div className="flex items-center justify-between gap-2.5">
        <span className="text-[15px] font-bold text-primary-300">{careLineName}</span>
        {isActive ? (
          <Badge tone="success">Plano ativo</Badge>
        ) : (
          <Badge tone="neutral">{STATUS_LABEL[enrollment.status]}</Badge>
        )}
      </div>

      <div className="h-1.5 overflow-hidden rounded-pill bg-primary-100">
        <div
          data-testid="validity-progress"
          className="h-full rounded-pill bg-primary-300"
          style={{ width: `${pct}%` }}
        />
      </div>

      <span className="text-[12.5px] text-muted">
        Vigente até{' '}
        <strong className="text-primary-300">
          {formatDate(enrollment.valid_until, FUSO_PADRAO)}
        </strong>
      </span>
    </Card>
  );
}
