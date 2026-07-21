import { useState } from 'react';

import { ApiError, type AssessmentCode } from '../../shared/api';
import { Button } from '../../shared/ui/Button';
import { useAssessmentAvailability, useSubmitAssessment } from './useMood';

const cx = (...c: Array<string | false | null | undefined>) => c.filter(Boolean).join(' ');

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

/**
 * O formulário do instrumento periódico (WHO-5/PHQ-4), no visual do design system.
 * Estilizado só com tokens do DS (sem paletas fora do design system). O resultado
 * (faixa e encaminhamento) sai em tom INFORMATIVO: um rastreio positivo não é falha do
 * paciente, então nunca é vermelho de erro; só o ERRO REAL do envio usa `error`.
 *
 * O contrato — props `{ codigo, onDone }`, uma pergunta por vez e os rótulos dos
 * botões ("Enviar respostas", "Concluir", "Fechar") — é preservado: a
 * AssessmentPage e o gate de pré-consulta (AppointmentPage) dependem dele.
 */
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
      <section role="status" className="flex flex-col gap-3 rounded-md bg-primary-100 p-5">
        <span className="text-[15px] font-bold text-primary-300">Seu resultado</span>
        {codigo === 'WHO5' && resultado.index_score != null && (
          <p className="text-sm text-ink">
            Índice de bem-estar:{' '}
            <strong className="text-primary-300">{resultado.index_score}/100</strong> —{' '}
            {FAIXA_LABEL[resultado.faixa] ?? resultado.faixa}
          </p>
        )}
        {codigo === 'PHQ4' && resultado.subscores && (
          <p className="text-sm text-ink">
            Humor (PHQ-2): <strong className="text-primary-300">{resultado.subscores.phq2}</strong> ·
            Ansiedade (GAD-2):{' '}
            <strong className="text-primary-300">{resultado.subscores.gad2}</strong> —{' '}
            {FAIXA_LABEL[resultado.faixa] ?? resultado.faixa}
          </p>
        )}
        {resultado.flag_encaminhar && (
          <p className="rounded-md bg-[rgba(250,143,27,0.12)] p-3 text-sm text-ink">
            Seu resultado sugere que vale conversar com a equipe de cuidado. Isso vai apenas para a
            trilha clínica — nunca para gestores ou RH.
          </p>
        )}
        <Button color="primary" size="sm" onClick={onDone} className="self-start">
          Concluir
        </Button>
      </section>
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-start justify-between gap-4">
        <p className="text-sm text-muted">{inst.instrucao}</p>
        <button
          type="button"
          onClick={onDone}
          className="shrink-0 rounded-sm text-sm font-bold text-primary-300 underline transition active:opacity-70 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-300"
        >
          Fechar
        </button>
      </div>

      {bloqueio ? (
        // Bloqueio de cadência é estado do plano, não erro: fundo subtle navy.
        <p role="status" className="rounded-md bg-primary-100 p-3 text-sm text-primary-300">
          {bloqueio.reason}
        </p>
      ) : (
        <>
          <ol className="flex flex-col gap-5">
            {inst.itens.map((texto, i) => (
              <li key={i} className="flex flex-col gap-2">
                <p className="text-sm text-ink">
                  {i + 1}. {texto}
                </p>
                <div className="flex flex-wrap gap-2">
                  {inst.escala.map((rotulo, v) => {
                    const on = answers[i] === v;
                    return (
                      <button
                        key={v}
                        type="button"
                        aria-pressed={on}
                        onClick={() => setAnswers((prev) => prev.map((a, k) => (k === i ? v : a)))}
                        className={cx(
                          'rounded-pill border px-3 py-1 text-xs font-semibold transition',
                          'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-300',
                          on
                            ? 'border-primary-300 bg-primary-300 text-white'
                            : 'border-primary-200 text-muted hover:border-primary-300 hover:text-primary-300',
                        )}
                      >
                        {v} · {rotulo}
                      </button>
                    );
                  })}
                </div>
              </li>
            ))}
          </ol>

          <div className="flex flex-col gap-3">
            <Button
              color="primary"
              size="md"
              loading={submit.isPending}
              disabled={!completo}
              onClick={() =>
                completo && submit.mutate({ codigo, items: answers.map((a) => a as number) })
              }
              className="self-start"
            >
              {submit.isPending ? 'Enviando…' : 'Enviar respostas'}
            </Button>
            {submit.isError && (
              // Falha do ENVIO é erro real: tom de erro (o único vermelho aqui).
              <p role="alert" className="rounded-md bg-[rgba(205,25,25,0.08)] p-3 text-sm text-error">
                {submit.error instanceof ApiError ? submit.error.message : 'Não foi possível enviar.'}
              </p>
            )}
          </div>
        </>
      )}
    </div>
  );
}
