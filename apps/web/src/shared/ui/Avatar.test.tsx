import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { Avatar } from './Avatar';

describe('Avatar', () => {
  it('deriva as iniciais do primeiro e do último nome', () => {
    render(<Avatar name="Ana Beatriz" />);
    expect(screen.getByText('AB')).toBeInTheDocument();
  });

  it('usa uma única inicial para nome sem sobrenome', () => {
    render(<Avatar name="Ana" />);
    expect(screen.getByText('A')).toBeInTheDocument();
  });

  it('ignora nomes do meio, mantendo primeiro + último', () => {
    render(<Avatar name="Ana Paula Souza" />);
    expect(screen.getByText('AS')).toBeInTheDocument();
  });

  it('expõe o nome como rótulo acessível', () => {
    render(<Avatar name="Ana Beatriz" />);
    expect(screen.getByRole('img', { name: 'Ana Beatriz' })).toBeInTheDocument();
  });
});
