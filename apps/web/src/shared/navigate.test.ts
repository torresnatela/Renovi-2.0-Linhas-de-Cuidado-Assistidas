import { afterEach, describe, expect, it, vi } from 'vitest';

import { openExternal } from './navigate';

// jsdom não deixa espionar window.location.replace direto; substituímos por um
// stub e restauramos depois.
const original = window.location;

afterEach(() => {
  Object.defineProperty(window, 'location', { value: original, configurable: true });
});

function stubLocation() {
  const replace = vi.fn();
  Object.defineProperty(window, 'location', {
    value: { ...original, replace },
    configurable: true,
  });
  return replace;
}

describe('openExternal', () => {
  it('navega para uma url https', () => {
    const replace = stubLocation();
    openExternal('https://renovisaude.atendimento.hom.dav.med.br/a/abc');
    expect(replace).toHaveBeenCalledWith('https://renovisaude.atendimento.hom.dav.med.br/a/abc');
  });

  // Defesa em profundidade: um valor javascript: executaria na nossa origem.
  it('recusa esquema javascript e não navega', () => {
    const replace = stubLocation();
    expect(() => openExternal('javascript:alert(1)')).toThrow();
    expect(replace).not.toHaveBeenCalled();
  });

  it('recusa http (não-https) e não navega', () => {
    const replace = stubLocation();
    expect(() => openExternal('http://exemplo.com/a/abc')).toThrow();
    expect(replace).not.toHaveBeenCalled();
  });

  it('recusa url malformada', () => {
    const replace = stubLocation();
    expect(() => openExternal('não é url')).toThrow();
    expect(replace).not.toHaveBeenCalled();
  });
});
