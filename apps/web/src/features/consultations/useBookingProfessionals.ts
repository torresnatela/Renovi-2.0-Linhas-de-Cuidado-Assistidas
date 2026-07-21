import { useQueries } from '@tanstack/react-query';

import * as api from '../../shared/api';
import { schedulingKeys } from '../scheduling/useScheduling';

/**
 * Nome do profissional para os cards de "Minhas Consultas".
 *
 * `CareAppointment` (a consulta na visão da jornada) NÃO guarda o profissional —
 * só o BOOKING guarda (`Appointment.professional.full_name`, via
 * `api.getAppointment(booking_id)`). Enriquecer aqui é client-side; o contrato
 * (`shared/api.ts`) não muda.
 *
 * Reusa a MESMA queryKey/queryFn de `useAppointment`
 * (`schedulingKeys.appointment` + `api.getAppointment`) para o cache ser
 * COMPARTILHADO com a página de detalhe: ao navegar para
 * `/consultas/{booking_id}` o TanStack Query já tem o dado — zero fetch
 * duplicado.
 *
 * `staleTime` generoso: o nome do profissional de uma consulta já marcada não
 * muda no meio da sessão.
 *
 * É ENHANCEMENT, não dado crítico — por isso:
 * - recebe só os `bookingId`s da aba VISÍVEL (quem chama decide a lista; não
 *   dispara a página inteira de histórico enquanto o paciente está em
 *   Próximas);
 * - enquanto carrega, ou se a consulta ao booking falhar, a chave correspondente
 *   fica ausente do map (nunca lança, nunca aparece como erro) — o card cai de
 *   volta para mostrar só o label.
 */
export function useBookingProfessionals(
  bookingIds: string[],
): Record<string, string | undefined> {
  const unicos = Array.from(new Set(bookingIds));

  const results = useQueries({
    queries: unicos.map((id) => ({
      queryKey: schedulingKeys.appointment(id),
      queryFn: () => api.getAppointment(id),
      staleTime: 30 * 60 * 1000,
    })),
  });

  const porBooking: Record<string, string | undefined> = {};
  unicos.forEach((id, i) => {
    porBooking[id] = results[i]?.data?.professional.full_name;
  });
  return porBooking;
}
