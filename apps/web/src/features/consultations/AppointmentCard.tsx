import { Link } from 'react-router-dom';

import type { CareAppointment, CareAppointmentStatus } from '../../shared/api';
import { dayKey, formatDateTimeShort, formatTime } from '../../shared/datetime';
import { Badge } from '../../shared/ui/Badge';
import { DateBadge } from '../../shared/ui/DateBadge';
import { LinkButton, tituloConsulta } from './parts';

// Só destes dois estados dá para cancelar — os mesmos da tela antiga
// (`CareAppointmentsPage`): o servidor confirma com 409, mas o botão nem aparece
// para não prometer o que não há. `em_andamento` não se cancela.
const CANCELAVEIS: ReadonlySet<CareAppointmentStatus> = new Set(['agendada', 'confirmada']);

/**
 * Um card da aba "Próximas".
 *
 * O selo "Hoje" e o botão primário de detalhe são a diferença do card do dia:
 * `dayKey` compara o dia da consulta com o de `now` NO fuso da agenda (nunca no
 * do browser — um runner em UTC diria "hoje" no dia errado). Por isso `now` chega
 * de fora: a tela injeta `new Date()` em produção e uma data fixa no teste.
 *
 * O card NÃO entra na sala: o join mora no detalhe (`/consultas/{booking_id}`),
 * onde o gate de pré-consulta e a janela do servidor decidem. Aqui só se VÊ.
 */
export function AppointmentCard({
  consulta,
  now,
  onCancel,
  cancelando,
  nomeProfissional,
}: {
  consulta: CareAppointment;
  now: Date;
  onCancel: (consulta: CareAppointment) => void;
  cancelando: boolean;
  /** "Dra. Marina Costa" — enriquecimento client-side de `useBookingProfessionals`; `undefined` enquanto carrega/falha. */
  nomeProfissional?: string;
}) {
  const tz = consulta.time_zone;
  const hoje = dayKey(consulta.scheduled_at, tz) === dayKey(now.toISOString(), tz);
  const podeCancelar = CANCELAVEIS.has(consulta.status);
  const detalhe = `/consultas/${consulta.booking_id}`;

  // "hoje · 16:00 · por vídeo" | "20/07 às 09:00 · por vídeo" — sempre via
  // shared/datetime, no fuso da agenda.
  const caption = hoje
    ? `hoje · ${formatTime(consulta.scheduled_at, tz)} · por vídeo`
    : `${formatDateTimeShort(consulta.scheduled_at, tz)} · por vídeo`;

  return (
    <div
      className={`flex flex-col gap-3.5 rounded-lg border bg-white p-[18px] shadow-card ${
        hoje ? 'border-primary-200' : 'border-primary-100'
      }`}
    >
      <div className="flex items-center gap-3">
        <DateBadge iso={consulta.scheduled_at} timeZone={tz} />
        <div className="flex min-w-0 flex-1 flex-col gap-0.5">
          <span className="font-bold text-primary-300">
            {tituloConsulta(consulta.label, nomeProfissional)}
          </span>
          <span className="text-[13px] text-muted">{caption}</span>
        </div>
        {hoje && <Badge tone="success">Hoje</Badge>}
      </div>

      {hoje ? (
        <LinkButton to={detalhe}>Ver consulta</LinkButton>
      ) : (
        <Link to={detalhe} className="text-[13px] font-bold text-primary-300 underline">
          Ver consulta
        </Link>
      )}

      {podeCancelar && (
        <div className="flex items-center justify-between gap-3 border-t border-primary-100 pt-3">
          <span className="text-xs leading-[16px] text-muted">
            Cancelamentos com mais de 24h de antecedência não contam na sua cota.
          </span>
          <button
            type="button"
            onClick={() => onCancel(consulta)}
            disabled={cancelando}
            className="shrink-0 whitespace-nowrap text-[13px] font-bold text-primary-300 underline disabled:opacity-60"
          >
            {cancelando ? 'Cancelando…' : 'Cancelar'}
          </button>
        </div>
      )}
    </div>
  );
}
