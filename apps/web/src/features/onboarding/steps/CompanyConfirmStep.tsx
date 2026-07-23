import { useState } from 'react';

import { Button } from '../../../shared/ui/Button';

interface Props {
  /** Nomes das empresas do convite (normalmente uma). */
  companies: string[];
  /** SIM: faz parte da empresa → conclui o cadastro. */
  onConfirm: () => void;
  /** NÃO (após a dupla confirmação): registra a recusa. */
  onDecline: () => void;
  /** Conclusão em voo (fala com a DAV, é lenta). */
  pending: boolean;
  /** Recusa em voo. */
  declining: boolean;
  apiError?: string | null;
}

function companyPhrase(companies: string[]): string {
  if (companies.length === 0) return 'sua empresa';
  if (companies.length === 1) return companies[0];
  if (companies.length === 2) return `${companies[0]} e ${companies[1]}`;
  return `${companies.slice(0, -1).join(', ')} e ${companies[companies.length - 1]}`;
}

/**
 * Passo final do onboarding: "Você faz parte da empresa X?". O SIM é o caminho fácil
 * (botão grande, primário). O NÃO exige DUPLA confirmação — um clique só revela o
 * aviso; o segundo, explícito, é que registra a recusa — para ninguém recusar sem
 * querer e perder o convite.
 */
export function CompanyConfirmStep({
  companies,
  onConfirm,
  onDecline,
  pending,
  declining,
  apiError,
}: Props) {
  const [guardOpen, setGuardOpen] = useState(false);
  const empresas = companyPhrase(companies);
  const plural = companies.length > 1;

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-2">
        <h1 className="text-[22px] font-bold text-primary-300">
          Você faz parte {plural ? 'das empresas' : 'da empresa'}{' '}
          <span className="text-accent-300">{empresas}</span>?
        </h1>
        <p className="text-[14px] leading-[21px] text-muted">
          {plural ? 'Essas empresas convidaram' : 'Essa empresa convidou'} você para o Renovi.
          Confirme para concluir seu cadastro e liberar seus benefícios.
        </p>
      </div>

      {/* A espera é longa e real (conclusão síncrona contra a Doutor ao Vivo).
          Sem aviso, o usuário acha que travou e recarrega no meio. */}
      {pending && (
        <p role="status" className="rounded-md bg-primary-100 p-3.5 text-sm text-primary-300">
          Estamos criando seu cadastro de saúde na Doutor ao Vivo. Isso pode levar até um minuto —
          <strong> não feche nem recarregue esta página.</strong>
        </p>
      )}

      {apiError && (
        <p role="alert" className="rounded-md bg-[rgba(205,25,25,0.08)] p-3.5 text-sm text-error">
          {apiError}
        </p>
      )}

      <Button color="accent" size="lg" fullWidth loading={pending} onClick={onConfirm}>
        Sim, faço parte
      </Button>

      {!guardOpen ? (
        <Button
          color="ghost"
          size="md"
          fullWidth
          disabled={pending}
          onClick={() => setGuardOpen(true)}
        >
          Não faço parte dessa empresa
        </Button>
      ) : (
        <div className="flex flex-col gap-3 rounded-md bg-[rgba(205,25,25,0.06)] p-4">
          <p className="text-[13px] leading-[19px] text-ink">
            Se você <strong>não</strong> faz parte {plural ? 'dessas empresas' : 'dessa empresa'},
            não vamos concluir esse vínculo. Tem certeza? Se foi um engano, é só voltar.
          </p>
          <div className="flex gap-3">
            <Button
              color="ghost"
              size="md"
              fullWidth
              disabled={declining}
              onClick={() => setGuardOpen(false)}
            >
              Voltar
            </Button>
            <Button color="primary" size="md" fullWidth loading={declining} onClick={onDecline}>
              Confirmar que não
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
