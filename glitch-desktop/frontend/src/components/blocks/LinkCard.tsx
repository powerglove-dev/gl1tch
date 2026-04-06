import { ExternalLink } from "lucide-react";

interface Props {
  url: string;
  title: string;
  description?: string;
}

export function LinkCard({ url, title, description }: Props) {
  return (
    <a
      href={url}
      target="_blank"
      rel="noopener noreferrer"
      className="my-2 flex items-start gap-3 p-3 rounded-lg border border-surface hover:border-purple/50 hover:bg-surface/50 transition-all group"
    >
      <ExternalLink
        size={14}
        className="text-purple mt-0.5 shrink-0 group-hover:text-cyan transition-colors"
      />
      <div className="min-w-0">
        <div className="text-[13px] font-medium text-purple group-hover:text-cyan transition-colors truncate">
          {title}
        </div>
        {description && (
          <div className="text-[12px] text-comment mt-0.5 line-clamp-2">
            {description}
          </div>
        )}
        <div className="text-[11px] text-comment mt-1 truncate">{url}</div>
      </div>
    </a>
  );
}
