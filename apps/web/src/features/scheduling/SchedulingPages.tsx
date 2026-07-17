import { Link, useParams } from 'react-router-dom';

import type { Professional } from '../../shared/api';
import { useProfessionals, useSpecialties } from './useScheduling';
import { Carregando, Erro, Vazio, Passos } from './ui';

/**
 * Passo 1 — escolher a especialidade.
 *
 * A API só devolve especialidade que leva a algum lugar (tem profissional com
 * horário livre), então aqui não há filtro nenhum: lista vazia significa mesmo
 * "não há atendimento agora", e não "o servidor caiu" — essa diferença é 503 e
 * cai no <Erro/>.
 */
export function SpecialtyPickerPage() {
  const { data, isLoading, error } = useSpecialties();

  return (
    <main className="mx-auto max-w-3xl px-6 py-10">
      <Passos atual={1} />
      <h2 className="mb-1 text-lg font-medium">Qual atendimento você precisa?</h2>
      <p className="mb-6 text-sm text-slate-600">Escolha a especialidade para ver os profissionais.</p>

      {isLoading && <Carregando>Carregando especialidades…</Carregando>}
      {error && <Erro error={error} />}
      {data?.length === 0 && (
        <Vazio>
          Não há atendimento disponível no momento. Tente novamente mais tarde.
        </Vazio>
      )}

      <ul className="grid gap-3 sm:grid-cols-2">
        {data?.map((e) => (
          <li key={e.id}>
            <Link
              to={`/agendar/${e.id}`}
              className="block rounded-lg border border-slate-200 bg-white p-4 hover:border-emerald-600 hover:bg-emerald-50"
            >
              <span className="font-medium">{e.name}</span>
            </Link>
          </li>
        ))}
      </ul>
    </main>
  );
}

/** Passo 2 — escolher o profissional. */
export function ProfessionalPickerPage() {
  const { specialtyId } = useParams();
  const { data, isLoading, error } = useProfessionals(specialtyId);

  return (
    <main className="mx-auto max-w-3xl px-6 py-10">
      <Passos atual={2} />
      <h2 className="mb-1 text-lg font-medium">Com quem você quer se consultar?</h2>
      <p className="mb-6 text-sm text-slate-600">
        Só aparecem profissionais com horário livre.{' '}
        <Link to="/agendar" className="text-emerald-700 underline">
          Trocar de especialidade
        </Link>
      </p>

      {isLoading && <Carregando>Carregando profissionais…</Carregando>}
      {error && <Erro error={error} />}
      {data?.length === 0 && (
        <Vazio>
          Nenhum profissional desta especialidade tem horário livre agora.{' '}
          <Link to="/agendar" className="text-emerald-700 underline">
            Ver outras especialidades
          </Link>
        </Vazio>
      )}

      <ul className="grid gap-3">
        {data?.map((p) => (
          <li key={p.id}>
            <Link
              to={`/agendar/${specialtyId}/${p.id}`}
              className="flex items-center gap-4 rounded-lg border border-slate-200 bg-white p-4 hover:border-emerald-600 hover:bg-emerald-50"
            >
              <Avatar profissional={p} />
              <span>
                <span className="block font-medium">{p.full_name}</span>
                <span className="block text-sm text-slate-600">{registro(p)}</span>
              </span>
            </Link>
          </li>
        ))}
      </ul>
    </main>
  );
}

/**
 * "CRP/SP 06/123456 · RQE 54321".
 *
 * Montado aqui, e não na API, porque as partes têm significado próprio: o RQE só
 * existe para especialista e é exibido condicionalmente. Uma string pronta vinda
 * do servidor obrigaria a fazer parsing para qualquer coisa além de ecoar.
 */
function registro(p: Professional): string {
  const base = `${p.license.council}/${p.license.region} ${p.license.number}`;
  return p.license.rqe ? `${base} · RQE ${p.license.rqe}` : base;
}

/** Sem foto cai nas iniciais — nunca num ícone quebrado. `image_url` é comum ser nulo. */
function Avatar({ profissional }: { profissional: Professional }) {
  if (profissional.image_url) {
    return (
      <img
        src={profissional.image_url}
        alt=""
        className="h-12 w-12 shrink-0 rounded-full object-cover"
      />
    );
  }
  const iniciais = profissional.full_name
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((n) => n[0]?.toUpperCase())
    .join('');
  return (
    <span
      aria-hidden="true"
      className="flex h-12 w-12 shrink-0 items-center justify-center rounded-full bg-emerald-100 font-medium text-emerald-800"
    >
      {iniciais}
    </span>
  );
}
