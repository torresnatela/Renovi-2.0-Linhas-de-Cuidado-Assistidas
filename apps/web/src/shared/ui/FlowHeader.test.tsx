import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';

import { FlowHeader } from './FlowHeader';

function renderFlow(ui: React.ReactNode) {
  return render(<MemoryRouter>{ui}</MemoryRouter>);
}

describe('FlowHeader', () => {
  it('renderiza eyebrow e título', () => {
    renderFlow(<FlowHeader eyebrow="Agendar · Psicologia" title="Com quem?" backTo="/jornada" />);
    expect(screen.getByText('Agendar · Psicologia')).toBeInTheDocument();
    expect(screen.getByText('Com quem?')).toBeInTheDocument();
  });

  it('com backTo vira um link "Voltar" para o destino', () => {
    renderFlow(<FlowHeader eyebrow="Agendar" title="Com quem?" backTo="/jornada" />);
    const back = screen.getByRole('link', { name: 'Voltar' });
    expect(back).toHaveAttribute('href', '/jornada');
    expect(screen.queryByRole('button', { name: 'Voltar' })).not.toBeInTheDocument();
  });

  it('com onBack vira um button "Voltar" que dispara o callback', async () => {
    const onBack = vi.fn();
    const user = userEvent.setup();
    renderFlow(<FlowHeader eyebrow="Agendar" title="Com quem?" onBack={onBack} />);
    const back = screen.getByRole('button', { name: 'Voltar' });
    expect(screen.queryByRole('link', { name: 'Voltar' })).not.toBeInTheDocument();
    await user.click(back);
    expect(onBack).toHaveBeenCalledTimes(1);
  });

  it('renderiza o progresso com caption e largura pct%', () => {
    renderFlow(
      <FlowHeader
        eyebrow="Agendar"
        title="Com quem?"
        backTo="/jornada"
        progress={{ pct: 66, caption: 'Passo 2 de 3 · consulta 3 de 4 do mês' }}
      />,
    );
    expect(screen.getByText('Passo 2 de 3 · consulta 3 de 4 do mês')).toBeInTheDocument();
    const bar = screen.getByRole('progressbar');
    expect(bar).toHaveStyle({ width: '66%' });
    expect(bar).toHaveAttribute('aria-valuenow', '66');
  });

  it('sem progress não renderiza a barra', () => {
    renderFlow(<FlowHeader eyebrow="Agendar" title="Com quem?" backTo="/jornada" />);
    expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
  });

  it('renderiza o slot de ajuda quando fornecido', () => {
    renderFlow(
      <FlowHeader
        eyebrow="Agendar"
        title="Com quem?"
        backTo="/jornada"
        help={<span>ajuda-slot</span>}
      />,
    );
    expect(screen.getByText('ajuda-slot')).toBeInTheDocument();
  });
});
