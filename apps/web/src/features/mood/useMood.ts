import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import * as api from '../../shared/api';

export const moodKeys = {
  all: ['mood'] as const,
  today: () => [...moodKeys.all, 'today'] as const,
  instrument: (codigo: string) => [...moodKeys.all, 'instrument', codigo] as const,
  availability: (codigo: string) => [...moodKeys.all, 'availability', codigo] as const,
};

/** A config do instrumento muda raramente (reference data versionada). */
const INSTRUMENT_STALE = 10 * 60 * 1000;

/** O dia do paciente: consentimento, elegibilidade e o check-in de hoje. */
export function useMoodToday() {
  return useQuery({
    queryKey: moodKeys.today(),
    queryFn: api.getMoodToday,
  });
}

/** Config do instrumento (dimensões, rótulos, tags) para desenhar a grade. */
export function useMoodInstrument(codigo: string) {
  return useQuery({
    queryKey: moodKeys.instrument(codigo),
    queryFn: () => api.getMoodInstrument(codigo),
    staleTime: INSTRUMENT_STALE,
  });
}

/** Concede o consentimento e revalida o dia (que passa a permitir o check-in). */
export function useGrantConsent() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (versaoTermo: string) => api.grantConsent(versaoTermo),
    onSuccess: () => qc.invalidateQueries({ queryKey: moodKeys.today() }),
  });
}

/** Registra o check-in do dia e atualiza `today` com o resultado. */
export function useRecordCheckin() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: api.recordMoodCheckin,
    onSuccess: () => qc.invalidateQueries({ queryKey: moodKeys.today() }),
  });
}

/** Disponibilidade de um instrumento periódico (cadência avaliada pelo motor). */
export function useAssessmentAvailability(codigo: string | null) {
  return useQuery({
    queryKey: moodKeys.availability(codigo ?? ''),
    queryFn: () => api.getAssessmentAvailability(codigo!),
    enabled: Boolean(codigo),
  });
}

/** Submete um instrumento periódico e revalida o dia (o gatilho reage). */
export function useSubmitAssessment() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ codigo, items }: { codigo: api.AssessmentCode; items: number[] }) =>
      api.submitAssessment(codigo, items),
    onSuccess: () => qc.invalidateQueries({ queryKey: moodKeys.today() }),
  });
}

/** Aciona a afordância "preciso de ajuda agora" (roteia ao canal de urgência). */
export function useHelpNow() {
  return useMutation({ mutationFn: api.moodHelpNow });
}
