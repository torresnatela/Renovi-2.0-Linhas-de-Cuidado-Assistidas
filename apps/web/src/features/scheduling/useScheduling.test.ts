import { describe, expect, it } from 'vitest';

import { proximoPoll } from './useScheduling';

const agora = new Date('2026-07-20T08:00:00-03:00').getTime();

describe('proximoPoll', () => {
  const LIMITE_TIMER = 2_147_483_647; // máximo de um setTimeout de 32 bits

  /**
   * O bug que este teste trava: uma consulta a 30 dias dá um delay de ~2,6e9 ms,
   * acima do limite do timer → setTimeout dispararia NA HORA, num laço apertado.
   * O teto de 24h tem que segurar isso ABAIXO do limite.
   */
  it('nunca passa do limite do timer de 32 bits, nem para consultas muito no futuro', () => {
    const daquiA30Dias = new Date(agora + 30 * 24 * 3600_000).toISOString();
    const delay = proximoPoll(daquiA30Dias, agora);
    expect(delay).toBeLessThan(LIMITE_TIMER);
    expect(delay).toBe(24 * 60 * 60 * 1000); // bateu no teto de 24h
  });

  it('dorme até pouco depois de abrir quando falta pouco', () => {
    const em10min = new Date(agora + 10 * 60_000).toISOString();
    expect(proximoPoll(em10min, agora)).toBe(10 * 60_000 + 1000);
  });

  it('respeita o piso de 15s se o cliente acha que já passou da abertura', () => {
    const jaPassou = new Date(agora - 5 * 60_000).toISOString();
    expect(proximoPoll(jaPassou, agora)).toBe(15_000);
  });
});
