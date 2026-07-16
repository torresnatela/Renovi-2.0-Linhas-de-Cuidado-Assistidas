import { FormEvent, useState } from 'react';
import { Link, Navigate } from 'react-router-dom';

import type { RegisterRequest } from '../../shared/api';
import { useRegister, useSession } from './useSession';

const MIN_PASSWORD = 12;

function Field({
  label,
  name,
  ...props
}: { label: string; name: string } & React.InputHTMLAttributes<HTMLInputElement>) {
  return (
    <label className="block">
      <span className="text-sm font-medium">{label}</span>
      <input name={name} className="mt-1 w-full rounded border px-3 py-2" {...props} />
    </label>
  );
}

export function RegisterPage() {
  const session = useSession();
  const register = useRegister();
  const [erroLocal, setErroLocal] = useState<string | null>(null);

  if (session.data) return <Navigate to="/" replace />;

  function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setErroLocal(null);

    const f = new FormData(e.currentTarget);
    const password = String(f.get('password'));

    // Só o que dá para checar sem o servidor. CPF, e-mail duplicado e vínculo
    // com a Doutor ao Vivo são decisão da API — replicar essas regras aqui só
    // criaria duas verdades que divergem com o tempo.
    if (password.length < MIN_PASSWORD) {
      setErroLocal(`A senha precisa ter pelo menos ${MIN_PASSWORD} caracteres.`);
      return;
    }
    if (password !== String(f.get('password_confirm'))) {
      setErroLocal('As senhas não conferem.');
      return;
    }

    const body: RegisterRequest = {
      full_name: String(f.get('full_name')),
      cpf: String(f.get('cpf')),
      birth_date: String(f.get('birth_date')),
      email: String(f.get('email')),
      phone: String(f.get('phone')),
      password,
      address: {
        zip_code: String(f.get('zip_code')),
        street: String(f.get('street')),
        number: String(f.get('number')),
        complement: String(f.get('complement') ?? ''),
        neighborhood: String(f.get('neighborhood')),
        city: String(f.get('city')),
        state: String(f.get('state')),
      },
    };
    register.mutate(body);
  }

  const erro = erroLocal ?? (register.isError ? register.error.message : null);

  return (
    <main className="mx-auto max-w-2xl p-6">
      <h1 className="mb-2 text-2xl font-semibold">Criar minha conta</h1>
      <p className="mb-6 text-sm text-slate-600">
        Seus dados são usados para criar seu cadastro de saúde e liberar suas consultas.
      </p>

      <form onSubmit={onSubmit} className="space-y-6">
        <fieldset disabled={register.isPending} className="space-y-4">
          <legend className="mb-2 font-medium">Dados pessoais</legend>
          <Field label="Nome completo" name="full_name" required minLength={3} autoComplete="name" />
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="CPF" name="cpf" required inputMode="numeric" placeholder="000.000.000-00" />
            <Field label="Data de nascimento" name="birth_date" type="date" required />
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="E-mail" name="email" type="email" required autoComplete="email" />
            <Field
              label="Celular"
              name="phone"
              required
              inputMode="tel"
              placeholder="11912345678"
              autoComplete="tel"
            />
          </div>
        </fieldset>

        <fieldset disabled={register.isPending} className="space-y-4">
          <legend className="mb-2 font-medium">Endereço</legend>
          <div className="grid gap-4 sm:grid-cols-3">
            <Field label="CEP" name="zip_code" required inputMode="numeric" placeholder="06472000" />
            <div className="sm:col-span-2">
              <Field label="Logradouro" name="street" required />
            </div>
          </div>
          <div className="grid gap-4 sm:grid-cols-3">
            <Field label="Número" name="number" required />
            <Field label="Complemento" name="complement" />
            <Field label="Bairro" name="neighborhood" required />
          </div>
          <div className="grid gap-4 sm:grid-cols-3">
            <div className="sm:col-span-2">
              <Field label="Cidade" name="city" required />
            </div>
            <Field label="UF" name="state" required maxLength={2} placeholder="SP" />
          </div>
        </fieldset>

        <fieldset disabled={register.isPending} className="space-y-4">
          <legend className="mb-2 font-medium">Senha</legend>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field
              label={`Senha (mínimo ${MIN_PASSWORD} caracteres)`}
              name="password"
              type="password"
              required
              minLength={MIN_PASSWORD}
              autoComplete="new-password"
            />
            <Field
              label="Confirme a senha"
              name="password_confirm"
              type="password"
              required
              autoComplete="new-password"
            />
          </div>
        </fieldset>

        {erro && (
          <p role="alert" className="rounded bg-red-50 p-3 text-sm text-red-700">
            {erro}
          </p>
        )}

        {/*
          A espera aqui é longa e real: o cadastro é síncrono contra a Doutor ao
          Vivo, que já levou dezenas de segundos em homologação. Um spinner mudo
          faria o usuário achar que travou e recarregar a página no meio — então
          dizemos o que está acontecendo e quanto pode demorar.
        */}
        {register.isPending && (
          <p role="status" className="rounded bg-emerald-50 p-3 text-sm text-emerald-900">
            Estamos criando seu cadastro de saúde na Doutor ao Vivo. Isso pode levar até um minuto —
            <strong> não feche nem recarregue esta página.</strong>
          </p>
        )}

        <button
          type="submit"
          disabled={register.isPending}
          className="w-full rounded bg-emerald-700 py-2 font-medium text-white disabled:opacity-60"
        >
          {register.isPending ? 'Criando sua conta…' : 'Criar conta'}
        </button>
      </form>

      <p className="mt-6 text-sm">
        Já tem conta?{' '}
        <Link to="/entrar" className="text-emerald-700 underline">
          Entrar
        </Link>
      </p>
    </main>
  );
}
