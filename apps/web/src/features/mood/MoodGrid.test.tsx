import { fireEvent, render, screen } from '@testing-library/react';
import { useState } from 'react';
import { describe, expect, it, vi } from 'vitest';

import { MoodGrid, type MoodPoint } from './MoodGrid';

const GRID_LABEL = 'Grade de humor: valência por energia';

/**
 * Wrapper CONTROLADO: a MoodGrid não guarda estado (o valor é do consumidor).
 * O harness segura o ponto e repassa cada mudança ao espião, para exercitar a
 * conversão de coordenada e a operação por teclado de forma isolada — as
 * intenções que a MoodPage.test (aposentada na Etapa 8) cobria pela tela inteira.
 */
function Harness({ onValue }: { onValue?: (p: MoodPoint) => void }) {
  const [value, setValue] = useState<MoodPoint | null>(null);
  return (
    <MoodGrid
      value={value}
      onChange={(p) => {
        setValue(p);
        onValue?.(p);
      }}
    />
  );
}

// jsdom devolve um rect zerado; injetamos um 200×200 para o cálculo de
// coordenadas fazer sentido (mesmo padrão dos testes de tela).
function stub200(el: HTMLElement) {
  el.getBoundingClientRect = () =>
    ({ left: 0, top: 0, width: 200, height: 200, right: 200, bottom: 200, x: 0, y: 0 }) as DOMRect;
}

describe('MoodGrid', () => {
  it('converte o clique na grade em valência/energia (topo = mais energia)', () => {
    const onValue = vi.fn();
    render(<Harness onValue={onValue} />);
    const grade = screen.getByRole('button', { name: GRID_LABEL });
    stub200(grade);

    // (150, 50) numa grade 200×200 => valência 75, energia 75 (topo = mais energia).
    fireEvent.click(grade, { clientX: 150, clientY: 50 });

    expect(onValue).toHaveBeenCalledWith({ valencia: 75, energia: 75 });
    expect(screen.getByTestId('mood-marker')).toBeInTheDocument();
  });

  it('opera por teclado e anuncia o ponto a leitores de tela (aria-live)', () => {
    render(<Harness />);
    const grade = screen.getByRole('button', { name: GRID_LABEL });
    grade.focus();

    // Do centro (50,50): uma seta à direita e uma para cima => (55, 55).
    fireEvent.keyDown(grade, { key: 'ArrowRight' });
    fireEvent.keyDown(grade, { key: 'ArrowUp' });

    expect(screen.getByTestId('mood-value')).toHaveTextContent(
      'valência 55 de 100, energia 55 de 100',
    );
  });
});
