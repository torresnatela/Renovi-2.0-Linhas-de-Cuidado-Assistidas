import { afterEach, describe, expect, it, vi } from 'vitest';

import { lookupCep } from './viaCep';

afterEach(() => {
  vi.restoreAllMocks();
});

function mockFetch(impl: () => Promise<unknown> | never) {
  vi.stubGlobal('fetch', vi.fn(impl));
}

describe('lookupCep', () => {
  it('mapeia a resposta da ViaCEP', async () => {
    mockFetch(async () => ({
      ok: true,
      json: async () => ({
        logradouro: 'Avenida Copacabana',
        bairro: 'Dezoito do Forte',
        localidade: 'Barueri',
        uf: 'SP',
      }),
    }));

    await expect(lookupCep('06472-000')).resolves.toEqual({
      street: 'Avenida Copacabana',
      neighborhood: 'Dezoito do Forte',
      city: 'Barueri',
      state: 'SP',
    });
  });

  it('devolve null quando o CEP não existe ({ erro: true })', async () => {
    mockFetch(async () => ({ ok: true, json: async () => ({ erro: true }) }));
    await expect(lookupCep('00000-000')).resolves.toBeNull();
  });

  it('devolve null em falha de rede — silencioso', async () => {
    mockFetch(async () => {
      throw new Error('offline');
    });
    await expect(lookupCep('06472000')).resolves.toBeNull();
  });

  it('nem chama a rede se o CEP não tem 8 dígitos', async () => {
    const fetchSpy = vi.fn();
    vi.stubGlobal('fetch', fetchSpy);
    await expect(lookupCep('0647')).resolves.toBeNull();
    expect(fetchSpy).not.toHaveBeenCalled();
  });
});
