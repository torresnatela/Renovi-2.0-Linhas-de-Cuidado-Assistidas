import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';

import type { DiaResumo } from './DateStep';
import { CalendarGrid, addDays, dowOf } from './CalendarGrid';

function dia(key: string, inicio: string): DiaResumo {
  return { key, inicio, timeZone: 'America/Sao_Paulo' };
}

describe('aritmética civil pura (sem new Date do browser)', () => {
  it('dowOf trata a string dayKey como calendário civil (0=dom..6=sáb)', () => {
    // 20/07/2026 é uma segunda-feira; 19/07 é domingo; 25/07 é sábado.
    expect(dowOf('2026-07-19')).toBe(0); // domingo
    expect(dowOf('2026-07-20')).toBe(1); // segunda
    expect(dowOf('2026-07-25')).toBe(6); // sábado
  });

  it('addDays soma dias cruzando o limite do mês sem tocar em fuso', () => {
    expect(addDays('2026-07-20', 0)).toBe('2026-07-20');
    expect(addDays('2026-07-20', -1)).toBe('2026-07-19');
    expect(addDays('2026-07-31', 1)).toBe('2026-08-01');
    expect(addDays('2026-07-20', 27)).toBe('2026-08-16');
  });
});

describe('CalendarGrid', () => {
  it('rende exatamente os dias derivados: disponível vira botão, o resto span não-clicável', () => {
    render(<CalendarGrid dias={[dia('2026-07-20', '2026-07-20T09:00:00-03:00')]} onEscolher={vi.fn()} />);

    // 28 células (4 semanas de 7 dias), começando no domingo da semana do 1º dia.
    const disponivel = screen.getByRole('button', { name: /seg, 20\/07/i });
    expect(disponivel).toBeInTheDocument();

    // O dia sem slots (21/07) NÃO é botão — é um span inerte.
    expect(screen.queryByRole('button', { name: /21\/07/i })).not.toBeInTheDocument();
    expect(screen.getByText('21')).toBeInTheDocument();
  });

  /**
   * O CalendarGrid consome SÓ a string `dayKey` já derivada (`dias[].key`) — ele
   * nunca reparte o instante `starts_at`. Este teste fixa que a grade renderiza o
   * dia EXATO do dayKey (20/07) e mantém o seguinte (21/07) inerte, sem que o
   * `inicio` às 23:30 −03:00 no `dias` desloque nada: a grade não olha para ele.
   *
   * O caminho de risco de verdade — server-time (23:30 −03:00) → `dayKey` no fuso
   * da agenda, que num runner UTC poderia virar 21/07 — é a `derivarDias` da
   * página, coberto de ponta a ponta em `ScheduleCarePage.mobile.test.tsx`
   * ("não sofre off-by-one: slot −03:00 perto da meia-noite fica no dia certo").
   */
  it('rende o dia exato do dayKey, ignorando o horário do `inicio`', () => {
    render(
      <CalendarGrid
        dias={[dia('2026-07-20', '2026-07-20T23:30:00-03:00')]}
        onEscolher={vi.fn()}
      />,
    );

    expect(screen.getByRole('button', { name: /seg, 20\/07/i })).toBeInTheDocument();
    // O dia 21 (o que apareceria num cálculo em UTC) segue inerte.
    expect(screen.queryByRole('button', { name: /21\/07/i })).not.toBeInTheDocument();
  });

  it('o tap num dia disponível chama onEscolher com o dayKey', async () => {
    const user = userEvent.setup();
    const onEscolher = vi.fn();
    render(
      <CalendarGrid dias={[dia('2026-07-20', '2026-07-20T09:00:00-03:00')]} onEscolher={onEscolher} />,
    );

    await user.click(screen.getByRole('button', { name: /seg, 20\/07/i }));
    expect(onEscolher).toHaveBeenCalledWith('2026-07-20');
  });

  it('o dia selecionado ganha aria-pressed', () => {
    render(
      <CalendarGrid
        dias={[dia('2026-07-20', '2026-07-20T09:00:00-03:00')]}
        selecionado="2026-07-20"
        onEscolher={vi.fn()}
      />,
    );

    expect(screen.getByRole('button', { name: /seg, 20\/07/i })).toHaveAttribute(
      'aria-pressed',
      'true',
    );
  });

  it('rotula os meses cobertos pela grade', () => {
    render(<CalendarGrid dias={[dia('2026-07-20', '2026-07-20T09:00:00-03:00')]} onEscolher={vi.fn()} />);
    // A grade começa em 19/07 (dom) e cobre 4 semanas até 15/08.
    expect(screen.getByText('Julho – Agosto de 2026')).toBeInTheDocument();
  });
});
