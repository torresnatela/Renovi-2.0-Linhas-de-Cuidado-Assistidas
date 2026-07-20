/**
 * Formatação de data/hora — SEMPRE no fuso que a API mandou, nunca no do browser.
 *
 * Por que isto existe como módulo, com `timeZone` OBRIGATÓRIO em toda função:
 *
 * A origem dos horários é um DATETIME sem fuso no MySQL legado, que significa
 * hora de parede de São Paulo. A API resolve isso para um instante e diz, em
 * `time_zone`, em que fuso ele deve ser lido. Se o front formatar no fuso do
 * BROWSER, um paciente viajando — ou um runner de CI em UTC — vê 12:00 para uma
 * consulta das 09:00. Sem erro, sem aviso, só errado, e só descoberto no dia da
 * consulta.
 *
 * `toLocaleTimeString()` sem `timeZone` usa o fuso do browser. Como esquecer o
 * parâmetro é fácil e o estrago é invisível, ele é obrigatório aqui — o
 * TypeScript recusa a chamada sem ele.
 */

/**
 * O fuso em que as datas SEM `time_zone` próprio são exibidas.
 *
 * Slots e consultas trazem o fuso da agenda embutido no dado (`time_zone`); já a
 * vigência da matrícula, os eventos e o `available_from` de um bloqueio são
 * instantes sem fuso anexo. A plataforma opera em hora de parede de São Paulo (o
 * legado é de lá), então é esse o fuso de LEITURA — explícito, nunca o do browser
 * (a mesma regra deste módulo: um runner de CI em UTC mentiria a data).
 */
export const FUSO_PADRAO = 'America/Sao_Paulo';

/** "09:00" */
export function formatTime(iso: string, timeZone: string): string {
  return new Date(iso).toLocaleTimeString('pt-BR', {
    timeZone,
    hour: '2-digit',
    minute: '2-digit',
  });
}

/** "segunda-feira, 20 de julho" */
export function formatDateLong(iso: string, timeZone: string): string {
  return new Date(iso).toLocaleDateString('pt-BR', {
    timeZone,
    weekday: 'long',
    day: 'numeric',
    month: 'long',
  });
}

/** "20/07 às 09:00" */
export function formatDateTimeShort(iso: string, timeZone: string): string {
  const dia = new Date(iso).toLocaleDateString('pt-BR', {
    timeZone,
    day: '2-digit',
    month: '2-digit',
  });
  return `${dia} às ${formatTime(iso, timeZone)}`;
}

/** "30/09/2026" — data numérica com ano (ex.: vigência do plano). */
export function formatDate(iso: string, timeZone: string): string {
  return new Date(iso).toLocaleDateString('pt-BR', {
    timeZone,
    day: '2-digit',
    month: '2-digit',
    year: 'numeric',
  });
}

/**
 * "jul" — mês abreviado (sem o ponto que o pt-BR anexa), para o selo de data.
 *
 * O caixa-alta ("JUL") é do CSS, não da fonte: a string aqui é minúscula para o
 * nome acessível continuar legível.
 */
export function monthAbbrev(iso: string, timeZone: string): string {
  return new Date(iso)
    .toLocaleDateString('pt-BR', { timeZone, month: 'short' })
    .replace(/\.$/, '');
}

/** "23" — o dia do mês, no fuso da agenda, para o selo de data. */
export function dayOfMonth(iso: string, timeZone: string): string {
  return new Date(iso).toLocaleDateString('pt-BR', { timeZone, day: '2-digit' });
}

/**
 * "2026-07-20" — a chave para agrupar horários por dia.
 *
 * Tem que ser calculada NO fuso da agenda: agrupar pelo dia do browser jogaria o
 * slot das 23:00 de segunda no balde de terça para quem estiver a leste.
 */
export function dayKey(iso: string, timeZone: string): string {
  // en-CA dá o formato ISO (AAAA-MM-DD) já no fuso pedido, sem precisar remontar
  // a data à mão.
  return new Date(iso).toLocaleDateString('en-CA', { timeZone });
}
