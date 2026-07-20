import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';

import { HelpPill } from './HelpPill';

describe('HelpPill', () => {
  it('é um botão com o nome acessível "Pedir ajuda"', () => {
    render(<HelpPill onClick={() => {}} />);
    expect(screen.getByRole('button', { name: 'Pedir ajuda' })).toBeInTheDocument();
  });

  it('clicar dispara onClick', async () => {
    const onClick = vi.fn();
    const user = userEvent.setup();
    render(<HelpPill onClick={onClick} />);
    await user.click(screen.getByRole('button', { name: 'Pedir ajuda' }));
    expect(onClick).toHaveBeenCalledOnce();
  });

  it('busy: desabilita e mostra o spinner, mantendo o rótulo', async () => {
    const onClick = vi.fn();
    const user = userEvent.setup();
    render(<HelpPill onClick={onClick} busy />);
    const botao = screen.getByRole('button', { name: 'Pedir ajuda' });
    expect(botao).toBeDisabled();
    expect(screen.getByTestId('help-spinner')).toBeInTheDocument();
    // O rótulo permanece — nada de spinner mudo.
    expect(screen.getByText('Pedir ajuda')).toBeInTheDocument();
    await user.click(botao);
    expect(onClick).not.toHaveBeenCalled();
  });

  it('sem busy não mostra spinner', () => {
    render(<HelpPill onClick={() => {}} />);
    expect(screen.queryByTestId('help-spinner')).not.toBeInTheDocument();
  });
});
