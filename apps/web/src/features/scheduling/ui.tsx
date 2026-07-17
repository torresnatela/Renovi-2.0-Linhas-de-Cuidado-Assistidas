import type { ReactNode } from 'react';

import { ApiError } from '../../shared/api';

/**
 * Pedacinhos de UI repetidos pelas telas do agendamento.
 *
 * Ficam AQUI, dentro da feature, e não em `shared/`: extrair um design system a
 * partir das cinco telas de uma feature só assa os acidentes dessa feature. A
 * hora honesta é quando a `journey/` trouxer as telas 6 a 10 e a forma aparecer
 * (ADR-001, "sem overengineering").
 */

export function Carregando({ children }: { children: ReactNode }) {
  return (
    <p role="status" className="rounded bg-slate-100 p-3 text-sm text-slate-700">
      {children}
    </p>
  );
}

export function Vazio({ children }: { children: ReactNode }) {
  return (
    <p className="rounded border border-dashed border-slate-300 p-6 text-center text-sm text-slate-600">
      {children}
    </p>
  );
}

/**
 * Erro de carregamento.
 *
 * O 503 tem frase própria: "não há horários" e "não conseguimos ler os horários"
 * são coisas diferentes, e confundi-las faz o paciente desistir de um
 * profissional que estava livre. Por isso a API responde 503 em vez de lista
 * vazia — e jogar fora essa distinção aqui anularia a decisão.
 */
export function Erro({ error }: { error: unknown }) {
  const indisponivel = error instanceof ApiError && error.status === 503;
  return (
    <p role="alert" className="rounded bg-red-50 p-3 text-sm text-red-700">
      {indisponivel
        ? 'Não conseguimos consultar a agenda agora. Isso é um problema nosso, não seu — tente de novo em instantes.'
        : error instanceof Error
          ? error.message
          : 'Não foi possível carregar.'}
    </p>
  );
}

/** A trilha do wizard. A URL é o estado, então ela só reflete onde já estamos. */
export function Passos({ atual }: { atual: 1 | 2 | 3 }) {
  const nomes = ['Especialidade', 'Profissional', 'Horário'];
  return (
    <ol className="mb-6 flex flex-wrap gap-2 text-sm" aria-label="Etapas do agendamento">
      {nomes.map((nome, i) => {
        const passo = i + 1;
        const feito = passo < atual;
        const aqui = passo === atual;
        return (
          <li key={nome} className="flex items-center gap-2">
            <span
              aria-current={aqui ? 'step' : undefined}
              className={
                aqui
                  ? 'font-medium text-emerald-700'
                  : feito
                    ? 'text-slate-600'
                    : 'text-slate-400'
              }
            >
              {passo}. {nome}
            </span>
            {passo < nomes.length && <span className="text-slate-300">›</span>}
          </li>
        );
      })}
    </ol>
  );
}
