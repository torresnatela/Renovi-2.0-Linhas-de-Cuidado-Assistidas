// Máscaras puras de formulário (formatação progressiva) + conversores.
//
// São funções puras, sem React: cada `maskX` recebe o valor cru do input (o que o
// usuário digitou ou colou) e devolve a versão formatada e LIMITADA. A borda é o
// que importa — colar "948.190.898-46" ou digitar dígito a dígito tem que chegar
// no mesmo lugar. O comportamento espelha o script de referência do handoff
// (design_files/Acesso Desktop.dc.html).

/** Só os dígitos de uma string. `(11) 91234-5678` → `11912345678`. */
export function digitsOnly(value: string): string {
  return value.replace(/\D/g, '');
}

const digits = (value: string, max: number) => digitsOnly(value).slice(0, max);

/** CPF progressivo, limitado a 11 dígitos: `000.000.000-00`. */
export function maskCpf(value: string): string {
  return digits(value, 11)
    .replace(/(\d{3})(\d)/, '$1.$2')
    .replace(/(\d{3})\.(\d{3})(\d)/, '$1.$2.$3')
    .replace(/\.(\d{3})(\d{1,2})$/, '.$1-$2');
}

/** Celular com DDD, limitado a 11 dígitos: `(00) 00000-0000`. */
export function maskPhone(value: string): string {
  const d = digits(value, 11);
  if (d.length <= 2) return d;
  if (d.length <= 7) return `(${d.slice(0, 2)}) ${d.slice(2)}`;
  return `(${d.slice(0, 2)}) ${d.slice(2, 7)}-${d.slice(7)}`;
}

/** CEP progressivo, limitado a 8 dígitos: `00000-000`. */
export function maskCep(value: string): string {
  const d = digits(value, 8);
  return d.length > 5 ? `${d.slice(0, 5)}-${d.slice(5)}` : d;
}

/** Data progressiva, limitada a 8 dígitos: `dd/mm/aaaa`. */
export function maskDate(value: string): string {
  const d = digits(value, 8);
  if (d.length <= 2) return d;
  if (d.length <= 4) return `${d.slice(0, 2)}/${d.slice(2)}`;
  return `${d.slice(0, 2)}/${d.slice(2, 4)}/${d.slice(4)}`;
}

/** UF: só letras, MAIÚSCULAS, no máximo 2. */
export function maskUf(value: string): string {
  return value
    .replace(/[^a-zA-Z]/g, '')
    .toUpperCase()
    .slice(0, 2);
}

/**
 * `dd/mm/aaaa` (mascarada) → ISO `aaaa-mm-dd`, que é o que a API espera. Devolve
 * `null` se a data não for uma data real (formato incompleto, mês 13, 31/02…) —
 * a validação de calendário evita mandar lixo plausível ao servidor.
 */
export function dateBrToIso(value: string): string | null {
  const m = /^(\d{2})\/(\d{2})\/(\d{4})$/.exec(value.trim());
  if (!m) return null;

  const [, dd, mm, yyyy] = m;
  const day = Number(dd);
  const month = Number(mm);
  const year = Number(yyyy);

  // Reconstrói a data em UTC e confere que os campos não "transbordaram"
  // (31/02 vira 03/03 no Date; a comparação rejeita isso).
  const date = new Date(Date.UTC(year, month - 1, day));
  if (
    date.getUTCFullYear() !== year ||
    date.getUTCMonth() !== month - 1 ||
    date.getUTCDate() !== day
  ) {
    return null;
  }

  return `${yyyy}-${mm}-${dd}`;
}
