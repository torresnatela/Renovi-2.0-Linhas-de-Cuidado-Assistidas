import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { Card } from './Card';

describe('Card', () => {
  it('renderiza numa superfície branca com borda e sombra do DS', () => {
    render(<Card>Conteúdo</Card>);
    const card = screen.getByText('Conteúdo');
    expect(card.className).toContain('bg-white');
    expect(card.className).toContain('border-primary-100');
    expect(card.className).toContain('shadow-card');
    expect(card.className).toContain('rounded-lg');
  });

  it('respeita o polimorfismo via `as`', () => {
    render(<Card as="section">Seção</Card>);
    expect(screen.getByText('Seção').tagName).toBe('SECTION');
  });

  it('usa o padding lg quando pedido', () => {
    render(<Card padding="lg">Grande</Card>);
    expect(screen.getByText('Grande').className).toContain('p-5');
  });
});
