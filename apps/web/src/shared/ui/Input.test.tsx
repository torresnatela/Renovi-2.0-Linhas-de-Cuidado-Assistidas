import { createRef } from 'react';
import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { Input } from './Input';

describe('Input', () => {
  it('associa a label ao campo', () => {
    render(<Input label="E-mail" />);
    expect(screen.getByLabelText('E-mail')).toBeInTheDocument();
  });

  it('sem erro, o campo não é aria-invalid', () => {
    render(<Input label="E-mail" />);
    expect(screen.getByLabelText('E-mail')).not.toHaveAttribute('aria-invalid', 'true');
  });

  it('com erro, liga aria-invalid e aria-describedby à mensagem', () => {
    render(<Input label="Senha" error="Senha muito curta" />);
    const field = screen.getByLabelText('Senha');
    expect(field).toHaveAttribute('aria-invalid', 'true');
    const msg = screen.getByText('Senha muito curta');
    expect(field.getAttribute('aria-describedby')).toContain(msg.id);
  });

  it('associa o hint via aria-describedby', () => {
    render(<Input label="CPF" hint="Só números" />);
    const field = screen.getByLabelText('CPF');
    const hint = screen.getByText('Só números');
    expect(field.getAttribute('aria-describedby')).toContain(hint.id);
  });

  it('com erro, esconde o hint e descreve só a mensagem de erro', () => {
    render(<Input label="Senha" hint="Mínimo de 8 caracteres" error="Senha muito curta" />);
    const field = screen.getByLabelText('Senha');
    expect(screen.queryByText('Mínimo de 8 caracteres')).not.toBeInTheDocument();
    const msg = screen.getByText('Senha muito curta');
    expect(field.getAttribute('aria-describedby')).toBe(msg.id);
  });

  it('encaminha a ref para o input', () => {
    const ref = createRef<HTMLInputElement>();
    render(<Input label="Nome" ref={ref} />);
    expect(ref.current).toBeInstanceOf(HTMLInputElement);
  });
});
