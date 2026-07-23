import { useEffect, useRef, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';

import type { OnboardingInfo, RegisterRequest } from '../../shared/api';
import { dateBrToIso, digitsOnly, maskPhone, maskUf } from '../../shared/masks';
import { Button } from '../../shared/ui/Button';
import { ErrorNotice, Loading } from '../../shared/ui/feedback';
import { IconBack } from '../../shared/ui/icons';
import { AccessLayout } from '../auth/AccessLayout';
import type { RegisterForm } from '../auth/RegisterPage';
import { AccessConsentStep } from '../auth/steps/AccessConsentStep';
import { AddressStep } from '../auth/steps/AddressStep';
import { PersonalDataStep } from '../auth/steps/PersonalDataStep';
import { SuccessStep } from '../auth/steps/SuccessStep';
import { lookupCep } from '../auth/viaCep';
import { CompanyConfirmStep } from './steps/CompanyConfirmStep';
import { useCompleteOnboarding, useDeclineOnboarding, useOnboardingInfo } from './useOnboarding';

const MIN_PASSWORD = 12;

/**
 * Página do convite de onboarding (/onboarding/:token). Busca o pré-preenchimento e,
 * com ele, hospeda o wizard de cadastro (reusando os passos do /cadastro) + o passo de
 * confirmação da empresa. Um token inválido/expirado vira uma mensagem com saída, não
 * um formulário quebrado.
 */
export function OnboardingPage() {
  const { token = '' } = useParams();
  const info = useOnboardingInfo(token);

  if (info.isPending) {
    return (
      <AccessLayout active="cadastro" showSwitcher={false}>
        <Loading label="Carregando seu convite…" />
      </AccessLayout>
    );
  }

  if (info.isError) {
    return (
      <AccessLayout active="cadastro" showSwitcher={false}>
        <div className="flex flex-col gap-3">
          <h1 className="text-[22px] font-bold text-primary-300">Convite indisponível</h1>
          <ErrorNotice error={info.error} />
          <p className="text-[13px] text-muted">
            Se você acha que isso é um engano, fale com o RH da sua empresa.
          </p>
        </div>
      </AccessLayout>
    );
  }

  return <OnboardingFlow token={token} info={info.data} />;
}

type Step = 1 | 2 | 3 | 4;
const STEP_TITLES: Record<1 | 2 | 3, string> = {
  1: 'Sobre você',
  2: 'Seu endereço',
  3: 'Seu acesso',
};

function seedForm(info: OnboardingInfo): RegisterForm {
  return {
    // A Gestão já nos mandou nome/e-mail/telefone — pré-preenchemos. O CPF NÃO: só
    // temos o cpf_hmac; o paciente digita e o backend confere por HMAC.
    nome: info.invite_name ?? '',
    cpf: '',
    nasc: '',
    cel: info.invite_phone ? maskPhone(info.invite_phone) : '',
    cep: '',
    rua: '',
    numero: '',
    compl: '',
    bairro: '',
    cidade: '',
    uf: '',
    email: info.invite_email ?? '',
    senha: '',
    senha2: '',
  };
}

function OnboardingFlow({ token, info }: { token: string; info: OnboardingInfo }) {
  const navigate = useNavigate();
  const complete = useCompleteOnboarding(token);
  const decline = useDeclineOnboarding(token);

  const [form, setForm] = useState<RegisterForm>(() => seedForm(info));
  const [step, setStep] = useState<Step>(1);
  const [stepError, setStepError] = useState<string | null>(null);
  const [consent, setConsent] = useState(false);
  const [cepLoading, setCepLoading] = useState(false);

  const headingRef = useRef<HTMLHeadingElement>(null);
  const prevStepRef = useRef(step);
  const lastCepRef = useRef<string | null>(null);

  // Move o foco para o heading do passo recém-exibido, mas só em TRANSIÇÕES (o guard
  // por passo anterior evita roubar o foco no mount, inclusive sob <StrictMode>).
  useEffect(() => {
    if (prevStepRef.current !== step) headingRef.current?.focus();
    prevStepRef.current = step;
  }, [step]);

  const patch = (p: Partial<RegisterForm>) => setForm((f) => ({ ...f, ...p }));

  // Conclusão bem-sucedida: a sessão já veio do complete → tela de sucesso, e a
  // jornada em seguida (a conta já está matriculada).
  if (complete.isSuccess) {
    const firstName = form.nome.trim().split(' ')[0] || 'tudo pronto';
    return (
      <AccessLayout active="cadastro" showSwitcher={false}>
        <SuccessStep firstName={firstName} onStart={() => navigate('/')} />
      </AccessLayout>
    );
  }

  // Recusa registrada: beco sem saída amigável (não criamos conta).
  if (decline.isSuccess) {
    return (
      <AccessLayout active="cadastro" showSwitcher={false}>
        <DeclinedStep onRegister={() => navigate('/cadastro')} />
      </AccessLayout>
    );
  }

  // Validação leve (o que dá para checar sem servidor) — igual à do /cadastro.
  function validateStep1(): string | null {
    if (!form.nome.trim()) return 'Informe seu nome completo.';
    if (digitsOnly(form.cpf).length !== 11) return 'Confira o CPF — precisa ter 11 dígitos.';
    if (dateBrToIso(form.nasc) === null) return 'Confira a data de nascimento (dd/mm/aaaa).';
    const cel = digitsOnly(form.cel).length;
    if (cel < 10 || cel > 11) return 'Confira o celular com DDD.';
    return null;
  }

  function validateStep2(): string | null {
    if (digitsOnly(form.cep).length !== 8) return 'Confira o CEP — precisa ter 8 dígitos.';
    if (!form.rua.trim()) return 'Informe o endereço.';
    if (!form.numero.trim()) return 'Informe o número.';
    if (!form.bairro.trim()) return 'Informe o bairro.';
    if (!form.cidade.trim()) return 'Informe a cidade.';
    if (maskUf(form.uf).length !== 2) return 'Informe a UF (2 letras).';
    return null;
  }

  function submitBlockReason(): string | null {
    if (!consent) return 'Aceite os termos para continuar.';
    if (form.senha.length < MIN_PASSWORD)
      return `A senha precisa ter pelo menos ${MIN_PASSWORD} caracteres.`;
    if (form.senha !== form.senha2) return 'As senhas precisam coincidir.';
    if (!form.email.trim()) return 'Informe seu e-mail.';
    return null;
  }

  function advance(validate: () => string | null, next: Step) {
    const err = validate();
    if (err) {
      setStepError(err);
      return;
    }
    setStepError(null);
    setStep(next);
  }

  function goBack() {
    setStepError(null);
    setStep((s) => (s > 1 ? ((s - 1) as Step) : s));
  }

  async function onCepComplete(cep: string) {
    const digits = digitsOnly(cep);
    if (digits === lastCepRef.current) return;
    lastCepRef.current = digits;

    setCepLoading(true);
    const addr = await lookupCep(cep);
    setCepLoading(false);
    if (!addr) return;
    if (lastCepRef.current !== digits) return;
    setForm((f) => ({
      ...f,
      rua: addr.street || f.rua,
      bairro: addr.neighborhood || f.bairro,
      cidade: addr.city || f.cidade,
      uf: maskUf(addr.state) || f.uf,
    }));
  }

  function buildBody(): RegisterRequest {
    return {
      full_name: form.nome.trim(),
      cpf: form.cpf,
      birth_date: dateBrToIso(form.nasc) ?? '',
      email: form.email.trim(),
      phone: digitsOnly(form.cel),
      password: form.senha,
      address: {
        zip_code: form.cep,
        street: form.rua.trim(),
        number: form.numero.trim(),
        complement: form.compl.trim(),
        neighborhood: form.bairro.trim(),
        city: form.cidade.trim(),
        state: form.uf,
      },
    };
  }

  return (
    <AccessLayout active="cadastro" showSwitcher={false}>
      <div className="flex flex-col gap-[18px]">
        {step <= 3 ? (
          <div className="flex flex-col gap-2">
            <div className="flex items-center gap-2.5">
              {step > 1 && <BackButton onClick={goBack} />}
              <div className="flex flex-col">
                <span
                  aria-live="polite"
                  className="text-[11px] font-bold uppercase tracking-[0.08em] text-muted"
                >
                  Passo {step} de 3
                </span>
                <h1
                  ref={headingRef}
                  tabIndex={-1}
                  className="text-[22px] font-bold text-primary-300 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-300 focus-visible:ring-offset-2"
                >
                  {STEP_TITLES[step as 1 | 2 | 3]}
                </h1>
              </div>
            </div>
            <div className="h-1.5 overflow-hidden rounded-pill bg-primary-100">
              <div
                data-testid="onboarding-progress-fill"
                className="h-full rounded-pill bg-primary-300 transition-[width] duration-300"
                style={{ width: `${(step / 3) * 100}%` }}
              />
            </div>
            <span className="text-[12.5px] text-muted">
              Você foi convidado para o Renovi. Complete seu cadastro para começar.
            </span>
          </div>
        ) : (
          <div className="flex items-center gap-2.5">
            <BackButton onClick={goBack} />
            <span className="text-[11px] font-bold uppercase tracking-[0.08em] text-muted">
              Última etapa
            </span>
          </div>
        )}

        {step === 1 && (
          <PersonalDataStep
            form={form}
            onChange={patch}
            onContinue={() => advance(validateStep1, 2)}
            error={stepError}
          />
        )}
        {step === 2 && (
          <AddressStep
            form={form}
            onChange={patch}
            onContinue={() => advance(validateStep2, 3)}
            onCepComplete={onCepComplete}
            cepLoading={cepLoading}
            error={stepError}
          />
        )}
        {step === 3 && (
          <AccessConsentStep
            form={form}
            onChange={patch}
            consent={consent}
            onToggleConsent={() => setConsent((c) => !c)}
            onSubmit={() => advance(submitBlockReason, 4)}
            pending={false}
            blockReason={submitBlockReason()}
            apiError={null}
            submitLabel="Continuar"
          />
        )}
        {step === 4 && (
          <CompanyConfirmStep
            companies={info.companies}
            onConfirm={() => complete.mutate(buildBody())}
            onDecline={() => decline.mutate()}
            pending={complete.isPending}
            declining={decline.isPending}
            apiError={complete.isError ? complete.error.message : null}
          />
        )}
      </div>
    </AccessLayout>
  );
}

function BackButton({ onClick }: { onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-label="Voltar"
      className="inline-flex h-[34px] w-[34px] flex-shrink-0 items-center justify-center rounded-full border border-primary-200 bg-white text-primary-300 transition active:opacity-70"
    >
      <IconBack size={16} />
    </button>
  );
}

/** Beco sem saída da recusa: sem conta criada, com um caminho para o autocadastro. */
function DeclinedStep({ onRegister }: { onRegister: () => void }) {
  return (
    <div className="flex flex-col items-center gap-4 px-3 py-6 text-center">
      <h1 className="text-[24px] font-bold text-primary-300">Tudo bem</h1>
      <p className="max-w-[340px] text-[15px] leading-[23px] text-ink">
        Você indicou que não faz parte dessa empresa, então não concluímos esse vínculo. Se foi um
        engano, fale com o RH da sua empresa para reenviar o convite.
      </p>
      <div className="mt-2 w-full">
        <Button color="ghost" size="lg" fullWidth onClick={onRegister}>
          Fazer meu cadastro mesmo assim
        </Button>
      </div>
    </div>
  );
}
