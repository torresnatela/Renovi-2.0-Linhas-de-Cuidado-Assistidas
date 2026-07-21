import type { DiaResumo } from './DateStep';

const cx = (...c: Array<string | false | null | undefined>) => c.filter(Boolean).join(' ');

/**
 * O calendário mobile (mock `Agendar`, passo 2). PROIBIDO `new Date()` no fuso do
 * browser: a origem dos horários é −03:00 e o runner pode estar em UTC, então um
 * slot das 23:30 viraria o dia seguinte (off-by-one). Toda a aritmética aqui é
 * civil, sobre a string `dayKey` (`AAAA-MM-DD`) que a página JÁ derivou no fuso da
 * agenda — o algoritmo de Howard Hinnant converte data civil ⇄ dia serial sem
 * jamais instanciar um `Date`.
 */

// Dias seriais desde 1970-01-01 (calendário gregoriano proléptico).
function daysFromCivil(y: number, m: number, d: number): number {
  const yy = m <= 2 ? y - 1 : y;
  const era = Math.floor((yy >= 0 ? yy : yy - 399) / 400);
  const yoe = yy - era * 400;
  const doy = Math.floor((153 * (m > 2 ? m - 3 : m + 9) + 2) / 5) + d - 1;
  const doe = yoe * 365 + Math.floor(yoe / 4) - Math.floor(yoe / 100) + doy;
  return era * 146097 + doe - 719468;
}

function civilFromDays(z: number): [number, number, number] {
  const zz = z + 719468;
  const era = Math.floor((zz >= 0 ? zz : zz - 146096) / 146097);
  const doe = zz - era * 146097;
  const yoe = Math.floor(
    (doe - Math.floor(doe / 1460) + Math.floor(doe / 36524) - Math.floor(doe / 146096)) / 365,
  );
  const y = yoe + era * 400;
  const doy = doe - (365 * yoe + Math.floor(yoe / 4) - Math.floor(yoe / 100));
  const mp = Math.floor((5 * doy + 2) / 153);
  const d = doy - Math.floor((153 * mp + 2) / 5) + 1;
  const m = mp < 10 ? mp + 3 : mp - 9;
  return [m <= 2 ? y + 1 : y, m, d];
}

const pad = (n: number) => String(n).padStart(2, '0');

function parts(key: string): [number, number, number] {
  return [Number(key.slice(0, 4)), Number(key.slice(5, 7)), Number(key.slice(8, 10))];
}

/** Dia da semana civil de um `dayKey`: 0=domingo … 6=sábado. */
export function dowOf(key: string): number {
  const [y, m, d] = parts(key);
  return ((daysFromCivil(y, m, d) + 4) % 7 + 7) % 7;
}

/** Soma `n` dias a um `dayKey`, devolvendo outro `dayKey` — pura aritmética civil. */
export function addDays(key: string, n: number): string {
  const [y, m, d] = parts(key);
  const [ny, nm, nd] = civilFromDays(daysFromCivil(y, m, d) + n);
  return `${ny}-${pad(nm)}-${pad(nd)}`;
}

const DOW_ABBR = ['dom', 'seg', 'ter', 'qua', 'qui', 'sex', 'sáb'];
const MESES = [
  'Janeiro', 'Fevereiro', 'Março', 'Abril', 'Maio', 'Junho',
  'Julho', 'Agosto', 'Setembro', 'Outubro', 'Novembro', 'Dezembro',
];

interface Celula {
  key: string;
  disponivel: boolean;
}

/**
 * As 28 células (4 semanas), começando no DOMINGO da semana do primeiro dia
 * disponível. Um dia é `disponivel` se está no conjunto derivado da availability.
 */
export function buildCalendarCells(dias: DiaResumo[]): Celula[] {
  if (dias.length === 0) return [];
  // `dayKey` (AAAA-MM-DD) ordena lexicograficamente = cronologicamente.
  const primeiro = dias.reduce((a, b) => (a.key < b.key ? a : b)).key;
  const inicio = addDays(primeiro, -dowOf(primeiro));
  const disponiveis = new Set(dias.map((d) => d.key));
  return Array.from({ length: 28 }, (_, i) => {
    const key = addDays(inicio, i);
    return { key, disponivel: disponiveis.has(key) };
  });
}

/** "Julho – Agosto de 2026" — rótulo dos meses cobertos pela grade. */
function rotuloMeses(celulas: Celula[]): string {
  if (celulas.length === 0) return '';
  const [y0, m0] = parts(celulas[0].key);
  const [y1, m1] = parts(celulas[celulas.length - 1].key);
  if (y0 === y1 && m0 === m1) return `${MESES[m0 - 1]} de ${y0}`;
  if (y0 === y1) return `${MESES[m0 - 1]} – ${MESES[m1 - 1]} de ${y0}`;
  return `${MESES[m0 - 1]} de ${y0} – ${MESES[m1 - 1]} de ${y1}`;
}

interface CalendarGridProps {
  dias: DiaResumo[];
  selecionado?: string | null;
  onEscolher: (dayKey: string) => void;
}

export function CalendarGrid({ dias, selecionado, onEscolher }: CalendarGridProps) {
  const celulas = buildCalendarCells(dias);

  return (
    <div className="flex flex-col gap-3.5">
      <div className="flex items-center justify-between">
        <span className="text-[15px] font-bold text-primary-300">{rotuloMeses(celulas)}</span>
        <span className="text-xs text-muted">próximas 4 semanas</span>
      </div>

      <div className="grid grid-cols-7 gap-1.5">
        {celulas.map((c) => {
          const [, mm, dd] = parts(c.key);
          const dow = DOW_ABBR[dowOf(c.key)];
          const nome = `${dow}, ${pad(dd)}/${pad(mm)}`;
          const sel = c.disponivel && c.key === selecionado;

          if (!c.disponivel) {
            // Dia sem horário: inerte, esmaecido, sem ponto (mock).
            return (
              <span
                key={c.key}
                aria-hidden="true"
                className="flex flex-col items-center gap-[3px] rounded-md px-0 py-2 opacity-45"
              >
                <span className="text-[10px] font-bold uppercase text-muted">{dow}</span>
                <span className="text-[15px] font-bold text-muted">{dd}</span>
              </span>
            );
          }

          return (
            <button
              key={c.key}
              type="button"
              aria-label={nome}
              aria-pressed={sel}
              onClick={() => onEscolher(c.key)}
              className={cx(
                'flex flex-col items-center gap-[3px] rounded-md border-[1.5px] px-0 py-2 transition',
                'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-300',
                sel
                  ? 'border-primary-300 bg-primary-300'
                  : 'border-primary-200 bg-white hover:border-primary-300',
              )}
            >
              <span
                className={cx('text-[10px] font-bold uppercase', sel ? 'text-white/75' : 'text-muted')}
              >
                {dow}
              </span>
              <span className={cx('text-[15px] font-bold', sel ? 'text-white' : 'text-primary-300')}>
                {dd}
              </span>
              <span
                className={cx('h-[5px] w-[5px] rounded-full', sel ? 'bg-white' : 'bg-success')}
              />
            </button>
          );
        })}
      </div>
    </div>
  );
}
