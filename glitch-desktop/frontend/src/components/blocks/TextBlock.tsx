import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeHighlight from "rehype-highlight";

interface Props {
  content: string;
  streaming?: boolean;
}

export function TextBlock({ content, streaming }: Props) {
  return (
    <div className="prose">
      <ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeHighlight]}>
        {content}
      </ReactMarkdown>
      {streaming && <span className="streaming-cursor" />}
    </div>
  );
}
