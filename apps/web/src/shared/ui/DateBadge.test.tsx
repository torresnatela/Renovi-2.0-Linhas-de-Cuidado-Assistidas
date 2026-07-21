import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { DateBadge } from './DateBadge';

describe('DateBadge', () => {
  it('mostra mês abreviado e dia no fuso pedido', () => {
    render(<DateBadge iso="2026-07-23T10:00:00-03:00" timeZone="America/Sao_Paulo" />);
    // "JUL" é caixa-alta do CSS; o texto-fonte é "jul".
    expect(screen.getByText('jul')).toBeInTheDocument();
    expect(screen.getByText('23')).toBeInTheDocument();
  });

  /**
   * O horário das 23:30 em SP já é o dia seguinte em UTC. O selo tem que ler o dia
   * NO fuso da agenda — senão a consulta apareceria no dia errado.
   */
  it('não pula de dia por causa do UTC (23:30-03:00 continua dia 23)', () => {
    render(<DateBadge iso="2026-07-23T23:30:00-03:00" timeZone="America/Sao_Paulo" />);
    expect(screen.getByText('23')).toBeInTheDocument();
    expect(screen.queryByText('24')).not.toBeInTheDocument();
  });

  it('respeita o timeZone recebido (o mesmo instante vira dia 24 em UTC)', () => {
    render(<DateBadge iso="2026-07-23T23:30:00-03:00" timeZone="UTC" />);
    expect(screen.getByText('24')).toBeInTheDocument();
  });
});
