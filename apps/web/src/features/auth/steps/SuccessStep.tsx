import { Button } from '../../../shared/ui/Button';
import { IconCheck } from '../../../shared/ui/icons';

interface Props {
  firstName: string;
  onStart: () => void;
}

/** Fim do cadastro: a sessão já veio do register, "Começar" só leva à jornada. */
export function SuccessStep({ firstName, onStart }: Props) {
  return (
    <div className="flex flex-col items-center gap-4 px-3 py-6 text-center">
      <span className="inline-flex h-[72px] w-[72px] items-center justify-center rounded-full bg-success text-white">
        <IconCheck size={34} />
      </span>
      <h1 className="text-[28px] font-bold text-primary-300">Conta criada, {firstName}</h1>
      <p className="max-w-[340px] text-[15px] leading-[23px] text-ink">
        Sua jornada de cuidado já está pronta. Seus dados são sigilosos — sua empresa nunca vê
        informações individuais.
      </p>
      <div className="mt-2 w-full">
        <Button color="accent" size="lg" fullWidth onClick={onStart}>
          Começar
        </Button>
      </div>
    </div>
  );
}
