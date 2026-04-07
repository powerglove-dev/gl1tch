import { useState } from "react";
import { Copy, Check, ExternalLink } from "lucide-react";
import { Collapsible } from "./Collapsible";

interface Props {
  content: string;
  language: string;
  filename?: string;
}

export function CodeBlock({ content, language, filename }: Props) {
  const [copied, setCopied] = useState(false);

  async function handleCopy() {
    await navigator.clipboard.writeText(content);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }

  return (
    <div className="my-3 rounded-lg border border-surface overflow-hidden">
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-2 bg-bg-darker border-b border-surface">
        <div className="flex items-center gap-2 text-[12px]">
          {filename && <span className="text-cyan">{filename}</span>}
          <span className="text-comment">{language}</span>
        </div>
        <div className="flex items-center gap-1">
          {filename && (
            <button
              className="p-1 rounded hover:bg-surface text-comment hover:text-fg transition-colors"
              title="Open in editor"
            >
              <ExternalLink size={13} />
            </button>
          )}
          <button
            onClick={handleCopy}
            className="p-1 rounded hover:bg-surface text-comment hover:text-fg transition-colors"
            title="Copy"
          >
            {copied ? (
              <Check size={13} className="text-green" />
            ) : (
              <Copy size={13} />
            )}
          </button>
        </div>
      </div>

      {/* Code */}
      <Collapsible maxHeight={320} label={`${language || "code"}`}>
        <pre className="p-4 overflow-x-auto text-[13px] leading-relaxed bg-bg-darker font-mono">
          <code>{content}</code>
        </pre>
      </Collapsible>
    </div>
  );
}
