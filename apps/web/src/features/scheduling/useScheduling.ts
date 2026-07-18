import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import * as api from '../../shared/api';

export const schedulingKeys = {
  all: ['scheduling'] as const,
  specialties: () => [...schedulingKeys.all, 'specialties'] as const,
  professionals: (specialtyId: string) =>
    [...schedulingKeys.all, 'professionals', specialtyId] as const,
  slots: (professionalId: string) => [...schedulingKeys.all, 'slots', professionalId] as const,
  /**
   * 'list' e 'detail' separados de propósito: sem isso, invalidar a lista
   * derrubaria o detalhe que o onSuccess acabou de escrever, e a tela pós-
   * agendamento piscaria em "carregando" logo depois de o paciente esperar 20s.
   */
  appointments: () => [...schedulingKeys.all, 'appointments', 'list'] as const,
  appointment: (id: string) => [...schedulingKeys.all, 'appointments', 'detail', id] as const,
};

/**
 * Catálogo do legado: muda pouco. O staleTime generoso é o que evita bater no
 * MySQL de terceiro a cada passo PARA TRÁS no wizard.
 */
const CATALOGO_STALE = 5 * 60 * 1000;

/**
 * Horário livre é perecível: alguém marca enquanto o paciente decide. 30s é curto
 * o bastante para não oferecer fantasma e longo o bastante para não consultar o
 * legado a cada foco de janela.
 */
const SLOTS_STALE = 30 * 1000;

export function useSpecialties() {
  return useQuery({
    queryKey: schedulingKeys.specialties(),
    queryFn: api.listSpecialties,
    staleTime: CATALOGO_STALE,
  });
}

export function useProfessionals(specialtyId: string | undefined) {
  return useQuery({
    queryKey: schedulingKeys.professionals(specialtyId ?? ''),
    queryFn: () => api.listProfessionals(specialtyId!),
    enabled: Boolean(specialtyId),
    staleTime: CATALOGO_STALE,
  });
}

export function useSlots(professionalId: string | undefined) {
  return useQuery({
    queryKey: schedulingKeys.slots(professionalId ?? ''),
    queryFn: () => api.listSlots(professionalId!),
    enabled: Boolean(professionalId),
    staleTime: SLOTS_STALE,
  });
}

export function useAppointments() {
  return useQuery({
    queryKey: schedulingKeys.appointments(),
    queryFn: api.listAppointments,
  });
}

export function useAppointment(id: string | undefined) {
  return useQuery({
    queryKey: schedulingKeys.appointment(id ?? ''),
    queryFn: () => api.getAppointment(id!),
    enabled: Boolean(id),
    /**
     * Em vez de perguntar de minuto em minuto, DORME até a hora que o servidor
     * disse que a janela abre e confere UMA vez.
     *
     * `opens_at` é dado, não regra: o front não sabe — nem precisa saber — que a
     * antecedência é de 30 minutos. O piso de 15s cobre relógio do cliente
     * adiantado: se ele acha que já passou de opens_at mas o servidor ainda diz
     * TOO_EARLY, isto vira um poll curto em vez de um laço de milissegundos.
     *
     * Aba em segundo plano não dispara (é o padrão do TanStack Query); quem cobre
     * a volta é o refetchOnWindowFocus, que também já é padrão.
     *
     * O TETO de 24h não é firula: `setTimeout` guarda o delay num int de 32 bits,
     * e qualquer valor acima de ~24,8 dias estoura e dispara NA HORA. A janela de
     * agendamento é de 30 dias, então uma consulta marcada para daqui a 25+ dias
     * daria um `faltam` gigante → refetch imediato → mesmo `opens_at` → refetch
     * imediato de novo: um laço apertado martelando o servidor por dias, o oposto
     * de "dormir". Com o teto, no pior caso ele acorda uma vez por dia e volta a
     * dormir.
     */
    refetchInterval: (query) => {
      const appt = query.state.data;
      if (!appt || appt.join.status !== 'TOO_EARLY') return false;
      return proximoPoll(appt.join.opens_at, Date.now());
    },
  });
}

/**
 * Quanto esperar até reconferir a janela de entrada, dado quando ela abre.
 *
 * PISO de 15s: cobre o relógio do cliente adiantado (ele acha que já passou de
 * opens_at, o servidor ainda diz TOO_EARLY) — vira um poll curto, não um laço de
 * milissegundos.
 *
 * TETO de 24h: `setTimeout` guarda o delay num int de 32 bits e estoura acima de
 * ~24,8 dias, disparando NA HORA. A janela de agendamento é de 30 dias, então uma
 * consulta a 25+ dias daria um delay gigante → refetch imediato → mesmo opens_at
 * → refetch imediato: um laço apertado por dias. Com o teto, no pior caso acorda
 * uma vez por dia.
 *
 * Exportada para ser testável — a lógica de overflow não pode viver escondida
 * dentro de um closure de useQuery.
 */
export function proximoPoll(opensAt: string, agora: number): number {
  const UM_DIA = 24 * 60 * 60 * 1000;
  const faltam = new Date(opensAt).getTime() - agora;
  return Math.min(Math.max(faltam + 1000, 15_000), UM_DIA);
}

export function useCreateAppointment() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: api.createAppointment,
    /**
     * onSettled, e NÃO onSuccess: no 502/504 o resultado é DESCONHECIDO — o
     * horário foi reservado e a consulta pode existir (ADR-016). Invalidar só no
     * sucesso deixaria a tela oferecendo como livre um horário que já é do
     * próprio paciente, e a lista sem a consulta que ele talvez tenha marcado.
     *
     * Invalida a LISTA e os HORÁRIOS, mas NÃO a subárvore de detalhe — senão
     * derrubaria o detalhe que o onSuccess acaba de semear e a tela pós-
     * agendamento piscaria em "carregando" logo depois de o paciente esperar 20s.
     * É por isso que schedulingKeys separa 'list' de 'detail'. (Antes isto
     * invalidava `all`, que casa com o detalhe e anulava a separação.)
     */
    onSettled: () => {
      qc.invalidateQueries({ queryKey: schedulingKeys.appointments() });
      qc.invalidateQueries({ queryKey: [...schedulingKeys.all, 'slots'] });
    },
    onSuccess: (appt) => qc.setQueryData(schedulingKeys.appointment(appt.id), appt),
  });
}

export function useJoinAppointment(appointmentId: string) {
  return useMutation({
    mutationFn: () => api.joinAppointment(appointmentId),
  });
}
