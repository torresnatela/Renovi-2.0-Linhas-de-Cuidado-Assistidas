import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import * as api from '../../shared/api';
import { sessionKey } from '../auth/useSession';

/**
 * useOnboardingInfo busca o pré-preenchimento do convite. `retry: false`: um token
 * inválido/expirado é resposta legítima (não erro transitório) e não deve ser
 * reperguntado — a página mostra o motivo e manda pedir um novo link.
 */
export function useOnboardingInfo(token: string) {
  return useQuery({
    queryKey: ['onboarding', token],
    queryFn: () => api.getOnboardingInfo(token),
    retry: false,
    staleTime: Infinity,
  });
}

export function useCompleteOnboarding(token: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: api.RegisterRequest) => api.completeOnboarding(token, body),
    // A conclusão já abre a sessão no servidor: aproveitamos a conta devolvida em
    // vez de gastar um GET /me (mesmo padrão do useRegister).
    onSuccess: (account) => qc.setQueryData(sessionKey, account),
  });
}

export function useDeclineOnboarding(token: string) {
  return useMutation({
    mutationFn: () => api.declineOnboarding(token),
  });
}
