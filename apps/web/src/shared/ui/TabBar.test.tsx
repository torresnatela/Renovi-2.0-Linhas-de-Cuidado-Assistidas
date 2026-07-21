import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it } from 'vitest';

import { TabBar } from './TabBar';

function renderTabBar(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <TabBar />
    </MemoryRouter>,
  );
}

describe('TabBar', () => {
  it('expõe as três abas com nomes acessíveis e destinos', () => {
    renderTabBar('/jornada');
    expect(screen.getByRole('navigation', { name: 'Principal' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Jornada' })).toHaveAttribute('href', '/jornada');
    expect(screen.getByRole('link', { name: 'Consultas' })).toHaveAttribute('href', '/consultas');
    expect(screen.getByRole('link', { name: 'Perfil' })).toHaveAttribute('href', '/perfil');
  });

  it('marca a aba da rota atual com aria-current="page"', () => {
    renderTabBar('/consultas');
    expect(screen.getByRole('link', { name: 'Consultas' })).toHaveAttribute('aria-current', 'page');
    expect(screen.getByRole('link', { name: 'Jornada' })).not.toHaveAttribute('aria-current');
    expect(screen.getByRole('link', { name: 'Perfil' })).not.toHaveAttribute('aria-current');
  });

  // O par outline/filled sinaliza a aba ativa sem depender só de cor. Os ícones
  // filled nascem no viewBox nativo 21×21 do handoff; os outline no grid 24 do DS
  // — é essa diferença de viewBox que a asserção usa (menos frágil que snapshot).
  it('usa o ícone filled na aba ativa e outline nas inativas', () => {
    renderTabBar('/jornada');
    expect(screen.getByRole('link', { name: 'Jornada' }).querySelector('svg')).toHaveAttribute(
      'viewBox',
      '0 0 21 21',
    );
    expect(screen.getByRole('link', { name: 'Consultas' }).querySelector('svg')).toHaveAttribute(
      'viewBox',
      '0 0 24 24',
    );
    expect(screen.getByRole('link', { name: 'Perfil' }).querySelector('svg')).toHaveAttribute(
      'viewBox',
      '0 0 24 24',
    );
  });
});
