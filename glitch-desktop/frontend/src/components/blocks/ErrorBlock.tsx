import { AlertTriangle } from "lucide-react";

interface Props {
  message: string;
}

export function ErrorBlock({ message }: Props) {
  return (
    <div className="my-2 flex items-start gap-2 p-3 rounded-lg bg-red/10 border border-red/20">
      <AlertTriangle size={14} className="text-pink mt-0.5 shrink-0" />
      <div className="text-[13px] text-pink">{message}</div>
    </div>
  );
}
