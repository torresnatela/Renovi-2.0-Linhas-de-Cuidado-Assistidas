// Consulta de endereço por CEP na ViaCEP, direto do navegador.
//
// Fica FORA de shared/api.ts de propósito: não é a nossa API (não usa `credentials`,
// não passa pela camada de erro RFC 7807) e é uma conveniência de preenchimento, não
// uma fonte de verdade. Se a ViaCEP cair ou o CEP não existir, devolvemos `null` em
// silêncio — o usuário simplesmente digita o endereço à mão. Nunca bloqueia o cadastro.

import { digitsOnly } from '../../shared/masks';

export interface CepAddress {
  street: string;
  neighborhood: string;
  city: string;
  state: string;
}

/** Forma bruta do JSON da ViaCEP (campos que nos interessam). */
interface ViaCepResponse {
  erro?: boolean;
  logradouro?: string;
  bairro?: string;
  localidade?: string;
  uf?: string;
}

export async function lookupCep(cep: string): Promise<CepAddress | null> {
  const digits = digitsOnly(cep);
  if (digits.length !== 8) return null;

  try {
    const res = await fetch(`https://viacep.com.br/ws/${digits}/json/`);
    if (!res.ok) return null;

    const data = (await res.json()) as ViaCepResponse;
    // CEP inexistente vem como { erro: true } com HTTP 200.
    if (data.erro) return null;

    return {
      street: data.logradouro ?? '',
      neighborhood: data.bairro ?? '',
      city: data.localidade ?? '',
      state: data.uf ?? '',
    };
  } catch {
    // Rede offline, DNS, CORS: falha silenciosa, preenchimento manual.
    return null;
  }
}
