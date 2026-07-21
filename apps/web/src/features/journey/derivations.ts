import type { CareAppointment } from '../../shared/api';

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
