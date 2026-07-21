import { useState } from 'react';
import { Link, useParams } from 'react-router-dom';

import { ApiError, type Appointment, type AppointmentStatus } from '../../shared/api';
import { formatDateLong, formatTime } from '../../shared/datetime';
import { openExternal } from '../../shared/navigate';
import { Badge } from '../../shared/ui/Badge';
import { Button } from '../../shared/ui/Button';
import { Card } from '../../shared/ui/Card';
import { DateBadge } from '../../shared/ui/DateBadge';
import { ErrorNotice, Loading } from '../../shared/ui/feedback';
import { IconBack } from '../../shared/ui/icons';
import { AssessmentForm } from '../mood/AssessmentForm';
import { useMoodToday } from '../mood/useMood';
import { reasonText } from './reasons';
import { useAppointment, useJoinAppointment } from './useScheduling';

/** /consultas/:appointmentId — o detalhe, com o botão de entrar. */
export function AppointmentPage() {
  const { appointmentId } = useParams();
  const { data, isLoading, error } = useAppointment(appointmentId);

  return (
    <div className="mx-auto flex max-w-2xl flex-col gap-6">
      <Link
        to="/consultas"
        className="inline-flex w-fit items-center gap-1 text-sm font-bold text-primary-300"
      >
        <IconBack size={18} />
        Minhas consultas
      </Link>

      {isLoading && <Loading label="Carregando a consulta…" />}
      {error && <ErrorNotice error={error} />}

      {data && (
        <Card as="section" padding="lg" className="flex flex-col gap-6">
          <div className="flex items-start gap-4">
            <DateBadge iso={data.starts_at} timeZone={data.time_zone} />
            <div className="flex min-w-0 flex-1 flex-col gap-0.5">
              <h1 className="text-lg font-bold text-primary-300">{data.specialty.name}</h1>
              <p className="text-sm text-muted">com {data.professional.full_name}</p>
              <p className="mt-1 text-sm text-ink first-letter:uppercase">
                {formatDateLong(data.starts_at, data.time_zone)} às{' '}
                {formatTime(data.starts_at, data.time_zone)}
              </p>
            </div>
            <Selo consulta={data} />
          </div>

          <PainelDeEntrada consulta={data} />
        </Card>
      )}
    </div>
  );
}

/**
 * O botão de entrar — ou o MOTIVO.
 *
 * Regra de ouro do apps/web/CLAUDE.md: nunca um botão só desabilitado. E repare
 * que "30 minutos" não aparece neste arquivo: o que a tela mostra é a HORA que o
 * servidor mandou em `join.opens_at`. Mudar a antecedência é uma variável de
 * ambiente no servidor, sem tocar aqui — e um relógio adiantado no cliente não
 * abre a sala mais cedo, porque quem decide é o `join.status`.
 *
 * Fora de OPEN, nem monta o `JoinGate` — logo, nem consulta o humor de hoje: o
 * gate de pré-consulta só existe quando a sala pode de fato abrir.
 */
function PainelDeEntrada({ consulta }: { consulta: Appointment }) {
  const { status, opens_at, reason } = consulta.join;

  if (status !== 'OPEN') {
    const texto =
      status === 'TOO_EARLY'
        ? // Uma HORA é melhor que um cronômetro: dá para o paciente pôr um
          // despertador, e não exige timer, re-render por segundo nem teste com
          // relógio falso.
          `Você poderá entrar a partir das ${formatTime(opens_at, consulta.time_zone)} de ${formatDateLong(opens_at, consulta.time_zone)}.`
        : reasonText(reason, 'Esta consulta não está disponível para acesso.');

    return (
      <p role="status" className="rounded-md bg-primary-100 p-4 text-sm text-primary-300">
        {texto}
      </p>
    );
  }

  return <JoinGate consulta={consulta} />;
}

/**
 * A entrada na sala, COM o gate de pré-consulta (decisão de produto, 2026-07-20).
 *
 * Quando a janela está aberta e há um instrumento ofertado pelo gatilho de humor
 * (`today.offer` = WHO-5|PHQ-4), o clique em "Entrar" primeiro mostra o
 * `AssessmentForm` inline; só depois de respondê-lo (ou fechá-lo) a sala abre.
 *
 * Três invariantes tornam isso SEGURO — o gate nunca prende o paciente:
 *  - avaliado SÓ no clique: se `today` ainda não resolveu (ou falhou), `offer` é
 *    undefined e seguimos direto ao join (o loading do humor não atrasa a página);
 *  - `gateFeito` trava depois da primeira vez: uma falha no instrumento não
 *    re-oferece nem entra em loop — libera a entrada;
 *  - qualquer erro do instrumento é do próprio `AssessmentForm` (aviso discreto),
 *    e o "Concluir"/"Fechar" dele chama `onDone` → a sala abre.
 */
function JoinGate({ consulta }: { consulta: Appointment }) {
  const entrar = useJoinAppointment(consulta.id);
  const today = useMoodToday();
  const [mostrarForm, setMostrarForm] = useState(false);
  const [gateFeito, setGateFeito] = useState(false);

  function irParaSala() {
    // Guarda contra o duplo-clique no mesmo tick: cada clique é um POST que
    // registra acesso no servidor, e dois abririam a sala duas vezes.
    if (entrar.isPending) return;
    entrar.mutate(undefined, { onSuccess: (t) => openExternal(t.url) });
  }

  function aoClicarEntrar() {
    const offer = today.data?.offer;
    if (offer && !gateFeito) {
      setMostrarForm(true);
      return;
    }
    irParaSala();
  }

  function concluirGate() {
    setGateFeito(true);
    setMostrarForm(false);
    irParaSala();
  }

  const offer = today.data?.offer;
  if (mostrarForm && offer) {
    return (
      <Card padding="lg" className="flex flex-col gap-3 border-primary-200">
        <div className="flex flex-col gap-1">
          <h2 className="text-base font-bold text-primary-300">
            Antes da consulta, responda estas perguntas rápidas
          </h2>
          <p className="text-[13px] text-muted">
            São dois minutos e ajudam sua equipe de cuidado. Suas respostas vão apenas para a sua
            trilha clínica — nunca para gestores ou RH.
          </p>
        </div>
        <AssessmentForm codigo={offer} onDone={concluirGate} />
      </Card>
    );
  }

  return (
    <div className="flex flex-col gap-3">
      <Button color="primary" loading={entrar.isPending} onClick={aoClicarEntrar}>
        {entrar.isPending ? 'Abrindo sua sala…' : 'Entrar na consulta'}
      </Button>

      {/*
        O 409 aqui é a rede de proteção da corrida (relógio adiantado, cache
        velho): o botão só apareceu porque o `join.status` dizia OPEN, mas quem
        manda é o servidor no momento do clique.
      */}
      {entrar.isError && (
        <p role="alert" className="rounded-md bg-[rgba(205,25,25,0.08)] p-4 text-sm text-error">
          {reasonText(
            entrar.error instanceof ApiError ? entrar.error.reason : undefined,
            entrar.error.message,
          )}
        </p>
      )}
    </div>
  );
}

/**
 * O selo de status.
 *
 * UNCONFIRMED tem texto próprio e é o mais importante: a consulta PODE existir na
 * Doutor ao Vivo e nunca vamos descobrir sozinhos (ADR-016). Escondê-la seria
 * pior que a incerteza — o paciente pode ter uma consulta de verdade marcada.
 */
function Selo({ consulta }: { consulta: Appointment }) {
  const mapa: Record<
    AppointmentStatus,
    { tone: 'success' | 'neutral' | 'accent' | 'alert'; label: string }
  > = {
    CONFIRMED: { tone: 'success', label: 'Confirmada' },
    PROCESSING: { tone: 'neutral', label: 'Confirmando…' },
    UNCONFIRMED: { tone: 'alert', label: 'Verificando' },
    CANCELLED: { tone: 'neutral', label: 'Cancelada' },
  };
  const { tone, label } = mapa[consulta.status];

  if (consulta.status === 'UNCONFIRMED') {
    return (
      <span
        className="shrink-0"
        title="A Doutor ao Vivo não confirmou a tempo. Nossa equipe está verificando se a consulta foi marcada."
      >
        <Badge tone={tone}>{label}</Badge>
      </span>
    );
  }

  return (
    <span className="shrink-0">
      <Badge tone={tone}>{label}</Badge>
    </span>
  );
}
