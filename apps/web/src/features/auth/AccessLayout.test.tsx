import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it } from 'vitest';

import { AccessLayout } from './AccessLayout';

function renderLayout() {
  return render(
    <MemoryRouter>
      <AccessLayout active="login">
        <p>conteúdo do formulário</p>
      </AccessLayout>
    </MemoryRouter>,
  );
}

describe('AccessLayout', () => {
  // O painel de marca (desktop, `lg:flex`) já tem seu próprio logo com
  // alt="Renovi" — ambos coexistem no DOM do jsdom (que não aplica `display`
  // de CSS). O logo mobile precisa ser puramente decorativo para não duplicar
  // o accessible name "Renovi" para leitores de tela.
  it('logo mobile é decorativo (alt vazio + aria-hidden) e não duplica o nome acessível "Renovi"', () => {
    renderLayout();

    expect(screen.getAllByRole('img', { name: 'Renovi' })).toHaveLength(1);

    const images = document.querySelectorAll('img');
    expect(images).toHaveLength(2);
    const mobileLogo = Array.from(images).find((img) => img.getAttribute('aria-hidden') === 'true');
    expect(mobileLogo).toBeDefined();
    expect(mobileLogo).toHaveAttribute('alt', '');
    expect(mobileLogo).toHaveClass('lg:hidden');
  });

  it('mostra a tagline institucional escondida no desktop (lg:hidden) no fim da coluna', () => {
    renderLayout();

    const taglines = screen.getAllByText(
      /Renovi — tecnologia a serviço da sua saúde e do seu bem-estar\./i,
    );
    const mobileTagline = taglines.find((el) => el.className.includes('lg:hidden'));
    expect(mobileTagline).toBeDefined();
    expect(mobileTagline).toHaveClass('text-xs', 'text-muted', 'text-center', 'lg:hidden');
  });
});
