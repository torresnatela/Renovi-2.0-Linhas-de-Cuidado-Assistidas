import { describe, expect, it } from 'vitest';

import { dateBrToIso, digitsOnly, maskCep, maskCpf, maskDate, maskPhone, maskUf } from './masks';

describe('digitsOnly', () => {
  it('mantém só os dígitos', () => {
    expect(digitsOnly('(11) 91234-5678')).toBe('11912345678');
    expect(digitsOnly('948.190.898-46')).toBe('94819089846');
    expect(digitsOnly('abc')).toBe('');
  });
});

describe('maskCpf', () => {
  it('formata progressivamente', () => {
    expect(maskCpf('948')).toBe('948');
    expect(maskCpf('9481')).toBe('948.1');
    expect(maskCpf('948190')).toBe('948.190');
    expect(maskCpf('9481908')).toBe('948.190.8');
    expect(maskCpf('94819089846')).toBe('948.190.898-46');
  });

  it('normaliza um valor já pontuado (colar)', () => {
    expect(maskCpf('948.190.898-46')).toBe('948.190.898-46');
  });

  it('descarta dígitos além de 11', () => {
    expect(maskCpf('9481908984612345')).toBe('948.190.898-46');
  });
});

describe('maskPhone', () => {
  it('formata progressivamente', () => {
    expect(maskPhone('11')).toBe('11');
    expect(maskPhone('119')).toBe('(11) 9');
    expect(maskPhone('1191234')).toBe('(11) 91234');
    expect(maskPhone('11912345678')).toBe('(11) 91234-5678');
  });

  it('limita a 11 dígitos', () => {
    expect(maskPhone('119123456789999')).toBe('(11) 91234-5678');
  });
});

describe('maskCep', () => {
  it('formata progressivamente e limita a 8 dígitos', () => {
    expect(maskCep('064')).toBe('064');
    expect(maskCep('06472')).toBe('06472');
    expect(maskCep('06472000')).toBe('06472-000');
    expect(maskCep('0647200099')).toBe('06472-000');
  });
});

describe('maskDate', () => {
  it('formata progressivamente dd/mm/aaaa', () => {
    expect(maskDate('2')).toBe('2');
    expect(maskDate('23')).toBe('23');
    expect(maskDate('2301')).toBe('23/01');
    expect(maskDate('23011976')).toBe('23/01/1976');
    expect(maskDate('230119761234')).toBe('23/01/1976');
  });
});

describe('maskUf', () => {
  it('só letras, maiúsculas, no máximo 2', () => {
    expect(maskUf('sp')).toBe('SP');
    expect(maskUf('sp1')).toBe('SP');
    expect(maskUf('r j')).toBe('RJ');
    expect(maskUf('minas')).toBe('MI');
  });
});

describe('dateBrToIso', () => {
  it('converte dd/mm/aaaa em ISO', () => {
    expect(dateBrToIso('23/07/1992')).toBe('1992-07-23');
    expect(dateBrToIso('01/01/2000')).toBe('2000-01-01');
  });

  it('devolve null para data incompleta ou inválida', () => {
    expect(dateBrToIso('23/07')).toBeNull();
    expect(dateBrToIso('23/07/92')).toBeNull();
    expect(dateBrToIso('31/02/2000')).toBeNull();
    expect(dateBrToIso('00/13/2020')).toBeNull();
    expect(dateBrToIso('')).toBeNull();
  });
});
