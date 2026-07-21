import { describe, expect, it } from 'vitest';

import {
  dayKey,
  dayOfMonth,
  formatDate,
  formatDateTimeShort,
  formatTime,
  FUSO_PADRAO,
  monthAbbrev,
  monthKey,
  monthLabel,
} from './datetime';

// 09:00 em São Paulo. O mesmo INSTANTE é meio-dia em UTC e 13:00 em Lisboa.
const noveDaManhaEmSP = '2026-07-20T09:00:00-03:00';

describe('formatTime', () => {
  /**
   * O teste que sustenta a feature inteira.
   *
   * As duas asserções juntas são o ponto: uma implementação que IGNORE o
   * parâmetro `timeZone` (um `toLocaleTimeString()` seco) passa na primeira e
   * quebra na segunda. E como as duas cravam o fuso, o teste vale o mesmo em
   * qualquer máquina — inclusive no runner de CI, que roda em UTC.
   */
  it('formata no fuso pedido, não no do ambiente', () => {
    expect(formatTime(noveDaManhaEmSP, 'America/Sao_Paulo')).toBe('09:00');
    expect(formatTime(noveDaManhaEmSP, 'Europe/Lisbon')).toBe('13:00');
  });

  // O legado guarda DATETIME ingênuo; se alguém "consertar" a API para mandar Z
  // sem converter, a consulta anda 3 horas. Este é o formato certo.
  it('entende o offset explícito do RFC 3339', () => {
    expect(formatTime('2026-07-20T12:00:00Z', 'America/Sao_Paulo')).toBe('09:00');
  });
});

describe('dayKey', () => {
  it('agrupa pelo dia DO FUSO da agenda, não pelo do browser', () => {
    // 23:00 de 20/07 em São Paulo já é 21/07 em UTC. Agrupar pelo fuso errado
    // jogaria este horário no balde do dia seguinte.
    const tarde = '2026-07-20T23:00:00-03:00';
    expect(dayKey(tarde, 'America/Sao_Paulo')).toBe('2026-07-20');
    expect(dayKey(tarde, 'UTC')).toBe('2026-07-21');
  });
});

describe('formatDateTimeShort', () => {
  it('mostra dia e hora no fuso da agenda', () => {
    expect(formatDateTimeShort(noveDaManhaEmSP, 'America/Sao_Paulo')).toBe('20/07 às 09:00');
  });
});

// 23:30 de 31/07 em São Paulo já é 01/08 (outro dia E outro mês) em UTC. É a
// prova de que os helpers de selo de data leem no fuso da agenda, não no do
// ambiente — no runner de CI (UTC) o dia e o mês virariam os errados.
const fimDeJulhoEmSP = '2026-07-31T23:30:00-03:00';

describe('formatDate', () => {
  it('formata dd/MM/yyyy no fuso pedido', () => {
    expect(formatDate('2026-09-30T00:00:00-03:00', 'America/Sao_Paulo')).toBe('30/09/2026');
  });

  it('usa o fuso da agenda, não o do ambiente', () => {
    expect(formatDate(fimDeJulhoEmSP, 'America/Sao_Paulo')).toBe('31/07/2026');
    expect(formatDate(fimDeJulhoEmSP, 'UTC')).toBe('01/08/2026');
  });
});

describe('monthAbbrev', () => {
  it('dá o mês abreviado sem ponto, no fuso da agenda', () => {
    expect(monthAbbrev(fimDeJulhoEmSP, 'America/Sao_Paulo')).toBe('jul');
    // Em UTC o mesmo instante já é agosto.
    expect(monthAbbrev(fimDeJulhoEmSP, 'UTC')).toBe('ago');
  });
});

describe('dayOfMonth', () => {
  it('dá o dia do mês no fuso da agenda', () => {
    expect(dayOfMonth(fimDeJulhoEmSP, 'America/Sao_Paulo')).toBe('31');
    expect(dayOfMonth(fimDeJulhoEmSP, 'UTC')).toBe('01');
  });
});

describe('monthKey', () => {
  it('agrupa pelo MÊS do fuso da agenda, não pelo do ambiente', () => {
    // 00:30 UTC de 01/08 ainda é 31/07 (à noite) em São Paulo — logo, mês de
    // JULHO. Agrupar pelo mês do browser (UTC) jogaria a consulta no balde de
    // agosto e ela sumiria do grupo certo.
    const viraMes = '2026-08-01T00:30:00Z';
    expect(monthKey(viraMes, 'America/Sao_Paulo')).toBe('2026-07');
    expect(monthKey(viraMes, 'UTC')).toBe('2026-08');
  });

  it('dá o mês do instante no fuso pedido', () => {
    expect(monthKey(noveDaManhaEmSP, 'America/Sao_Paulo')).toBe('2026-07');
  });
});

describe('monthLabel', () => {
  it('rótulo PT-BR capitalizado, no fuso da agenda', () => {
    expect(monthLabel(noveDaManhaEmSP, 'America/Sao_Paulo')).toBe('Julho de 2026');
    // A mesma virada de mês: em SP ainda é julho; em UTC já é agosto.
    expect(monthLabel('2026-08-01T00:30:00Z', 'America/Sao_Paulo')).toBe('Julho de 2026');
    expect(monthLabel('2026-08-01T00:30:00Z', 'UTC')).toBe('Agosto de 2026');
  });
});

describe('FUSO_PADRAO', () => {
  // Mora aqui (e não em useJourney) porque é o fuso de LEITURA de todo instante
  // sem time_zone próprio — vigência, eventos, available_from. useJourney apenas
  // o re-exporta para os imports antigos não quebrarem.
  it('é o fuso de parede da plataforma', () => {
    expect(FUSO_PADRAO).toBe('America/Sao_Paulo');
  });
});
