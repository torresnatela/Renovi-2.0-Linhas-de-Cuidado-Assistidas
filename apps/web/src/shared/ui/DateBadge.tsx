import { dayOfMonth, monthAbbrev } from '../datetime';

/**
 * Selo de data curto (mês + dia) para consultas. `timeZone` é OBRIGATÓRIO — o
 * dia é lido no fuso da agenda, nunca no do browser (um horário das 23:30 em SP
 * cairia no dia seguinte se lido em UTC). Ver shared/datetime.
 */
export function DateBadge({ iso, timeZone }: { iso: string; timeZone: string }) {
  return (
    <div className="flex w-[52px] shrink-0 flex-col items-center rounded-md bg-primary-100 py-2 text-primary-300">
      <span className="text-[10px] font-bold uppercase opacity-70">
        {monthAbbrev(iso, timeZone)}
      </span>
      <span className="text-[20px] font-bold leading-[22px]">{dayOfMonth(iso, timeZone)}</span>
    </div>
  );
}
