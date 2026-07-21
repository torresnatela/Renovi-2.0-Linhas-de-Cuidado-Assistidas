import { useNavigate } from 'react-router-dom';

import { Avatar } from '../../shared/ui/Avatar';
import { Button } from '../../shared/ui/Button';
import { Card } from '../../shared/ui/Card';
import { useLogout, useSession } from '../auth/useSession';

/**
 * Stub honesto do Perfil: identidade (avatar + nome + e-mail) e a saída. A Etapa 7
 * expande (dados do plano, notificações, LGPD). O logout limpa TODO o cache
 * derivado da conta (ver `useLogout`) e leva ao /entrar.
 */
export function ProfilePage() {
  const session = useSession();
  const logout = useLogout();
  const navigate = useNavigate();
  const conta = session.data;

  return (
    <Card as="section" padding="lg" className="mx-auto max-w-xl">
      <div className="flex items-center gap-5">
        <Avatar name={conta?.full_name ?? ''} size="lg" />
        <div className="min-w-0">
          <h1 className="truncate text-xl font-bold text-primary-300">{conta?.full_name}</h1>
          <p className="truncate text-sm text-muted">{conta?.email}</p>
        </div>
      </div>

      <Button
        color="ghost"
        className="mt-8"
        loading={logout.isPending}
        onClick={() => logout.mutate(undefined, { onSuccess: () => navigate('/entrar') })}
      >
        Sair
      </Button>
    </Card>
  );
}
