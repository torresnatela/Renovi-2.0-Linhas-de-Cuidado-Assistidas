import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import * as api from '../../shared/api';

export const sessionKey = ['session'] as const;

/**
 * useSession diz quem está logado.
 *
 * A sessão vive num cookie httpOnly, invisível ao JS — então a única forma de
 * saber se há sessão é perguntar ao servidor. Um 401 aqui é resposta válida
 * ("ninguém logado"), não erro: por isso `retry: false` e o null no catch.
 */
export function useSession() {
  return useQuery({
    queryKey: sessionKey,
    queryFn: async () => {
      try {
        return await api.getMe();
      } catch (err) {
        if (err instanceof api.ApiError && err.status === 401) return null;
        throw err;
      }
    },
    retry: false,
    staleTime: 5 * 60 * 1000,
  });
}

export function useLogin() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ cpf, password }: { cpf: string; password: string }) => api.login(cpf, password),
    onSuccess: (account) => qc.setQueryData(sessionKey, account),
  });
}

export function useRegister() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: api.RegisterRequest) => api.registerAccount(body),
    // O cadastro já abre a sessão no servidor: aproveitamos a conta devolvida em
    // vez de gastar um GET /me.
    onSuccess: (account) => qc.setQueryData(sessionKey, account),
  });
}

export function useLogout() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: api.logout,
    // clear() e não setQueryData(null): no logout, todo cache derivado da conta
    // (jornada, elegibilidade, consultas) tem que sumir junto — senão o próximo
    // usuário no mesmo browser veria dados de saúde do anterior.
    onSuccess: () => qc.clear(),
  });
}
