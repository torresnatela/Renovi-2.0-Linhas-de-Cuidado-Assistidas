import { useMutation, useQuery, useQueryClient, type QueryClient } from '@tanstack/react-query';

import * as api from '../../shared/api';

// O fuso de leitura mudou-se para shared/datetime (é uma preocupação de
// data/hora, não da jornada). Re-exportado aqui para os imports antigos
// (JourneyPage, ui.tsx) continuarem valendo sem alteração.
export { FUSO_PADRAO } from '../../shared/datetime';

/**
 * Chaves hierárquicas, no padrão do schedulingKeys: a raiz 'journey' permite
 * invalidar tudo de uma vez, e os prefixos ('availability', 'appointments')
 * permitem invalidar por família sem casar a chave inteira.
 */
export const journeyKeys = {
  all: ['journey'] as const,
  tree: () => [...journeyKeys.all, 'tree'] as const,
  availability: (itemId: string) => [...journeyKeys.all, 'availability', itemId] as const,
  appointments: (status?: string) =>
    [...journeyKeys.all, 'appointments', 'list', status ?? 'todas'] as const,
  audit: () => [...journeyKeys.all, 'audit'] as const,
};

/**
 * A jornada muda com o que o paciente faz (agendar, cancelar) e com o relógio (um
 * item que destrava). 30s é curto o bastante para a tela não mostrar um veredito
 * velho e longo o bastante para não remontar a jornada a cada foco de janela.
 */
const JORNADA_STALE = 30 * 1000;

/** Horário livre é perecível — mesmo racional (e mesmo valor) do agendamento. */
const SLOTS_STALE = 30 * 1000;

export function useJourney() {
  return useQuery({
    queryKey: journeyKeys.tree(),
    queryFn: api.getJourney,
    staleTime: JORNADA_STALE,
  });
}

export function useAvailability(itemId: string | undefined, enabled = true) {
  return useQuery({
    queryKey: journeyKeys.availability(itemId ?? ''),
    queryFn: () => api.getAvailability(itemId!),
    enabled: enabled && Boolean(itemId),
    staleTime: SLOTS_STALE,
  });
}

export function useCareAppointments(status?: string) {
  return useQuery({
    queryKey: journeyKeys.appointments(status),
    queryFn: () => api.listCareAppointments(status),
  });
}

export function useAudit() {
  return useQuery({
    queryKey: journeyKeys.audit(),
    queryFn: () => api.getAudit(),
  });
}

/**
 * Agendar e cancelar mexem no MESMO trio: a jornada (vereditos), a disponibilidade
 * (o horário some/volta) e a lista de consultas. Invalidar os três juntos mantém
 * as telas coerentes sem cada mutation ter que lembrar de quê depende de quê.
 */
function invalidarJornada(qc: QueryClient) {
  qc.invalidateQueries({ queryKey: journeyKeys.tree() });
  qc.invalidateQueries({ queryKey: [...journeyKeys.all, 'availability'] });
  qc.invalidateQueries({ queryKey: [...journeyKeys.all, 'appointments'] });
}

export function useScheduleCare() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { body: { item_id: string; slot_id: string }; idemKey: string }) =>
      api.createCareAppointment(vars.body, vars.idemKey),
    // onSettled, e NÃO onSuccess: como no booking (ADR-016), no 502 o resultado é
    // DESCONHECIDO — a consulta pode existir. Invalidar sempre evita oferecer como
    // livre um horário que o próprio paciente talvez já tenha marcado, e a lista
    // sem uma consulta que talvez exista.
    onSettled: () => invalidarJornada(qc),
  });
}

export function useCancelCare() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.cancelCareAppointment(id),
    // Cancelar devolve a cota ao item: jornada, disponibilidade e lista mudam
    // juntas — as mesmas invalidações do agendamento.
    onSettled: () => invalidarJornada(qc),
  });
}
