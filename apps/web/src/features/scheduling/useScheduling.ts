import { useMutation, useQuery } from '@tanstack/react-query';

import * as api from '../../shared/api';

/**
 * Após o redesign (Etapa 8) só sobrou o DETALHE da consulta: o wizard por
 * especialidade foi aposentado (agendar é por item da linha, em `journey/`) e a
 * lista virou a `ConsultationsPage`. Restam a leitura do detalhe (com o poll da
 * janela de entrada) e a entrada na sala.
 */
export const schedulingKeys = {
  all: ['scheduling'] as const,
  appointment: (id: string) => [...schedulingKeys.all, 'appointments', 'detail', id] as const,
};

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

export function useJoinAppointment(appointmentId: string) {
  return useMutation({
    mutationFn: () => api.joinAppointment(appointmentId),
  });
}
