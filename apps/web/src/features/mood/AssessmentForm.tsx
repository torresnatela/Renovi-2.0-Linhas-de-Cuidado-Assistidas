import { useState } from 'react';

import { ApiError, type AssessmentCode } from '../../shared/api';
import { useAssessmentAvailability, useSubmitAssessment } from './useMood';

type Instrumento = {
  titulo: string;
  instrucao: string;
  /** Rótulos da escala Likert; o índice do rótulo É o valor enviado. */
  escala: string[];
  itens: string[];
};

// Textos das validações BR (WHO-5 e PHQ-4 são de uso livre). A ORDEM do PHQ-4 é
// humor (PHQ-2, itens 1–2) e depois ansiedade (GAD-2, itens 3–4), casando com o
// motor de pontuação (scoring.ScorePHQ4).
const INSTRUMENTOS: Record<AssessmentCode, Instrumento> = {
  WHO5: {
    titulo: 'Índice de Bem-Estar (WHO-5)',
    instrucao: 'Nas últimas duas semanas, com que frequência você se sentiu assim?',
    escala: [
      'Em nenhum momento',
      'De vez em quando',
      'Menos da metade do tempo',
      'Mais da metade do tempo',
      'A maior parte do tempo',
      'O tempo todo',
    ],
    itens: [
      'Eu me senti alegre e de bom humor',
      'Eu me senti calmo(a) e relaxado(a)',
      'Eu me senti ativo(a) e vigoroso(a)',
      'Eu me senti descansado(a) ao acordar',
      'Meu dia a dia esteve cheio de coisas que me interessam',
    ],
  },
  PHQ4: {
    titulo: 'Rastreio de Humor e Ansiedade (PHQ-4)',
    instrucao:
      'Nas últimas duas semanas, com que frequência você foi incomodado(a) pelos problemas abaixo?',
    escala: ['Nenhuma vez', 'Vários dias', 'Mais da metade dos dias', 'Quase todos os dias'],
    itens: [
      'Pouco interesse ou prazer em fazer as coisas',
      'Sentir-se para baixo, deprimido(a) ou sem perspectiva',
      'Sentir-se nervoso(a), ansioso(a) ou muito tenso(a)',
      'Não conseguir parar ou controlar as preocupações',
    ],
  },
};

const FAIXA_LABEL: Record<string, string> = {
  normal: 'dentro do esperado',
  sinaliza: 'baixo bem-estar (sinaliza)',
  encaminha: 'rastreio positivo',
  moderado: 'sofrimento moderado',
};

export function AssessmentForm({ codigo, onDone }: { codigo: AssessmentCode; onDone: () => void }) {
  const inst = INSTRUMENTOS[codigo];
  const availability = useAssessmentAvailability(codigo);
  const submit = useSubmitAssessment();
  const [answers, setAnswers] = useState<(number | null)[]>(() => inst.itens.map(() => null));

  const bloqueio =
    availability.data && !availability.data.eligibility.allowed
      ? availability.data.eligibility.blocks[0]
      : null;
  const completo = answers.every((a) => a !== null);
  const resultado = submit.data;

  if (resultado) {
    return (
      <section className="rounded-lg border border-emerald-200 bg-emerald-50 p-6" role="status">
        <h3 className="mb-2 font-medium">{inst.titulo} — resultado</h3>
        {codigo === 'WHO5' && resultado.index_score != null && (
          <p className="text-sm text-emerald-900">
            Índice de bem-estar: <strong>{resultado.index_score}/100</strong> —{' '}
            {FAIXA_LABEL[resultado.faixa] ?? resultado.faixa}
          </p>
        )}
        {codigo === 'PHQ4' && resultado.subscores && (
          <p className="text-sm text-emerald-900">
            Humor (PHQ-2): <strong>{resultado.subscores.phq2}</strong> · Ansiedade (GAD-2):{' '}
            <strong>{resultado.subscores.gad2}</strong> — {FAIXA_LABEL[resultado.faixa] ?? resultado.faixa}
          </p>
        )}
        {resultado.flag_encaminhar && (
          <p className="mt-3 rounded bg-rose-50 p-3 text-sm text-rose-900">
            Seu resultado sugere que vale conversar com a equipe de cuidado. Isso vai apenas para a
            trilha clínica — nunca para gestores ou RH.
          </p>
        )}
        <button
          type="button"
          onClick={onDone}
          className="mt-4 rounded-md bg-emerald-700 px-4 py-2 text-sm font-medium text-white hover:bg-emerald-800"
        >
          Concluir
        </button>
      </section>
    );
  }

  return (
    <section className="rounded-lg border border-slate-200 bg-white p-6">
      <div className="mb-2 flex items-center justify-between">
        <h3 className="font-medium">{inst.titulo}</h3>
        <button type="button" onClick={onDone} className="text-sm text-slate-500 underline">
          Fechar
        </button>
      </div>
      <p className="mb-4 text-sm text-slate-600">{inst.instrucao}</p>

      {bloqueio ? (
        <p className="rounded bg-amber-50 p-3 text-sm text-amber-900">{bloqueio.reason}</p>
      ) : (
        <>
          <ol className="space-y-4">
            {inst.itens.map((texto, i) => (
              <li key={i}>
                <p className="mb-2 text-sm text-slate-800">
                  {i + 1}. {texto}
                </p>
                <div className="flex flex-wrap gap-2">
                  {inst.escala.map((rotulo, v) => {
                    const on = answers[i] === v;
                    return (
                      <button
                        key={v}
                        type="button"
                        onClick={() => setAnswers((prev) => prev.map((a, k) => (k === i ? v : a)))}
                        className={
                          'rounded border px-2 py-1 text-xs ' +
                          (on
                            ? 'border-emerald-600 bg-emerald-600 text-white'
                            : 'border-slate-300 text-slate-600')
                        }
                      >
                        {v} · {rotulo}
                      </button>
                    );
                  })}
                </div>
              </li>
            ))}
          </ol>
          <button
            type="button"
            onClick={() => completo && submit.mutate({ codigo, items: answers.map((a) => a as number) })}
            disabled={!completo || submit.isPending}
            className="mt-6 rounded-md bg-emerald-700 px-4 py-2 text-sm font-medium text-white hover:bg-emerald-800 disabled:opacity-60"
          >
            {submit.isPending ? 'Enviando…' : 'Enviar respostas'}
          </button>
          {submit.isError && (
            <p className="mt-3 text-sm text-rose-700">
              {submit.error instanceof ApiError ? submit.error.message : 'Não foi possível enviar.'}
            </p>
          )}
        </>
      )}
    </section>
  );
}
