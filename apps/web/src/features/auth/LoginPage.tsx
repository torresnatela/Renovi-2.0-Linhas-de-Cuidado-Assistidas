import { FormEvent, useState } from 'react';
import { Link, Navigate } from 'react-router-dom';

import { useLogin, useSession } from './useSession';

export function LoginPage() {
  const session = useSession();
  const login = useLogin();
  const [cpf, setCpf] = useState('');
  const [password, setPassword] = useState('');

  if (session.data) return <Navigate to="/" replace />;

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    login.mutate({ cpf, password });
  }

  return (
    <main className="mx-auto max-w-md p-6">
      <h1 className="mb-6 text-2xl font-semibold">Entrar</h1>

      <form onSubmit={onSubmit} className="space-y-4">
        <label className="block">
          <span className="text-sm font-medium">CPF</span>
          <input
            name="cpf"
            value={cpf}
            onChange={(e) => setCpf(e.target.value)}
            required
            autoComplete="username"
            inputMode="numeric"
            placeholder="000.000.000-00"
            className="mt-1 w-full rounded border px-3 py-2"
          />
        </label>

        <label className="block">
          <span className="text-sm font-medium">Senha</span>
          <input
            name="password"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
            autoComplete="current-password"
            className="mt-1 w-full rounded border px-3 py-2"
          />
        </label>

        {login.isError && (
          <p role="alert" className="rounded bg-red-50 p-3 text-sm text-red-700">
            {login.error.message}
          </p>
        )}

        <button
          type="submit"
          disabled={login.isPending}
          className="w-full rounded bg-emerald-700 py-2 font-medium text-white disabled:opacity-60"
        >
          {login.isPending ? 'Entrando…' : 'Entrar'}
        </button>
      </form>

      <p className="mt-6 text-sm">
        Ainda não tem conta?{' '}
        <Link to="/cadastro" className="text-emerald-700 underline">
          Cadastre-se
        </Link>
      </p>
    </main>
  );
}
