import { FormEvent, useState } from 'react';
import { Link, Navigate } from 'react-router-dom';

import { maskCpf } from '../../shared/masks';
import { Button } from '../../shared/ui/Button';
import { Input } from '../../shared/ui/Input';
import { AccessLayout } from './AccessLayout';
import { useLogin, useSession } from './useSession';

export function LoginPage() {
  const session = useSession();
  const login = useLogin();
  const [cpf, setCpf] = useState('');
  const [password, setPassword] = useState('');
  const [showHelp, setShowHelp] = useState(false);

  if (session.data) return <Navigate to="/" replace />;

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    login.mutate({ cpf, password });
  }

  return (
    <AccessLayout active="login">
      <form onSubmit={onSubmit} className="flex flex-col gap-4">
        <div className="flex flex-col gap-1">
          <h1 className="text-[28px] font-bold text-primary-300">Que bom te ver</h1>
          <p className="text-[15px] text-muted">Entre para acessar sua jornada de cuidado.</p>
        </div>

        <Input
          label="CPF"
          name="cpf"
          value={cpf}
          onChange={(e) => setCpf(maskCpf(e.target.value))}
          required
          autoComplete="username"
          inputMode="numeric"
          placeholder="000.000.000-00"
        />

        <Input
          label="Senha"
          name="password"
          type="password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          required
          autoComplete="current-password"
          placeholder="Sua senha"
        />

        {/* Não há "esqueci minha senha" na API — o acesso nasce do vínculo da
            empresa. A ajuda é orientação de contato, sem endpoint. */}
        <button
          type="button"
          onClick={() => setShowHelp((v) => !v)}
          aria-expanded={showHelp}
          className="self-end text-[13.5px] font-bold text-primary-300 transition active:opacity-70"
        >
          Precisa de ajuda para entrar?
        </button>

        {showHelp && (
          <div className="rounded-md bg-primary-100 p-3.5 text-[13px] leading-[19px] text-primary-300">
            Seu acesso é criado quando sua empresa vincula você ao Renovi. Se ainda não tem senha,
            crie sua conta. Se esqueceu a senha ou não consegue entrar, fale com o RH da sua empresa
            ou escreva para <span className="font-bold">suporte@renovisaude.com.br</span>.
          </div>
        )}

        {login.isError && (
          <p role="alert" className="rounded-md bg-[rgba(205,25,25,0.08)] p-3.5 text-sm text-error">
            {login.error.message}
          </p>
        )}

        <Button type="submit" color="primary" size="lg" fullWidth loading={login.isPending}>
          Entrar
        </Button>

        <p className="text-center text-[13.5px] text-muted">
          Primeira vez aqui?{' '}
          <Link to="/cadastro" className="font-bold text-primary-300">
            Crie sua conta
          </Link>
        </p>
      </form>
    </AccessLayout>
  );
}
