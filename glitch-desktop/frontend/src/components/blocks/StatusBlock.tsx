import { Loader2 } from "lucide-react";

interface Props {
  text: string;
}

export function StatusBlock({ text }: Props) {
  return (
    <div className="my-2 flex items-center gap-2 text-[12px] text-orange">
      <Loader2 size={12} className="animate-spin" />
      <span>{text}</span>
    </div>
  );
}
