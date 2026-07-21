import { maskCpf, maskDate, maskPhone } from '../../../shared/masks';
import { Button } from '../../../shared/ui/Button';
import { Input } from '../../../shared/ui/Input';
import type { RegisterForm } from '../RegisterPage';

interface Props {
  form: RegisterForm;
  onChange: (patch: Partial<RegisterForm>) => void;
  onContinue: () => void;
  error?: string | null;
}

/** Passo 1 do cadastro: identidade. As máscaras são aplicadas na digitação. */
export function PersonalDataStep({ form, onChange, onContinue, error }: Props) {
  return (
    <div className="flex flex-col gap-3.5">
      <Input
        label="Nome completo"
        placeholder="Como no seu documento"
        autoComplete="name"
        value={form.nome}
        onChange={(e) => onChange({ nome: e.target.value })}
      />
      <Input
        label="CPF"
        placeholder="000.000.000-00"
        inputMode="numeric"
        value={form.cpf}
        onChange={(e) => onChange({ cpf: maskCpf(e.target.value) })}
      />
      <div className="flex gap-3">
        <div className="flex-1">
          <Input
            label="Nascimento"
            placeholder="dd/mm/aaaa"
            inputMode="numeric"
            value={form.nasc}
            onChange={(e) => onChange({ nasc: maskDate(e.target.value) })}
          />
        </div>
        <div className="flex-1">
          <Input
            label="Celular"
            placeholder="(00) 00000-0000"
            inputMode="numeric"
            autoComplete="tel"
            value={form.cel}
            onChange={(e) => onChange({ cel: maskPhone(e.target.value) })}
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
