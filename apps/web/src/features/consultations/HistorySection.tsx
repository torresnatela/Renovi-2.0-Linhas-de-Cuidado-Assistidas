import { useState } from 'react';

import type { CareAppointment } from '../../shared/api';
import { formatDateTimeShort, monthKey, monthLabel } from '../../shared/datetime';
import { Empty } from '../../shared/ui/feedback';
import { IconCheck } from '../../shared/ui/icons';
import { SectionLabel } from './parts';

type Filtro = 'todas' | 'realizadas' | 'canceladas';
type HistoryStatus = 'realizada' | 'falta' | 'cancelada';

const CHIPS: { value: Filtro; label: string }[] = [
  { value: 'todas', label: 'Todas' },
  { value: 'realizadas', label: 'Realizadas' },
  { value: 'canceladas', label: 'Canceladas' },
];

// `falta` é neutra ("Não realizada"), não uma falha vermelha — o paciente pode
// não ter comparecido por mil motivos, e a tela não julga. Só aparece em "Todas".
const META: Record<HistoryStatus, { word: string; label: string; done: boolean }> = {
  realizada: { word: 'por vídeo', label: 'Realizada', done: true },
  cancelada: { word: 'cancelada', label: 'Cancelada', done: false },
  falta: { word: 'não realizada', label: 'Não realizada', done: false },
};

function matches(status: CareAppointment['status'], filtro: Filtro): boolean {
  if (filtro === 'todas') return true;
  if (filtro === 'realizadas') return status === 'realizada';
  return status === 'cancelada';
}

/**
 * A aba "Histórico": chips de filtro (client-side) + a lista agrupada por mês,
 * do mais recente para o mais antigo.
 *
 * O agrupamento usa `monthKey`/`monthLabel` NO fuso de cada consulta — uma
 * consulta às 23:30 de 31/07 em SP é 01/08 em UTC, e agrupar pelo mês do browser
 * a jogaria no balde errado. As consultas chegam já ordenadas desc, então os
 * grupos saem em ordem sem reordenar.
 */
export function HistorySection({ consultas }: { consultas: CareAppointment[] }) {
  const [filtro, setFiltro] = useState<Filtro>('todas');
  const filtradas = consultas.filter((c) => matches(c.status, filtro));

  const grupos: { key: string; label: string; itens: CareAppointment[] }[] = [];
  for (const c of filtradas) {
    const key = monthKey(c.scheduled_at, c.time_zone);
    let grupo = grupos.find((g) => g.key === key);
    if (!grupo) {
      grupo = { key, label: monthLabel(c.scheduled_at, c.time_zone), itens: [] };
      grupos.push(grupo);
    }
    grupo.itens.push(c);
  }

  return (
    <div className="flex max-w-[720px] flex-col gap-4">
      <div className="flex flex-wrap gap-2">
        {CHIPS.map((chip) => {
          const active = filtro === chip.value;
          return (
            <button
              key={chip.value}
              type="button"
              aria-pressed={active}
              onClick={() => setFiltro(chip.value)}
              className={`rounded-pill border px-4 py-[9px] text-[13.5px] font-bold transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-300 ${
                active
                  ? 'border-primary-300 bg-primary-300 text-white'
                  : 'border-primary-200 bg-white text-primary-300'
              }`}
            >
              {chip.label}
            </button>
          );
        })}
      </div>

      {filtradas.length === 0 ? (
        <Empty title="Nenhuma consulta por aqui ainda." />
      ) : (
        grupos.map((grupo) => (
          <div key={grupo.key} className="flex flex-col gap-3">
            <SectionLabel>{grupo.label}</SectionLabel>
            {grupo.itens.map((c) => (
              <HistoryRow key={c.id} consulta={c} />
            ))}
          </div>
        ))
      )}

      <span className="py-1.5 text-[12.5px] text-muted">
        Consultas canceladas com mais de 24h de antecedência não contam na sua cota.
      </span>
    </div>
  );
}

function HistoryRow({ consulta }: { consulta: CareAppointment }) {
  const meta = META[consulta.status as HistoryStatus];
  const caption = `${formatDateTimeShort(consulta.scheduled_at, consulta.time_zone)} · ${meta.word}`;

  return (
    <div className="flex items-center gap-3.5 rounded-lg border border-primary-100 bg-white px-[18px] py-[15px]">
      <span
        className={`inline-flex h-[38px] w-[38px] shrink-0 items-center justify-center rounded-full ${
          meta.done ? 'bg-[rgba(41,176,29,0.12)] text-success' : 'bg-primary-100 text-muted'
        }`}
      >
        {meta.done ? <IconCheck size={16} /> : <IconX size={15} />}
      </span>
      <div className="flex min-w-0 flex-1 flex-col gap-px">
        <span className="font-bold text-primary-300">{consulta.label}</span>
        <span className="text-[13px] text-muted">{caption}</span>
      </div>
      <span
        className={`shrink-0 whitespace-nowrap text-[12.5px] font-bold ${
          meta.done ? 'text-success' : 'text-muted'
        }`}
      >
        {meta.label}
      </span>
    </div>
  );
}

// O X do histórico não está no icons.tsx do DS (que é editado por outra etapa) —
// é um traço local, decorativo, herdando a cor do círculo que o embrulha.
function IconX({ size = 15 }: { size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2.2}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M6 6l12 12M18 6L6 18" />
    </svg>
  );
}
