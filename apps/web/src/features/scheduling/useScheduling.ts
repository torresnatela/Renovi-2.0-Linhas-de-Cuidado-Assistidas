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
     */
    refetchInterval: (query) => {
      const appt = query.state.data;
      if (!appt || appt.join.status !== 'TOO_EARLY') return false;
      const faltam = new Date(appt.join.opens_at).getTime() - Date.now();
      return Math.max(faltam + 1000, 15_000);
    },
  });
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
     */
    onSettled: () => {
      qc.invalidateQueries({ queryKey: schedulingKeys.appointments() });
      qc.invalidateQueries({ queryKey: schedulingKeys.all });
    },
    onSuccess: (appt) => qc.setQueryData(schedulingKeys.appointment(appt.id), appt),
  });
}

export function useJoinAppointment(appointmentId: string) {
  return useMutation({
    mutationFn: () => api.joinAppointment(appointmentId),
  });
}
