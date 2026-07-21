import { digitsOnly, maskCep, maskUf } from '../../../shared/masks';
import { Button } from '../../../shared/ui/Button';
import { Input } from '../../../shared/ui/Input';
import type { RegisterForm } from '../RegisterPage';

interface Props {
  form: RegisterForm;
  onChange: (patch: Partial<RegisterForm>) => void;
  onContinue: () => void;
  /** Disparado quando o CEP fica completo (8 dígitos): pai busca na ViaCEP. */
  onCepComplete: (cep: string) => void;
  cepLoading: boolean;
  error?: string | null;
}

/**
 * Passo 2 do cadastro: endereço. Ao completar o CEP, o pai preenche rua/bairro/
 * cidade/UF pela ViaCEP — mas os campos SEGUEM editáveis (o autofill é chute).
 */
export function AddressStep({ form, onChange, onContinue, onCepComplete, cepLoading, error }: Props) {
  function handleCep(raw: string) {
    const cep = maskCep(raw);
    onChange({ cep });
    if (digitsOnly(cep).length === 8) onCepComplete(cep);
  }

  return (
    <div className="flex flex-col gap-3.5">
      <Input
        label="CEP"
        placeholder="00000-000"
        inputMode="numeric"
        autoComplete="postal-code"
        hint={cepLoading ? 'Buscando endereço…' : 'Com o CEP a gente preenche o resto para você.'}
        value={form.cep}
        onChange={(e) => handleCep(e.target.value)}
      />
      <Input
        label="Endereço"
        placeholder="Rua, avenida…"
        autoComplete="address-line1"
        value={form.rua}
        onChange={(e) => onChange({ rua: e.target.value })}
      />
      <div className="flex gap-3">
        <div className="flex-1">
          <Input
            label="Número"
            placeholder="123"
            inputMode="numeric"
            value={form.numero}
            onChange={(e) => onChange({ numero: e.target.value })}
          />
        </div>
        <div className="flex-[1.6]">
          <Input
            label="Complemento"
            placeholder="Opcional"
            value={form.compl}
            onChange={(e) => onChange({ compl: e.target.value })}
          />
        </div>
      </div>
      <Input
        label="Bairro"
        placeholder="Seu bairro"
        value={form.bairro}
        onChange={(e) => onChange({ bairro: e.target.value })}
      />
      <div className="flex gap-3">
        <div className="flex-[2]">
          <Input
            label="Cidade"
            placeholder="Sua cidade"
            value={form.cidade}
            onChange={(e) => onChange({ cidade: e.target.value })}
          />
        </div>
        <div className="flex-1">
          <Input
            label="UF"
            placeholder="SP"
            value={form.uf}
            onChange={(e) => onChange({ uf: maskUf(e.target.value) })}
          />
        </div>
      </div>

      {error && (
        <p role="alert" className="rounded-md bg-[rgba(205,25,25,0.08)] p-3 text-[13px] text-error">
          {error}
        </p>
      )}

      <Button color="primary" size="lg" fullWidth onClick={onContinue}>
        Continuar
      </Button>
    </div>
  );
}
