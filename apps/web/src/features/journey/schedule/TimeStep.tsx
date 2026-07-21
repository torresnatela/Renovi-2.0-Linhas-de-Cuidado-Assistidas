import type { AnnotatedSlot, EligibilityBlock } from '../../../shared/api';
import { formatDateLong, formatTime } from '../../../shared/datetime';
import { EligibilityNotice } from '../../../shared/ui/EligibilityNotice';

const cx = (...c: Array<string | false | null | undefined>) => c.filter(Boolean).join(' ');

interface TimeStepProps {
  slots: AnnotatedSlot[];
  /** O slot da intenção viva (horário selecionado), ou null. */
  intencaoSlotId: string | null;
  /** Enquanto um agendamento está em voo, TODOS os horários travam. */
  emVoo: boolean;
  onEscolher: (slot: AnnotatedSlot) => void;
}

/**
 * A grade de horários do dia escolhido, com a regra de ouro por slot: um horário
 * que o motor barra vira pill riscada NÃO-clicável e o paciente lê o porquê (o
 * `reason` pronto do servidor). Motivos repetidos entre vários horários aparecem
 * UMA vez acima da grade (dedupe); os exclusivos de um horário, abaixo dele.
 */
export function TimeStep({ slots, intencaoSlotId, emVoo, onEscolher }: TimeStepProps) {
  const primeiro = slots[0];
  const dia = primeiro ? formatDateLong(primeiro.starts_at, primeiro.time_zone) : '';
  const duracao = primeiro ? duracaoMin(primeiro) : 0;
  const header = duracao > 0 ? `Sessões de ${duracao} minutos, por vídeo · ${dia}` : `Por vídeo · ${dia}`;

  const { comuns, ehComum } = particionarBloqueios(slots);

  return (
    <section className="flex flex-col gap-5">
      <header className="flex flex-col gap-1">
        <h2 className="text-xl font-bold text-primary-300">Escolha o horário</h2>
        <p className="text-sm text-muted first-letter:uppercase">{header}</p>
      </header>

      {/* Motivos compartilhados por vários horários: exibidos uma única vez. */}
      {comuns.length > 0 && (
        <EligibilityNotice compact blocks={comuns} timeZone={primeiro?.time_zone} />
      )}

      <div className="flex flex-wrap gap-3">
        {slots.map((s) => {
          const permitido = s.eligibility.allowed;
          const selecionado = s.id === intencaoSlotId;
          const hora = formatTime(s.starts_at, s.time_zone);
          const unicos = permitido ? [] : s.eligibility.blocks.filter((b) => !ehComum(b));

          return (
            <div key={s.id} className="flex min-w-0 flex-col gap-1.5">
              <button
                type="button"
                aria-pressed={permitido ? selecionado : undefined}
                aria-label={rotulo(hora, permitido, selecionado)}
                disabled={!permitido || emVoo}
                onClick={() => onEscolher(s)}
                className={pill(permitido, selecionado)}
              >
                {hora}
              </button>
              {unicos.length > 0 && (
                <EligibilityNotice compact blocks={unicos} timeZone={s.time_zone} />
              )}
            </div>
          );
        })}
      </div>
    </section>
  );
}

/** Duração do slot em minutos — só um delta de instantes, sem fuso envolvido. */
function duracaoMin(slot: AnnotatedSlot): number {
  const min = Math.round(
    (new Date(slot.ends_at).getTime() - new Date(slot.starts_at).getTime()) / 60000,
  );
  return Number.isFinite(min) && min > 0 ? min : 0;
}

function rotulo(hora: string, permitido: boolean, selecionado: boolean): string {
  if (!permitido) return `Horário das ${hora}, indisponível`;
  if (selecionado) return `Horário das ${hora}, selecionado`;
  return `Horário das ${hora}`;
}

function pill(permitido: boolean, selecionado: boolean): string {
  const base =
    'rounded-lg border px-4 py-3 text-sm font-bold transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-300';
  if (!permitido) {
    return cx(base, 'cursor-not-allowed border-primary-100 bg-white text-muted line-through');
  }
  if (selecionado) {
    return cx(base, 'border-primary-300 bg-primary-300 text-white disabled:cursor-not-allowed disabled:opacity-70');
  }
  return cx(
    base,
    'border-primary-200 bg-white text-primary-300 hover:border-primary-300 disabled:cursor-not-allowed disabled:opacity-50',
  );
}

/** Chave de identidade de um bloco, para contar repetições entre horários. */
function chave(b: EligibilityBlock): string {
  return `${b.rule_type}|${b.reason}|${b.available_from ?? ''}`;
}

/**
 * Separa os bloqueios do dia: `comuns` (o mesmo motivo em ≥2 horários, deduplicado)
 * vai uma vez acima da grade; o resto é exclusivo de um horário. `ehComum` deixa
 * cada pill filtrar da sua lista o que já subiu para o topo.
 */
function particionarBloqueios(slots: AnnotatedSlot[]): {
  comuns: EligibilityBlock[];
  ehComum: (b: EligibilityBlock) => boolean;
} {
  const contagem = new Map<string, number>();
  for (const s of slots) {
    if (s.eligibility.allowed) continue;
    for (const b of s.eligibility.blocks) {
      const k = chave(b);
      contagem.set(k, (contagem.get(k) ?? 0) + 1);
    }
  }

  const ehComum = (b: EligibilityBlock) => (contagem.get(chave(b)) ?? 0) >= 2;

  const comuns: EligibilityBlock[] = [];
  const vistos = new Set<string>();
  for (const s of slots) {
    if (s.eligibility.allowed) continue;
    for (const b of s.eligibility.blocks) {
      const k = chave(b);
      if (ehComum(b) && !vistos.has(k)) {
        vistos.add(k);
        comuns.push(b);
      }
    }
  }

  return { comuns, ehComum };
}
