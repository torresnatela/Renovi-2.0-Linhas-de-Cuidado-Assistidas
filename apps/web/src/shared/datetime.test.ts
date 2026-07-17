import { describe, expect, it } from 'vitest';

import { dayKey, formatDateTimeShort, formatTime } from './datetime';

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
