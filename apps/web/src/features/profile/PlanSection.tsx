import type { JourneyEnrollment, JourneyItem } from '../../shared/api';
import { Badge } from '../../shared/ui/Badge';
import { Card } from '../../shared/ui/Card';
import { EligibilityNotice } from '../../shared/ui/EligibilityNotice';
import { Empty, ErrorNotice, Loading } from '../../shared/ui/feedback';
import { PlanValidityBanner } from '../../shared/ui/PlanValidityBanner';
import { useJourney } from '../journey/useJourney';

/**
 * Plano e cobertura — dados REAIS da jornada, sem inventar cotas nem contagem de
 * uso (a API não expõe "2 de 4 usadas"). Por matrícula: a faixa de vigência
 * (PlanValidityBanner) e, abaixo, o que a linha inclui — um mini-card por item
 * com o estado do motor: liberado (Badge) ou bloqueado (EligibilityNotice, que
 * já traz o motivo pronto do servidor). Quem decide é o servidor; aqui só exibe.
 *
 * `GET /me/journey` devolve TODO o histórico de matrículas (inclusive
 * encerrada/expirada/concluída). O Perfil, como a Jornada (ver `JourneyPage`,
 * commit 4449cb2), só mostra o PRESENTE: filtramos para `status === 'ativa'`.
 * Histórico de planos não é papel do Perfil v1 — sem isso, matrículas mortas
 * apareciam com banner completo e itens marcados "encerrada", um ruído sem
 * utilidade para o paciente.
 */
export function PlanSection() {
  const journey = useJourney();
  const ativas = journey.data?.enrollments.filter((e) => e.enrollment.status === 'ativa') ?? [];

  return (
    // Ver PersonalDataSection: o scroll-mt maior é só para compensar o header
    // sticky do desktop; no mobile não há sticky, então reduz.
    <section id="plano" className="scroll-mt-6 lg:scroll-mt-24">
      <Card padding="lg">
        <h2 className="text-lg font-bold text-primary-300">Plano e cobertura</h2>

        <div className="mt-5">
          {journey.isLoading && <Loading label="Carregando seu plano…" />}
          {journey.isError && (
            <ErrorNotice error={journey.error} retry={() => journey.refetch()} />
          )}
          {journey.data && ativas.length === 0 && (
            <Empty
              title="Nenhum plano ativo no momento."
              hint="Assim que uma linha de cuidado for atribuída à sua conta, ela aparece aqui."
            />
          )}
          {journey.data && ativas.length > 0 && (
            <div className="flex flex-col gap-6">
              {ativas.map((e) => (
                <EnrollmentBlock key={e.enrollment.id} enrollment={e} />
              ))}
            </div>
          )}
        </div>
      </Card>
    </section>
  );
}

function EnrollmentBlock({ enrollment }: { enrollment: JourneyEnrollment }) {
  return (
    <div className="flex flex-col gap-4">
      <PlanValidityBanner
        enrollment={enrollment.enrollment}
        careLineName={enrollment.care_line_name}
      />

      <div className="flex flex-col gap-3">
        <span className="text-xs font-bold uppercase tracking-[0.06em] text-muted">
          Sua linha inclui
        </span>
        <div className="grid gap-3 [grid-template-columns:repeat(auto-fit,minmax(200px,1fr))]">
          {enrollment.items.map((item) => (
            <PlanItemCard key={item.item.id} item={item} />
          ))}
        </div>
      </div>
    </div>
  );
}

function PlanItemCard({ item }: { item: JourneyItem }) {
  const { allowed, blocks } = item.eligibility;
  return (
    <div className="flex flex-col gap-2.5 rounded-md border border-primary-100 p-4">
      <div className="flex flex-col gap-0.5">
        <span className="text-[14.5px] font-bold text-primary-300">{item.item.label}</span>
        {item.item.recurrence && (
          <span className="text-[12.5px] text-muted">{item.item.recurrence}</span>
        )}
      </div>
      {allowed ? (
        <span className="self-start">
          <Badge tone="accent">Disponível</Badge>
        </span>
      ) : (
        <EligibilityNotice blocks={blocks} compact />
      )}
    </div>
  );
}
