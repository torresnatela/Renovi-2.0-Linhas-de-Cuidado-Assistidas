import { useRef, useState } from 'react';
import { Link } from 'react-router-dom';

import { ApiError, type AssessmentCode, type MoodCheckin, type MoodToday } from '../../shared/api';
import { AssessmentForm } from './AssessmentForm';
import {
  useGrantConsent,
  useHelpNow,
  useMoodInstrument,
  useMoodToday,
  useRecordCheckin,
} from './useMood';

/** Versão do termo de consentimento aceito nesta tela. */
const TERMO_VERSAO = 'v1';

/**
 * Vocabulário e cores PRÓPRIOS da Renovi (a paleta do Mood Meter é marca da Yale
 * — Anexo C.4). O quadrante é DERIVADO pelo servidor; aqui só o traduzimos.
 */
const QUADRANTES: Record<string, { rotulo: string; cor: string }> = {
  agradavel_ativado: { rotulo: 'Agradável e com energia', cor: 'bg-amber-200/70' },
  agradavel_calmo: { rotulo: 'Agradável e tranquilo', cor: 'bg-emerald-200/70' },
  desagradavel_ativado: { rotulo: 'Desagradável e tenso', cor: 'bg-rose-200/70' },
  desagradavel_calmo: { rotulo: 'Desagradável e sem energia', cor: 'bg-indigo-200/70' },
};

function rotuloQuadrante(q: string): string {
  return QUADRANTES[q]?.rotulo ?? q;
}

export function MoodPage() {
  const today = useMoodToday();

  return (
    <main className="mx-auto max-w-3xl px-6 py-10">
      <div className="mb-6 flex items-center justify-between">
        <h2 className="text-lg font-medium">Como você está hoje?</h2>
        <Link to="/" className="text-sm text-emerald-700 underline">
          Início
        </Link>
      </div>

      {today.isLoading && <p className="text-slate-500">Carregando…</p>}
      {today.isError && (
        <p className="text-rose-700">Não foi possível carregar. Tente novamente.</p>
      )}

      {today.data && today.data.reason === 'consent_required' && <ConsentCard />}
      {today.data && today.data.reason === 'not_enrolled' && <NotEnrolledCard />}
      {today.data && today.data.can_checkin && (
        <div className="space-y-6">
          <CheckinSection existente={today.data.checkin ?? null} />
          <DeepeningSection today={today.data} />
        </div>
      )}

      {/* Afordância permanente de ajuda (guardrail 6.2): triagem, não tratamento. */}
      {today.data && <HelpNowCard />}
    </main>
  );
}

/**
 * Anel gatilhado no front: quando o gatilho oferece um instrumento (WHO-5/PHQ-4),
 * mostra o convite e o formulário; quando escala, mostra o encaminhamento clínico.
 * O front NÃO decide nada disto — só exibe `offer`/`escalate` que o servidor mandou.
 */
function DeepeningSection({ today }: { today: MoodToday }) {
  const [aberto, setAberto] = useState<AssessmentCode | null>(null);
  const offer = today.offer ?? null;

  return (
    <div className="space-y-4">
      {today.escalate && (
        <div className="rounded-lg border border-rose-200 bg-rose-50 p-4 text-sm text-rose-900">
          <strong>Vale conversar com a equipe de cuidado.</strong> Seu histórico recente sugere
          buscar apoio. Isso segue apenas para a trilha clínica — nunca para gestores.
        </div>
      )}

      {offer && !aberto && (
        <div className="flex items-center justify-between rounded-lg border border-emerald-200 bg-emerald-50 p-4">
          <p className="text-sm text-emerald-900">
            {offer === 'WHO5'
              ? 'Que tal um check-in um pouco mais completo? (2 min)'
              : 'Um último passo rápido nos ajuda a entender melhor. (1 min)'}
          </p>
          <button
            type="button"
            onClick={() => setAberto(offer)}
            className="shrink-0 rounded-md bg-emerald-700 px-4 py-2 text-sm font-medium text-white hover:bg-emerald-800"
          >
            Responder {offer === 'WHO5' ? 'WHO-5' : 'PHQ-4'}
          </button>
        </div>
      )}

      {aberto && <AssessmentForm codigo={aberto} onDone={() => setAberto(null)} />}
    </div>
  );
}

function HelpNowCard() {
  const help = useHelpNow();
  return (
    <section className="mt-6">
      {help.data ? (
        <div className="rounded-lg border border-sky-200 bg-sky-50 p-4 text-sm text-sky-900" role="status">
          <strong>{help.data.label}.</strong> {help.data.message}
        </div>
      ) : (
        <button
          type="button"
          onClick={() => help.mutate()}
          disabled={help.isPending}
          className="text-sm font-medium text-sky-700 underline disabled:opacity-60"
        >
          {help.isPending ? 'Conectando…' : 'Preciso de ajuda agora'}
        </button>
      )}
    </section>
  );
}

function ConsentCard() {
  const grant = useGrantConsent();
  return (
    <section className="rounded-lg border border-slate-200 bg-white p-6">
      <h3 className="mb-2 font-medium">Antes de começar</h3>
      <p className="mb-4 text-sm text-slate-600">
        O verificador de humor registra como você se sente ao longo do tempo. É um dado
        sensível de saúde: fica visível apenas para você e para a equipe clínica, nunca para
        gestores ou RH em nível individual. Você pode revogar a qualquer momento. Ao continuar,
        você autoriza esse registro (LGPD, art. 11).
      </p>
      <button
        type="button"
        onClick={() => grant.mutate(TERMO_VERSAO)}
        disabled={grant.isPending}
        className="rounded-md bg-emerald-700 px-4 py-2 text-sm font-medium text-white hover:bg-emerald-800 disabled:opacity-60"
      >
        {grant.isPending ? 'Registrando…' : 'Aceitar e continuar'}
      </button>
      {grant.isError && (
        <p className="mt-3 text-sm text-rose-700">Não foi possível registrar o consentimento.</p>
      )}
    </section>
  );
}

function NotEnrolledCard() {
  return (
    <section className="rounded-lg border border-slate-200 bg-white p-6">
      <h3 className="mb-2 font-medium">Check-in indisponível</h3>
      <p className="text-sm text-slate-600">
        Você ainda não tem uma linha de cuidado ativa com o verificador de humor. Fale com a
        equipe para ativar a sua.
      </p>
    </section>
  );
}

function CheckinSection({ existente }: { existente: MoodCheckin | null }) {
  const instrument = useMoodInstrument('GRID');
  const record = useRecordCheckin();
  const [ponto, setPonto] = useState<{ valencia: number; energia: number } | null>(null);
  const [tags, setTags] = useState<string[]>([]);
  const gridRef = useRef<HTMLDivElement>(null);

  // O check-in mais recente conhecido: o que o servidor devolveu no submit, ou o
  // de hoje já existente. Nunca recalculamos o quadrante — exibimos o do servidor.
  const salvo = record.data ?? existente;

  function selecionar(e: React.MouseEvent<HTMLDivElement>) {
    const el = gridRef.current;
    if (!el) return;
    const rect = el.getBoundingClientRect();
    const x = clamp((e.clientX - rect.left) / rect.width);
    const y = clamp((e.clientY - rect.top) / rect.height);
    setPonto({ valencia: Math.round(x * 100), energia: Math.round((1 - y) * 100) });
  }

  // Acessibilidade: a grade também é operável por teclado. As setas movem o ponto
  // em passos de 5; partindo do centro (50,50) quando ainda não há ponto.
  function ajustar(dValencia: number, dEnergia: number) {
    setPonto((prev) => {
      const base = prev ?? { valencia: 50, energia: 50 };
      return {
        valencia: Math.min(100, Math.max(0, base.valencia + dValencia)),
        energia: Math.min(100, Math.max(0, base.energia + dEnergia)),
      };
    });
  }

  function teclado(e: React.KeyboardEvent<HTMLDivElement>) {
    const passo = 5;
    const movimentos: Record<string, [number, number]> = {
      ArrowLeft: [-passo, 0],
      ArrowRight: [passo, 0],
      ArrowUp: [0, passo],
      ArrowDown: [0, -passo],
    };
    const mov = movimentos[e.key];
    if (!mov) return;
    e.preventDefault();
    ajustar(mov[0], mov[1]);
  }

  function registrar() {
    if (!ponto) return;
    record.mutate({
      valencia: ponto.valencia,
      energia: ponto.energia,
      context_tags: tags.length ? tags : undefined,
    });
  }

  return (
    <section className="space-y-6">
      {salvo && (
        <div
          className="rounded-lg border border-emerald-200 bg-emerald-50 p-4 text-sm text-emerald-900"
          role="status"
        >
          Humor de hoje registrado: <strong>{rotuloQuadrante(salvo.quadrante)}</strong>. Você pode
          atualizar tocando na grade novamente.
        </div>
      )}

      <div className="rounded-lg border border-slate-200 bg-white p-6">
        <p className="mb-1 text-sm text-slate-600">
          Toque no ponto que representa como você se sente (ou use as setas do teclado). O eixo
          horizontal é o quão agradável; o vertical, o quanto de energia.
        </p>

        <div className="mx-auto mt-4 max-w-sm">
          <div className="mb-1 text-center text-xs text-slate-400">Mais energia</div>
          <div className="flex items-stretch gap-1">
            <div className="flex items-center text-xs text-slate-400">
              <span className="-rotate-180 [writing-mode:vertical-rl]">Desagradável</span>
            </div>
            <div
              ref={gridRef}
              onClick={selecionar}
              onKeyDown={teclado}
              role="button"
              tabIndex={0}
              aria-label="Grade de humor: valência por energia"
              className="relative grid aspect-square w-full grid-cols-2 grid-rows-2 overflow-hidden rounded-lg border border-slate-300"
            >
              {/* Quadrantes (linha de cima = mais energia). Cores próprias da Renovi. */}
              <div className={QUADRANTES.desagradavel_ativado.cor} />
              <div className={QUADRANTES.agradavel_ativado.cor} />
              <div className={QUADRANTES.desagradavel_calmo.cor} />
              <div className={QUADRANTES.agradavel_calmo.cor} />
              {ponto && (
                <span
                  data-testid="mood-marker"
                  className="pointer-events-none absolute h-4 w-4 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-white bg-slate-900 shadow"
                  style={{ left: `${ponto.valencia}%`, top: `${100 - ponto.energia}%` }}
                />
              )}
            </div>
            <div className="flex items-center text-xs text-slate-400">
              <span className="[writing-mode:vertical-rl]">Agradável</span>
            </div>
          </div>
          <div className="mt-1 text-center text-xs text-slate-400">Menos energia</div>
        </div>

        {/* Anuncia o ponto escolhido a leitores de tela (a grade é visual). */}
        {ponto && (
          <p className="sr-only" aria-live="polite" data-testid="mood-value">
            Selecionado: valência {ponto.valencia} de 100, energia {ponto.energia} de 100.
          </p>
        )}

        {instrument.data && instrument.data.context_tags.length > 0 && (
          <fieldset className="mt-6">
            <legend className="mb-2 text-sm text-slate-600">O que influenciou? (opcional)</legend>
            <div className="flex flex-wrap gap-2">
              {instrument.data.context_tags.map((t) => {
                const on = tags.includes(t.chave);
                return (
                  <button
                    key={t.chave}
                    type="button"
                    aria-pressed={on}
                    onClick={() =>
                      setTags((prev) =>
                        on ? prev.filter((c) => c !== t.chave) : [...prev, t.chave],
                      )
                    }
                    className={
                      'rounded-full border px-3 py-1 text-sm ' +
                      (on
                        ? 'border-emerald-600 bg-emerald-600 text-white'
                        : 'border-slate-300 text-slate-700')
                    }
                  >
                    {t.rotulo}
                  </button>
                );
              })}
            </div>
          </fieldset>
        )}

        <button
          type="button"
          onClick={registrar}
          disabled={!ponto || record.isPending}
          className="mt-6 rounded-md bg-emerald-700 px-4 py-2 text-sm font-medium text-white hover:bg-emerald-800 disabled:opacity-60"
        >
          {record.isPending ? 'Registrando…' : 'Registrar meu humor'}
        </button>
        {record.isError && (
          <p className="mt-3 text-sm text-rose-700">
            {record.error instanceof ApiError
              ? record.error.message
              : 'Não foi possível registrar.'}
          </p>
        )}
      </div>
    </section>
  );
}

function clamp(v: number): number {
  return Math.min(1, Math.max(0, v));
}
