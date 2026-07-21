import { Button } from '../../../shared/ui/Button';
import { Input } from '../../../shared/ui/Input';
import type { RegisterForm } from '../RegisterPage';

interface Props {
  form: RegisterForm;
  onChange: (patch: Partial<RegisterForm>) => void;
  consent: boolean;
  onToggleConsent: () => void;
  onSubmit: () => void;
  pending: boolean;
  /** Motivo do bloqueio do CTA (null = liberado). Sempre visível ao lado do botão. */
  blockReason: string | null;
  apiError?: string | null;
}

/** Passo 3 do cadastro: acesso + consentimento LGPD, e o disparo do cadastro. */
export function AccessConsentStep({
  form,
  onChange,
  consent,
  onToggleConsent,
  onSubmit,
  pending,
  blockReason,
  apiError,
}: Props) {
  const senhasConflitam = form.senha2.length > 0 && form.senha !== form.senha2;

  return (
    <div className="flex flex-col gap-3.5">
      <Input
        label="E-mail"
        type="email"
        placeholder="seu@email.com"
        autoComplete="email"
        value={form.email}
        onChange={(e) => onChange({ email: e.target.value })}
      />
      <Input
        label="Senha"
        type="password"
        placeholder="Mínimo de 12 caracteres"
        autoComplete="new-password"
        hint="Mínimo de 12 caracteres"
        value={form.senha}
        onChange={(e) => onChange({ senha: e.target.value })}
      />
      <Input
        label="Confirmar senha"
        type="password"
        placeholder="Repita a senha"
        autoComplete="new-password"
        error={senhasConflitam ? 'As senhas não coincidem' : undefined}
        value={form.senha2}
        onChange={(e) => onChange({ senha2: e.target.value })}
      />

      <div className="flex items-start gap-3 rounded-md bg-primary-100 p-3.5">
        <input
          type="checkbox"
          checked={consent}
          onChange={onToggleConsent}
          aria-label="Li e aceito os Termos de uso e a Política de Privacidade"
          className="mt-0.5 h-[22px] w-[22px] flex-shrink-0 cursor-pointer accent-primary-300"
        />
        <span className="text-[13px] leading-[19px] text-primary-300">
          Li e aceito os{' '}
          <a href="#" className="font-bold">
            Termos de uso
          </a>{' '}
          e a{' '}
          <a href="#" className="font-bold">
            Política de Privacidade
          </a>
          . Entendo que meus dados de saúde são sigilosos — minha empresa nunca vê informações
          individuais.
        </span>
      </div>

      {/* A espera é longa e real (cadastro síncrono contra a Doutor ao Vivo).
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

      <Button
        color="accent"
        size="lg"
        fullWidth
        disabled={blockReason !== null}
        loading={pending}
        onClick={onSubmit}
      >
        Criar conta
      </Button>

      {/* Regra de ouro: botão desabilitado nunca fica mudo — diz o porquê. */}
      {blockReason && !pending && (
        <p className="text-center text-[13px] text-muted">{blockReason}</p>
      )}
    </div>
  );
}
