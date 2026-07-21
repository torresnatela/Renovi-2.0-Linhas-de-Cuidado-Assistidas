import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';

import { ApiError } from '../api';
import { Empty, ErrorNotice, Loading } from './feedback';

describe('Loading', () => {
  it('anuncia o carregamento com role status', () => {
    render(<Loading label="Carregando horários" />);
    expect(screen.getByRole('status')).toHaveTextContent('Carregando horários');
  });

  it('tem um rótulo padrão', () => {
    render(<Loading />);
    expect(screen.getByRole('status')).toHaveTextContent(/carregando/i);
  });
});

describe('Empty', () => {
  it('mostra título e dica e não é um alerta', () => {
    render(<Empty title="Nada por aqui" hint="Volte mais tarde" />);
    expect(screen.getByText('Nada por aqui')).toBeInTheDocument();
    expect(screen.getByText('Volte mais tarde')).toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });
});

describe('ErrorNotice', () => {
  it('trata 503 como indisponibilidade temporária (tom informativo, não alerta)', () => {
    render(<ErrorNotice error={new ApiError(503, 'indisponível')} />);
    expect(screen.getByText(/tente de novo em instantes/i)).toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('trata reason LEGACY_UNAVAILABLE como indisponibilidade', () => {
    render(
      <ErrorNotice error={new ApiError(500, 'x', undefined, { code: 'LEGACY_UNAVAILABLE' })} />,
    );
    expect(screen.getByText(/tente de novo em instantes/i)).toBeInTheDocument();
  });

  it('erro genérico mostra a mensagem real num alerta', () => {
    render(<ErrorNotice error={new Error('Falha ao salvar')} />);
    expect(screen.getByRole('alert')).toHaveTextContent('Falha ao salvar');
  });

  it('retry dispara o callback', async () => {
    const retry = vi.fn();
    const user = userEvent.setup();
    render(<ErrorNotice error={new Error('x')} retry={retry} />);
    await user.click(screen.getByRole('button', { name: /tentar de novo/i }));
    expect(retry).toHaveBeenCalledTimes(1);
  });
});
