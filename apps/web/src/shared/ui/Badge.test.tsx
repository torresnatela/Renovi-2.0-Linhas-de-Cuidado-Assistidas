import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { Badge } from './Badge';

describe('Badge', () => {
  it('mostra o texto curto', () => {
    render(<Badge tone="success">Plano ativo</Badge>);
    expect(screen.getByText('Plano ativo')).toBeInTheDocument();
  });

  it('tone success usa o verde do DS', () => {
    render(<Badge tone="success">Feito</Badge>);
    expect(screen.getByText('Feito').className).toContain('text-success');
  });

  it('tone accent usa o laranja do DS', () => {
    render(<Badge tone="accent">Hoje</Badge>);
    expect(screen.getByText('Hoje').className).toContain('text-accent-300');
  });

  it('é sempre uma pill', () => {
    render(<Badge tone="neutral">Rascunho</Badge>);
    expect(screen.getByText('Rascunho').className).toContain('rounded-pill');
  });
});
