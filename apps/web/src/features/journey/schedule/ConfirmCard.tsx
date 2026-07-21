import type { AnnotatedSlot, AppointmentProfessional } from '../../../shared/api';
import { formatDateLong, formatTime } from '../../../shared/datetime';
import { Button } from '../../../shared/ui/Button';
import { Card } from '../../../shared/ui/Card';

interface ConfirmCardProps {
  label: string;
  slot: AnnotatedSlot;
  profissional: AppointmentProfessional | null;
  loading: boolean;
  onConfirmar: () => void;
}

/**
 * O resumo do que vai ser marcado + o CTA. Datas SEMPRE no fuso do slot (nunca o
 * do browser). O botão é o único que dispara o POST — por isso ele trava (loading)
 * em voo, para não abrir um segundo agendamento concorrente.
 */
export function ConfirmCard({ label, slot, profissional, loading, onConfirmar }: ConfirmCardProps) {
  const dia = formatDateLong(slot.starts_at, slot.time_zone);
  const hora = formatTime(slot.starts_at, slot.time_zone);

  return (
    <Card padding="lg" className="flex flex-col gap-4">
      <div className="flex flex-col gap-1">
        <p className="font-bold text-primary-300 first-letter:uppercase">
          {label} · {dia} · {hora}
        </p>
        <p className="text-sm text-muted">
          com {profissional?.full_name ?? 'seu profissional'} · por vídeo · cancelamento gratuito até
          24h antes
        </p>
      </div>
      <Button color="accent" size="lg" fullWidth loading={loading} onClick={onConfirmar}>
        Confirmar consulta
      </Button>
    </Card>
  );
}
