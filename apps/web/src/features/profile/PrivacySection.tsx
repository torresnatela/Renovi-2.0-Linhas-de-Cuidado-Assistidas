import { CHECKIN_FINALIDADE, type ConsentStatus } from '../../shared/api';
import { formatDate, FUSO_PADRAO } from '../../shared/datetime';
import { Card } from '../../shared/ui/Card';
import { ErrorNotice, Loading } from '../../shared/ui/feedback';
import { useConsent, useRevokeConsent } from '../mood/useMood';

/**
 * Privacidade e segurança — o compromisso de sigilo (LGPD) em texto, e o único
 * controle de dados que a API de fato oferece ao paciente: revogar o
 * consentimento do check-in de humor.
 *
 * NÃO há "baixar meus dados", "excluir conta" nem "alterar senha" — não existe
 * endpoint para isso, e um botão que não faz nada é pior que sua ausência. A
 * revogação passa por um confirm() que explica a consequência (o check-in fica
 * bloqueado até um novo aceite); depois dela, `useRevokeConsent` revalida o dia,
 * então o Verificador de Humor reflete o bloqueio na hora.
 */
export function PrivacySection() {
  const consent = useConsent();

  return (
    // Ver PersonalDataSection: o scroll-mt maior é só para compensar o header
    // sticky do desktop; no mobile não há sticky, então reduz.
    <section id="privacidade" className="scroll-mt-6 lg:scroll-mt-24">
      <Card padding="lg">
        <div className="flex flex-col gap-1">
          <h2 className="text-lg font-bold text-primary-300">Privacidade e segurança</h2>
          <p className="text-[13.5px] leading-5 text-muted">
            Seus dados de saúde são sigilosos. Sua empresa nunca vê informações individuais —
            apenas dados agregados e anônimos.
          </p>
        </div>

        <div className="mt-5">
          {consent.isLoading && <Loading label="Carregando consentimento…" />}
          {consent.isError && (
            <ErrorNotice error={consent.error} retry={() => consent.refetch()} />
          )}
          {consent.data && <ConsentRow status={consent.data} />}
        </div>
      </Card>
    </section>
  );
}

function ConsentRow({ status }: { status: ConsentStatus }) {
  const revoke = useRevokeConsent();

  function handleRevoke() {
    const ok = window.confirm(
      'Revogar o consentimento vai bloquear o check-in de humor até você aceitar o termo ' +
        'novamente. Deseja continuar?',
    );
    if (ok) revoke.mutate(CHECKIN_FINALIDADE);
  }

  const detalhe = status.active
    ? status.concedido_em
      ? `Consentimento ativo · aceito em ${formatDate(status.concedido_em, FUSO_PADRAO)}`
      : 'Consentimento ativo'
    : 'Consentimento revogado · o check-in fica bloqueado até um novo aceite.';

  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center gap-3.5 rounded-md border border-primary-100 p-4">
        <span
          aria-hidden="true"
          className={`h-2 w-2 shrink-0 rounded-full ${status.active ? 'bg-success' : 'bg-primary-200'}`}
        />
        <div className="flex min-w-0 flex-1 flex-col gap-0.5">
          <span className="text-[15px] font-bold text-primary-300">Check-in de humor</span>
          <span className="text-[13px] text-muted">{detalhe}</span>
        </div>
        {status.active && (
          <button
            type="button"
            onClick={handleRevoke}
            disabled={revoke.isPending}
            className="shrink-0 rounded-sm px-2 py-1 text-[13px] font-bold text-error transition hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-300 disabled:cursor-not-allowed disabled:opacity-60"
          >
            {revoke.isPending ? 'Revogando…' : 'Revogar consentimento do check-in'}
          </button>
        )}
      </div>

      {/* Ação LGPD não pode falhar em silêncio: se a revogação erra, o paciente
          precisa ver que NÃO saiu do registro (senão acha que revogou sem ter). */}
      {revoke.isError && (
        <p role="alert" className="rounded-md bg-[rgba(205,25,25,0.08)] p-3 text-[13px] text-error">
          Não foi possível revogar agora. Tente de novo em instantes.
        </p>
      )}
    </div>
  );
}
