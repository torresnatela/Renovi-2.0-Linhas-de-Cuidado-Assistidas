import { useState } from 'react';
import { Link } from 'react-router-dom';

import type { MoodCheckin, MoodToday } from '../../shared/api';
import { Button } from '../../shared/ui/Button';
import { Card } from '../../shared/ui/Card';
import { IconCheck } from '../../shared/ui/icons';
import { MoodGrid, type MoodPoint } from './MoodGrid';
import { useGrantConsent, useMoodToday, useRecordCheckin } from './useMood';

/** Versão do termo de consentimento aceito no check-in de humor. */
const TERMO_VERSAO = 'v1';

/**
 * Vocabulário e cores 3×3 do design (marca da grade): linhas por energia
 * (baixa/média/alta) × colunas por valência (desagradável/neutro/agradável). É
 * AÇÚCAR DE EXIBIÇÃO derivado do ponto — o servidor guarda valência/energia crus.
 */
const ROTULOS = [
  ['Esgotado(a)', 'Quieto(a)', 'Tranquilo(a)'], // energia baixa
  ['Incomodado(a)', 'Neutro(a)', 'Bem'], // energia média
  ['Tenso(a)', 'Agitado(a)', 'Animado(a)'], // energia alta
] as const;
const CORES = [
  ['oklch(0.68 0.05 265)', 'oklch(0.72 0.04 250)', 'oklch(0.72 0.08 175)'],
  ['oklch(0.68 0.07 25)', 'oklch(0.72 0.03 260)', 'oklch(0.74 0.08 150)'],
  ['oklch(0.66 0.09 25)', 'oklch(0.75 0.08 60)', 'oklch(0.76 0.1 70)'],
] as const;

function regiao(p: MoodPoint): { rotulo: string; cor: string } {
  const vx = p.valencia / 100;
  const col = vx < 1 / 3 ? 0 : vx > 2 / 3 ? 2 : 1;
  const e = p.energia / 100;
  const row = e > 2 / 3 ? 2 : e < 1 / 3 ? 0 : 1; // energia alta = topo = linha 2
  return { rotulo: ROTULOS[row][col], cor: CORES[row][col] };
}

/**
 * O card de check-in de humor do aside da Jornada — uma máquina de estados guiada
 * pela resposta de `useMoodToday()`. É a ÚNICA superfície do check-in diário desde
 * o redesign (a antiga /humor foi aposentada): cobre consentimento, elegibilidade,
 * feito e aprofundamento, na forma compacta do painel lateral, roteando o
 * aprofundamento para `/avaliacoes/{codigo}`.
 */
export function MoodCheckinCard() {
  const today = useMoodToday();

  // Silêncio gentil (anti-engajamento): o aside não grita erro de humor. Some se
  // ainda não carregou ou falhou — a jornada continua legível.
  if (!today.data) return null;

  const data = today.data;
  if (data.reason === 'consent_required') return <ConsentCard />;
  if (data.reason === 'not_enrolled') return <NotEnrolledCard />;
  if (data.can_checkin || data.checkin) return <EnrolledCard today={data} />;
  return null;
}

function ConsentCard() {
  const grant = useGrantConsent();
  return (
    <Card className="flex flex-col gap-3">
      <span className="text-[17px] font-bold text-primary-300">Antes do primeiro check-in</span>
      <p className="text-[13px] leading-[19px] text-ink">
        O verificador de humor registra como você se sente ao longo do tempo — um dado sensível de
        saúde. Só você vê o que registra aqui, nunca gestores ou RH. Você pode revogar quando quiser.
      </p>
      <Button
        color="primary"
        size="sm"
        loading={grant.isPending}
        onClick={() => grant.mutate(TERMO_VERSAO)}
        className="self-start"
      >
        Aceitar e continuar
      </Button>
      {grant.isError && (
        <p className="text-[13px] text-error">
          Não foi possível registrar o consentimento. Tente de novo.
        </p>
      )}
    </Card>
  );
}

function NotEnrolledCard() {
  // Informativo neutro, SEM CTA: bloqueio não é erro (nunca vermelho).
  return (
    <Card className="flex flex-col gap-1.5">
      <span className="text-[15px] font-bold text-primary-300">Check-in de humor</span>
      <p className="text-[13px] leading-[19px] text-ink">
        Fica disponível quando sua linha de cuidado incluir o acompanhamento de humor. Fale com a
        equipe para ativar.
      </p>
    </Card>
  );
}

function EnrolledCard({ today }: { today: MoodToday }) {
  const record = useRecordCheckin();
  const [refazendo, setRefazendo] = useState(false);
  const [ponto, setPonto] = useState<MoodPoint>({ valencia: 50, energia: 50 });

  // O check-in mais recente conhecido: o retorno do submit ou o de hoje já feito.
  const salvo: MoodCheckin | null = record.data ?? today.checkin ?? null;
  const feito = Boolean(salvo) && !refazendo;

  return (
    <div className="flex flex-col gap-5">
      {feito ? (
        <FeitoCard
          checkin={salvo!}
          // "Refazer" só existe se a API ainda permite um novo registro hoje
          // (honestidade > design): sem can_checkin, não prometemos o que não dá.
          canRefazer={today.can_checkin}
          onRefazer={() => {
            setRefazendo(true);
            record.reset();
          }}
        />
      ) : (
        <AtivoCard
          ponto={ponto}
          onChange={setPonto}
          pending={record.isPending}
          erro={record.isError}
          onRegistrar={() =>
            record.mutate(
              {
                valencia: ponto.valencia,
                energia: ponto.energia,
                emotion_label: regiao(ponto).rotulo,
              },
              // Fecha o "Refazer": sem isto, `feito` (calculado com `refazendo`)
              // nunca voltava a true e o card ficava preso na grade mesmo após
              // um novo registro bem-sucedido — parecia que tinha falhado.
              { onSuccess: () => setRefazendo(false) },
            )
          }
        />
      )}
      <Deepening today={today} />
    </div>
  );
}

function AtivoCard({
  ponto,
  onChange,
  onRegistrar,
  pending,
  erro,
}: {
  ponto: MoodPoint;
  onChange: (p: MoodPoint) => void;
  onRegistrar: () => void;
  pending: boolean;
  erro: boolean;
}) {
  const { rotulo, cor } = regiao(ponto);
  return (
    <Card className="flex flex-col gap-3">
      <span className="self-start rounded-pill bg-primary-100 px-2.5 py-1 text-[10.5px] font-bold uppercase tracking-[0.06em] text-primary-300">
        Atividade do dia
      </span>
      <div className="flex flex-col gap-0.5">
        <span className="text-[17px] font-bold text-primary-300">Como você está agora?</span>
        <span className="text-[12.5px] text-muted">
          Leva uns 10 segundos. Só você vê o que registra aqui.
        </span>
      </div>

      <MoodGrid value={ponto} onChange={onChange} disabled={pending} />

      <div className="flex items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-2">
          <span className="h-3 w-3 shrink-0 rounded-full" style={{ background: cor }} />
          <span className="truncate text-[17px] font-bold text-primary-300">{rotulo}</span>
        </div>
        <Button color="accent" size="sm" loading={pending} onClick={onRegistrar}>
          Registrar
        </Button>
      </div>

      {erro && (
        <p className="text-[13px] text-error">Não foi possível registrar. Tente de novo.</p>
      )}
    </Card>
  );
}

function FeitoCard({
  checkin,
  canRefazer,
  onRefazer,
}: {
  checkin: MoodCheckin;
  canRefazer: boolean;
  onRefazer: () => void;
}) {
  // Rótulo do servidor quando disponível; senão, derivado do ponto (exibição).
  const label =
    checkin.emotion_label ||
    regiao({ valencia: checkin.valencia, energia: checkin.energia }).rotulo;
  return (
    <Card className="flex items-center gap-3 !p-4">
      <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-[rgba(41,176,29,0.12)] text-success">
        <IconCheck size={16} />
      </span>
      <div className="flex min-w-0 flex-1 flex-col gap-px">
        <span className="text-[14.5px] font-bold text-primary-300">
          Check-in de hoje feito: {label}
        </span>
        <span className="text-[12.5px] text-muted">Amanhã a gente se fala de novo.</span>
      </div>
      {canRefazer && (
        <button
          type="button"
          onClick={onRefazer}
          className="shrink-0 whitespace-nowrap rounded-sm text-[13px] font-bold text-primary-300 active:opacity-70 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-300"
        >
          Refazer
        </button>
      )}
    </Card>
  );
}

/**
 * O aprofundamento ofertado pelo gatilho (após o estado feito). O front NÃO decide
 * nada disto — só exibe `offer`/`escalate` que o servidor mandou, em tom neutro. O
 * convite roteia para `/avaliacoes/{codigo}` (a AssessmentPage já existente).
 */
function Deepening({ today }: { today: MoodToday }) {
  const offer = today.offer ?? null;
  const escalate = today.escalate ?? false;
  if (!offer && !escalate) return null;

  return (
    <>
      {escalate && (
        <Card className="flex flex-col gap-1.5">
          <span className="text-[14.5px] font-bold text-primary-300">
            Vale conversar com a equipe de cuidado
          </span>
          <span className="text-[13px] leading-[19px] text-ink">
            Seus últimos registros sugerem buscar apoio. Isso segue apenas para a trilha clínica —
            nunca para gestores.
          </span>
        </Card>
      )}
      {offer && (
        <Card className="flex flex-col gap-2.5">
          <span className="text-[14.5px] font-bold text-primary-300">
            A equipe preparou algumas perguntas rápidas
          </span>
          <span className="text-[13px] leading-[19px] text-muted">
            {offer === 'WHO5'
              ? 'Um check-in um pouco mais completo (2 min).'
              : 'Um último passo rápido nos ajuda a entender melhor (1 min).'}
          </span>
          <Link
            to={`/avaliacoes/${offer}`}
            className="self-start rounded-lg bg-primary-300 px-4 py-2 text-sm font-bold uppercase text-white transition active:opacity-70 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-300 focus-visible:ring-offset-2"
          >
            Responder agora
          </Link>
        </Card>
      )}
    </>
  );
}
