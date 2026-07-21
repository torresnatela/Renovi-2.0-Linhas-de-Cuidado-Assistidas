import { useEffect, useRef, useState } from 'react';
import { Navigate, useNavigate } from 'react-router-dom';

import type { RegisterRequest } from '../../shared/api';
import { dateBrToIso, digitsOnly, maskUf } from '../../shared/masks';
import { IconBack } from '../../shared/ui/icons';
import { AccessLayout } from './AccessLayout';
import { AccessConsentStep } from './steps/AccessConsentStep';
import { AddressStep } from './steps/AddressStep';
import { PersonalDataStep } from './steps/PersonalDataStep';
import { SuccessStep } from './steps/SuccessStep';
import { useRegister, useSession } from './useSession';
import { lookupCep } from './viaCep';

const MIN_PASSWORD = 12;

/** Estado do wizard. Vive em memória (fora da URL): recarregar reinicia — aceitável. */
export interface RegisterForm {
  nome: string;
  cpf: string;
  nasc: string;
  cel: string;
  cep: string;
  rua: string;
  numero: string;
  compl: string;
  bairro: string;
  cidade: string;
  uf: string;
  email: string;
  senha: string;
  senha2: string;
}

const EMPTY_FORM: RegisterForm = {
  nome: '',
  cpf: '',
  nasc: '',
  cel: '',
  cep: '',
  rua: '',
  numero: '',
  compl: '',
  bairro: '',
  cidade: '',
  uf: '',
  email: '',
  senha: '',
  senha2: '',
};

type Step = 1 | 2 | 3;
const STEP_TITLES: Record<Step, string> = { 1: 'Sobre você', 2: 'Seu endereço', 3: 'Seu acesso' };

export function RegisterPage() {
  const session = useSession();
  const register = useRegister();
  const navigate = useNavigate();

  const [form, setForm] = useState<RegisterForm>(EMPTY_FORM);
  const [step, setStep] = useState<Step>(1);
  const [stepError, setStepError] = useState<string | null>(null);
  const [consent, setConsent] = useState(false);
  const [cepLoading, setCepLoading] = useState(false);

  const headingRef = useRef<HTMLHeadingElement>(null);
  const prevStepRef = useRef(step);
  // Último CEP (8 dígitos) já consultado — evita repetir o lookup a cada tecla
  // depois que o CEP já está completo, e permite descartar resposta obsoleta.
  const lastCepRef = useRef<string | null>(null);

  // Avançar/voltar de passo desmonta o botão focado (Continuar/Voltar do passo
  // anterior) e o foco cai no <body>: usuário de leitor de tela perde o contexto
  // e o "Passo N de 3" nunca é anunciado. Movemos o foco para o heading do passo
  // recém-exibido — mas só em TRANSIÇÕES de passo, nunca no mount.
  //
  // O guard compara o PASSO ANTERIOR (não um boolean de primeiro-render): sob o
  // <StrictMode> do main.tsx, o React reexecuta o efeito em dev (mount → cleanup
  // → mount). Com o boolean, o segundo invoke via o flag já "false" e roubava o
  // foco no mount; com prevStep === step, o segundo invoke não faz nada.
  useEffect(() => {
    if (prevStepRef.current !== step) {
      headingRef.current?.focus();
    }
    prevStepRef.current = step;
  }, [step]);

  // Só barra a entrada de quem JÁ estava logado. Depois de um cadastro bem-sucedido
  // a sessão também existe, mas aí queremos a tela de sucesso, não o redirect.
  if (session.data && !register.isSuccess) return <Navigate to="/" replace />;

  const patch = (p: Partial<RegisterForm>) => setForm((f) => ({ ...f, ...p }));

  if (register.isSuccess) {
    const firstName = form.nome.trim().split(' ')[0] || 'tudo pronto';
    return (
      <AccessLayout active="cadastro" showSwitcher={false}>
        <SuccessStep firstName={firstName} onStart={() => navigate('/')} />
      </AccessLayout>
    );
  }

  // Validação leve (o que dá pra checar sem servidor). CPF válido, e-mail duplicado
  // e vínculo com a DAV são decisão da API — replicar aqui só criaria duas verdades.
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
    // O input dispara onCepComplete a cada tecla enquanto o CEP tiver 8 dígitos
    // (a máscara já satura em 8 — continuar digitando não muda o valor). Sem essa
    // guarda, cada keystroke extra refaria a consulta à ViaCEP.
    if (digits === lastCepRef.current) return;
    lastCepRef.current = digits;

    setCepLoading(true);
    const addr = await lookupCep(cep);
    setCepLoading(false);
    if (!addr) return; // falha/ inexistente: silêncio, preenche à mão
    // Resposta obsoleta: o usuário já mudou o CEP de novo antes desta resolver.
    if (lastCepRef.current !== digits) return;
    setForm((f) => ({
      ...f,
      rua: addr.street || f.rua,
      bairro: addr.neighborhood || f.bairro,
      cidade: addr.city || f.cidade,
      uf: maskUf(addr.state) || f.uf,
    }));
  }

  // Um único motivo, na ordem em que o usuário deve resolvê-los.
  function submitBlockReason(): string | null {
    if (!consent) return 'Aceite os termos para continuar.';
    if (form.senha.length < MIN_PASSWORD)
      return `A senha precisa ter pelo menos ${MIN_PASSWORD} caracteres.`;
    if (form.senha !== form.senha2) return 'As senhas precisam coincidir.';
    if (!form.email.trim()) return 'Informe seu e-mail.';
    return null;
  }

  function onSubmit() {
    if (submitBlockReason() !== null) return; // defensivo (o botão já fica desabilitado)

    const body: RegisterRequest = {
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
    register.mutate(body);
  }

  return (
    <AccessLayout active="cadastro">
      <div className="flex flex-col gap-[18px]">
        <div className="flex flex-col gap-2">
          <div className="flex items-center gap-2.5">
            {step > 1 && (
              <button
                type="button"
                onClick={goBack}
                aria-label="Voltar"
                className="inline-flex h-[34px] w-[34px] flex-shrink-0 items-center justify-center rounded-full border border-primary-200 bg-white text-primary-300 transition active:opacity-70"
              >
                <IconBack size={16} />
              </button>
            )}
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
                {STEP_TITLES[step]}
              </h1>
            </div>
          </div>
          <div className="h-1.5 overflow-hidden rounded-pill bg-primary-100">
            <div
              className="h-full rounded-pill bg-primary-300 transition-[width] duration-300"
              style={{ width: `${(step / 3) * 100}%` }}
            />
          </div>
          <span className="text-[12.5px] text-muted">Leva menos de 2 minutos.</span>
        </div>

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
            onSubmit={onSubmit}
            pending={register.isPending}
            blockReason={submitBlockReason()}
            apiError={register.isError ? register.error.message : null}
          />
        )}
      </div>
    </AccessLayout>
  );
}
