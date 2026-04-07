import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import remarkBreaks from "remark-breaks";
import rehypeHighlight from "rehype-highlight";
import { Collapsible } from "./Collapsible";
import { ActivityBlock } from "./ActivityBlock";
import { BrainBlock } from "./BrainBlock";
import { parseAgentOutput } from "@/lib/parseAgentOutput";

interface Props {
  content: string;
  streaming?: boolean;
}

// Collapse very long text blobs (e.g. minified JS someone pasted into chat).
// We skip collapsing while streaming so growing responses don't jitter.
const COLLAPSE_CHAR_THRESHOLD = 800;
const COLLAPSE_LINE_THRESHOLD = 16;

function shouldCollapse(content: string): boolean {
  if (content.length > COLLAPSE_CHAR_THRESHOLD) return true;
  let lines = 1;
  for (let i = 0; i < content.length; i++) {
    if (content.charCodeAt(i) === 10 && ++lines > COLLAPSE_LINE_THRESHOLD) return true;
  }
  return false;
}

export function TextBlock({ content, streaming }: Props) {
  // Split out CLI agent preamble / tool traces into a dedicated activity
  // panel so the body only contains the actual model response. We parse on
  // every render (cheap — linear over lines) rather than at stream time so
  // partial lines simply stay in the body until they're complete.
  const parsed = parseAgentOutput(content);
  const activity = (parsed.summary.length > 0 || parsed.tools.length > 0)
    ? <ActivityBlock summary={parsed.summary} tools={parsed.tools} />
    : null;

  const markdown = (
    <div className="prose">
      <ReactMarkdown
        remarkPlugins={[remarkGfm, remarkBreaks]}
        rehypePlugins={[rehypeHighlight]}
      >
        {parsed.body}
      </ReactMarkdown>
      {streaming && <span className="streaming-cursor" />}
    </div>
  );

  const body = shouldCollapse(parsed.body) && !streaming
    ? <Collapsible maxHeight={220}>{markdown}</Collapsible>
    : markdown;

  return (
    <>
      {activity}
      {parsed.brains.map((note, i) => (
        <BrainBlock key={i} note={note} />
      ))}
      {body}
    </>
  );
}
