import type { Reason } from '../../shared/api';

/**
 * Traduz o `reason.code` da API em frase para o paciente.
 *
 * É a tabela que a regra de ouro do CLAUDE.md exige: nunca um botão só
 * desabilitado, sempre o motivo. E é UMA tabela só — o mesmo `Reason` serve aos
 * erros do agendamento e (quando existir) aos vereditos do motor de
 * elegibilidade.
 *
 * O código vem separado do texto de propósito: "cedo demais" e "horário tomado"
 * são os dois 409, e a tela reage diferente a cada um. Se casássemos pela frase
 * da API, o front quebraria no dia em que alguém melhorasse o texto.
 */
const FRASES: Record<string, string> = {
  // Agendamento
  SLOT_TAKEN: 'Este horário acabou de ser reservado por outra pessoa. Escolha outro.',
  SLOT_EXPIRED: 'Este horário já passou. Escolha outro.',
  BOOKING_UNCONFIRMED:
    'Não conseguimos confirmar sua consulta com a Doutor ao Vivo a tempo. Ela pode ter sido marcada — veja em Minhas consultas.',

  // Janela de entrada
  JOIN_TOO_EARLY: 'Ainda não está na hora de entrar nesta consulta.',
  JOIN_TOO_LATE: 'Esta consulta já terminou.',
  JOIN_CANCELLED: 'Esta consulta foi cancelada.',
  JOIN_UNAVAILABLE:
    'Ainda estamos confirmando esta consulta com a Doutor ao Vivo. Assim que confirmarmos, o botão de entrar aparece aqui.',
};

/**
 * `fallback` é obrigatório: um código que ainda não conhecemos não pode virar
 * tela em branco nem "undefined". A API manda o `detail` justamente para isso —
 * ele é escrito pensando no paciente.
 */
export function reasonText(reason: Reason | undefined, fallback: string): string {
  if (!reason) return fallback;
  return FRASES[reason.code] ?? fallback;
}
