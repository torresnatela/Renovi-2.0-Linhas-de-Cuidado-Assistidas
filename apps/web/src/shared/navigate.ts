/**
 * Sai do app para a sala da teleconsulta.
 *
 * `replace`, e não `assign`: o link é uma CREDENCIAL, e `assign` o gravaria no
 * histórico do browser, de onde qualquer um com o aparelho o recupera depois.
 * `replace` troca a entrada atual (o detalhe da consulta) pela sala, sem deixar o
 * link no histórico. Custo aceito: o "voltar" da sala não retorna ao detalhe —
 * quem está entrando numa consulta não deveria estar quicando de volta mesmo.
 *
 * E é `replace`, não `window.open`: só sabemos a url DEPOIS do POST /join, fora
 * do gesto do usuário, e pop-up fora do gesto é bloqueado. Navegação na mesma aba
 * não passa por bloqueador.
 *
 * É um módulo, e não uma linha solta no componente, porque é o que permite
 * `vi.mock` no teste: o jsdom não deixa espionar `window.location`.
 */
export function openExternal(url: string): void {
  window.location.replace(url);
}
