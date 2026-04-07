package glitchctx

// OutputProtocolInstructions tells a text-based agent how to STRUCTURE its
// reply so the gl1tch chat can render distinct content types (notes, tables,
// status pings, code) without parsing freeform stdout.
//
// Append this to ProtocolInstructions when invoking a provider whose output
// will land in the desktop chat. Pure CLI/file-output use cases can skip it.
//
// The grammar mirrors the existing input protocol: each block is delimited
// by `<<GLITCH_*>>` and `<<GLITCH_END>>` lines, and `<<GLITCH_END>>` is
// shared so a single splitter handles every block type.
//
// The matching streaming parser lives in pkg/glitchproto/splitter.go in the
// main gl1tch module — when you change the marker grammar here, update the
// parser there.
const OutputProtocolInstructions = `## Output formatting protocol

Your reply is rendered in a chat UI, not a terminal. Wrap distinct pieces
of content in the blocks below so the chat can format each one properly.
Each delimiter MUST appear on its own line.

PROSE / EXPLANATION:

<<GLITCH_TEXT>>
markdown text — paragraphs, lists, links, inline code
<<GLITCH_END>>

STRUCTURED NOTE / FINDING (rendered as a coloured card):

<<GLITCH_NOTE type="finding|insight|decision|note" title="short title" tags="tag1,tag2">>
markdown body of the note
<<GLITCH_END>>

TABLE (use real markdown table syntax inside; the chat will render it):

<<GLITCH_TABLE title="optional caption">>
| col1 | col2 |
| --- | --- |
| a | b |
<<GLITCH_END>>

CODE BLOCK (separate from prose, so it gets a header + copy button):

<<GLITCH_CODE lang="go" file="optional/path.go">>
code goes here
<<GLITCH_END>>

TRANSIENT STATUS (shown as a "thinking" pill while you work; wrap each
short status update — they will be replaced as new ones arrive):

<<GLITCH_STATUS>>
analyzing repo structure
<<GLITCH_END>>

Rules:
- Every block must end with <<GLITCH_END>> on its own line.
- Anything OUTSIDE a block is treated as plain text and shown verbatim,
  but you should prefer wrapping prose in <<GLITCH_TEXT>> for clarity.
- Multiple blocks are allowed and rendered in order.
- Do NOT nest blocks.
- Tables MUST use real markdown pipe syntax with a separator row.
`
