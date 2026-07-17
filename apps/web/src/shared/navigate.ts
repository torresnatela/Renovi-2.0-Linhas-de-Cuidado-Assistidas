/**
 * Sai do app para a sala da teleconsulta.
 *
 * É `assign` e não `window.open`: só sabemos a url DEPOIS do POST /join, ou seja,
 * fora do gesto do usuário — e um pop-up aberto fora do gesto é bloqueado pelo
 * browser. Navegação na mesma aba não passa por bloqueador nenhum.
 *
 * E é um módulo, e não uma linha solta dentro do componente, porque é o que
 * permite `vi.mock` no teste: o jsdom não deixa espionar `window.location`.
 */
export function openExternal(url: string): void {
  window.location.assign(url);
}
