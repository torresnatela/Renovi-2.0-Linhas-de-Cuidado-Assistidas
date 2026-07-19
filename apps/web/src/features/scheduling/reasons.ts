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
  // Não promete que o botão aparece "sozinho": um UNCONFIRMED (DAV_UNKNOWN)
  // depende de verificação humana e pode nem virar confirmável — a tela não faz
  // polling nesse estado (seria o laço de bateria de novo). Peça para o paciente
  // voltar, em vez de prometer atualização automática.
  JOIN_UNAVAILABLE:
    'Ainda estamos confirmando esta consulta com a Doutor ao Vivo. Volte a esta página em instantes.',

  // Jornada / linha de cuidado (rotas /me/*). Os TEXTOS dos `blocks` de
  // elegibilidade vêm prontos do servidor e NÃO passam por aqui — esta tabela é só
  // para `Reason.code` de erro. ELIGIBILITY_BLOCKED é a mensagem de recuo: quando
  // a tela consegue os blocks, mostra-os; sem eles, cai neste texto.
  ELIGIBILITY_BLOCKED: 'As regras da sua linha de cuidado não permitem agendar este horário agora.',
  IDEMPOTENCY_KEY_REQUIRED:
    'Não foi possível concluir o agendamento. Recarregue a página e tente de novo.',
  CANCEL_NOT_ALLOWED: 'Esta consulta não pode mais ser cancelada.',
  AUDIT_CURSOR_INVALID: 'Não foi possível carregar mais itens. Recarregue a página.',
  LEGACY_UNAVAILABLE:
    'Não conseguimos consultar a agenda agora. Isso é um problema nosso, não seu — tente de novo em instantes.',
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
