import type {
  AssessmentCode,
  CareLineItemInfo,
  CareAppointment,
  JourneyEnrollment,
  JourneyItem,
  MoodToday,
} from '../../shared/api';

/**
 * Derivações de EXIBIÇÃO da Jornada — puras e sem regra de negócio. O servidor
 * decide o que vale (status, elegibilidade, vigência); estas funções só escolhem o
 * que mostrar e como. Ficam à parte para serem simples de ler e de testar.
 */

/** Trata por "você" + primeiro nome (DESIGN-SYSTEM §4.7). */
export function primeiroNome(nomeCompleto: string | undefined): string {
  if (!nomeCompleto) return '';
  return nomeCompleto.trim().split(/\s+/)[0] ?? '';
}

/**
 * A próxima consulta FUTURA já marcada (agendada/confirmada), a mais próxima no
 * tempo. É filtro de exibição — o status vem pronto do servidor; aqui só ordenamos
 * e cortamos o passado pelo relógio.
 */
export function proximaConsulta(appointments: CareAppointment[]): CareAppointment | null {
  const agora = Date.now();
  const futuras = appointments
    .filter(
      (a) =>
        (a.status === 'agendada' || a.status === 'confirmada') &&
        new Date(a.scheduled_at).getTime() > agora,
    )
    .sort((a, b) => new Date(a.scheduled_at).getTime() - new Date(b.scheduled_at).getTime());
  return futuras[0] ?? null;
}

/**
 * Falta menos de 30 dias para a vigência acabar? Decide apenas se mostramos o
 * aviso de renovação no aside — NÃO é regra (o servidor é quem invalida o plano).
 */
export function vigenciaPertoDoFim(validUntil: string): boolean {
  const TRINTA_DIAS = 30 * 24 * 60 * 60 * 1000;
  const restante = new Date(validUntil).getTime() - Date.now();
  return restante > 0 && restante < TRINTA_DIAS;
}

/**
 * Uma frase simples de resumo do dia, derivada só do que já carregamos: se há item
 * liberado para agendar e/ou check-in de humor pendente. Sem cobrança nem alarme.
 */
export function resumoDoDia(temItemLiberado: boolean, checkinPendente: boolean): string {
  const partes: string[] = [];
  if (temItemLiberado) partes.push('consultas para agendar');
  if (checkinPendente) partes.push('o check-in de hoje');
  if (partes.length === 0) {
    return 'Tudo em dia por hoje. A gente te avisa quando algo precisar de você.';
  }
  return `Você tem ${partes.join(' e ')}.`;
}

// ---------------------------------------------------------------------------
// Itens ATIVIDADE (Anexo C) — nunca agendáveis
// ---------------------------------------------------------------------------

/**
 * `ref`s de `kind: 'ATIVIDADE'` que o front sabe tratar. Não há endpoint que os
 * exponha — são as MESMAS constantes do backend, espelhadas aqui (mapear é
 * legítimo; comentado com a origem de cada uma):
 *   - `checkin-humor-diario` → apps/api/internal/models/mood_checkin.go, `CheckinHumorDiarioRef`
 *   - `who5-semanal`         → apps/api/internal/models/assessment.go,  `Who5ItemRef`
 *   - `phq4-gatilhado`       → apps/api/internal/models/assessment.go,  `Phq4ItemRef`
 * Um ref fora deste mapa (linha futura) fica sem ação: só título/legenda.
 */
const REF_CHECKIN_HUMOR_DIARIO = 'checkin-humor-diario';
const REF_WHO5_SEMANAL = 'who5-semanal';
const REF_PHQ4_GATILHADO = 'phq4-gatilhado';

/**
 * `care_line_code` da linha de cuidado ABERTA de saúde mental (Degrau 1, ADR-040) —
 * a MESMA constante do backend, espelhada aqui no mesmo padrão dos refs acima:
 *   - `saude-mental-aberta` → apps/api/internal/models/universal_enrollment.go, `UniversalMentalHealthCode`
 * É a única linha onde o front SINTETIZA o nó de check-in de humor na timeline (a
 * linha aberta não materializa o check-in como item de template).
 */
const CARE_LINE_CODE_SAUDE_MENTAL_ABERTA = 'saude-mental-aberta';

const OFERTA_POR_REF: Record<string, AssessmentCode> = {
  [REF_WHO5_SEMANAL]: 'WHO5',
  [REF_PHQ4_GATILHADO]: 'PHQ4',
};

/**
 * `kind === 'ATIVIDADE'` nomeado — um só lugar decide o que é agendável
 * (CONSULTA) vs. o que nunca é (ATIVIDADE: sem especialidade/slots), reusado
 * pela JourneyPage (resumo do dia) e pela JourneyTimeline (qual card montar).
 */
export function ehAtividade(item: Pick<CareLineItemInfo, 'kind'>): boolean {
  return item.kind === 'ATIVIDADE';
}

/** O estado de exibição de um passo ATIVIDADE na timeline. */
export type EstadoAtividade =
  | { tipo: 'feito_hoje' }
  | { tipo: 'checkin_pendente' }
  | { tipo: 'responder_agora'; codigo: AssessmentCode }
  | { tipo: 'ofertado_quando_fizer_sentido' }
  | { tipo: 'sem_acao' };

/**
 * Que estado uma ATIVIDADE está: deriva do check-in de hoje (`useMoodToday`,
 * já carregado pela página) — NUNCA da elegibilidade do motor. O motor avalia
 * a atividade como liberada por não ter regras de agendamento (ela não tem
 * especialidade nem slots) — "liberada" aqui não quer dizer "agendável".
 */
export function estadoAtividade(ref: string, mood: MoodToday | undefined): EstadoAtividade {
  if (ref === REF_CHECKIN_HUMOR_DIARIO) {
    return mood?.checkin ? { tipo: 'feito_hoje' } : { tipo: 'checkin_pendente' };
  }
  const codigo = OFERTA_POR_REF[ref];
  if (codigo) {
    return mood?.offer === codigo
      ? { tipo: 'responder_agora', codigo }
      : { tipo: 'ofertado_quando_fizer_sentido' };
  }
  return { tipo: 'sem_acao' };
}

/**
 * A lista de itens da linha ATIVA com um nó SINTÉTICO de check-in de humor
 * PREPOSTO — decisão de produto aprovada. A linha aberta de saúde mental não
 * traz o check-in como item de template, mas ele deve aparecer na jornada. O
 * pseudo-item é adicionado só quando TODAS valem:
 *  (a) há matrícula no humor (`mood` carregado e `reason !== 'not_enrolled'` —
 *      `consent_required` ainda conta como matriculado);
 *  (b) a linha ativa é a universal de saúde mental (`care_line_code` espelhado do
 *      backend em `CARE_LINE_CODE_SAUDE_MENTAL_ABERTA`);
 *  (c) a linha AINDA NÃO traz um item com o ref do check-in (dedupe — algumas
 *      linhas já o materializam).
 *
 * O nó flui pelo caminho EXISTENTE de ATIVIDADE (`ehAtividade`/`estadoAtividade`/
 * `AtividadeCard`): zero estado novo, mesma DOM nos dois viewports. Quando não
 * sintetiza, devolve o MESMO array (referência intacta).
 */
export function itensComCheckinSintetico(
  enrollment: JourneyEnrollment,
  mood: MoodToday | undefined,
): JourneyItem[] {
  const itens = enrollment.items;
  const matriculadoNoHumor = mood != null && mood.reason !== 'not_enrolled';
  const ehLinhaMental =
    enrollment.enrollment.care_line_code === CARE_LINE_CODE_SAUDE_MENTAL_ABERTA;
  const jaTemCheckin = itens.some((it) => it.item.ref === REF_CHECKIN_HUMOR_DIARIO);
  if (!matriculadoNoHumor || !ehLinhaMental || jaTemCheckin) return itens;

  // sort_order abaixo do menor item real → o nó abre a timeline (que reordena por
  // sort_order). Sem itens reais, um valor neutro basta.
  const menorSort = itens.length ? Math.min(...itens.map((it) => it.item.sort_order)) : 1;
  const sintetico: JourneyItem = {
    item: {
      id: `sintetico:${REF_CHECKIN_HUMOR_DIARIO}`,
      ref: REF_CHECKIN_HUMOR_DIARIO,
      kind: 'ATIVIDADE',
      // Sem especialidade nem slots — não é agendável (Anexo C).
      specialty_code: '',
      label: 'Check-in de humor',
      // Caption neutro e honesto, igual ao mock ("Diário · opcional").
      recurrence: 'Diário · opcional',
      sort_order: menorSort - 1,
    },
    // ATIVIDADE não usa elegibilidade para agir (o estado vem do check-in do dia);
    // `allowed` fica neutro só para satisfazer o shape de JourneyItem.
    eligibility: { allowed: true, blocks: [] },
  };
  return [sintetico, ...itens];
}
