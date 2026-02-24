/* eslint-disable solid/no-innerhtml -- Markdown renderer: all innerHTML goes through renderInline which escapes HTML entities. */
import { For, Show } from "solid-js";

interface MarkdownProps {
  content: string;
}

interface MarkdownNode {
  type: "heading" | "paragraph" | "code" | "list" | "blockquote" | "hr";
  level?: number;
  lang?: string;
  text?: string;
  children?: MarkdownNode[];
}

/** Parse markdown content into a flat list of block-level nodes. */
function parseBlocks(content: string): MarkdownNode[] {
  const lines = content.split("\n");
  const nodes: MarkdownNode[] = [];
  let i = 0;

  while (i < lines.length) {
    const line = lines[i];

    // Fenced code block
    if (line.startsWith("```")) {
      const lang = line.slice(3).trim();
      const codeLines: string[] = [];
      i++;
      while (i < lines.length && !lines[i].startsWith("```")) {
        codeLines.push(lines[i]);
        i++;
      }
      i++; // skip closing ```
      nodes.push({ type: "code", lang, text: codeLines.join("\n") });
      continue;
    }

    // Heading
    const headingMatch = line.match(/^(#{1,6})\s+(.+)/);
    if (headingMatch) {
      nodes.push({ type: "heading", level: headingMatch[1].length, text: headingMatch[2] });
      i++;
      continue;
    }

    // Horizontal rule
    if (/^(-{3,}|\*{3,}|_{3,})$/.test(line.trim())) {
      nodes.push({ type: "hr" });
      i++;
      continue;
    }

    // Blockquote
    if (line.startsWith("> ")) {
      const quoteLines: string[] = [];
      while (i < lines.length && lines[i].startsWith("> ")) {
        quoteLines.push(lines[i].slice(2));
        i++;
      }
      nodes.push({ type: "blockquote", text: quoteLines.join("\n") });
      continue;
    }

    // Unordered or ordered list
    if (/^[-*+]\s/.test(line) || /^\d+\.\s/.test(line)) {
      const listItems: MarkdownNode[] = [];
      while (i < lines.length && (/^[-*+]\s/.test(lines[i]) || /^\d+\.\s/.test(lines[i]))) {
        const itemText = lines[i].replace(/^[-*+]\s|^\d+\.\s/, "");
        listItems.push({ type: "paragraph", text: itemText });
        i++;
      }
      nodes.push({ type: "list", children: listItems });
      continue;
    }

    // Empty line
    if (line.trim() === "") {
      i++;
      continue;
    }

    // Paragraph: collect consecutive non-empty, non-special lines
    const paraLines: string[] = [];
    while (
      i < lines.length &&
      lines[i].trim() !== "" &&
      !lines[i].startsWith("#") &&
      !lines[i].startsWith("```") &&
      !lines[i].startsWith("> ") &&
      !/^[-*+]\s/.test(lines[i]) &&
      !/^\d+\.\s/.test(lines[i]) &&
      !/^(-{3,}|\*{3,}|_{3,})$/.test(lines[i].trim())
    ) {
      paraLines.push(lines[i]);
      i++;
    }
    if (paraLines.length > 0) {
      nodes.push({ type: "paragraph", text: paraLines.join("\n") });
    }
  }

  return nodes;
}

/** Render inline markdown (bold, italic, code, links) into HTML string. */
function renderInline(text: string): string {
  let result = text;
  // Escape HTML
  result = result.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
  // Bold
  result = result.replace(/\*\*(.+?)\*\*/g, "<strong>$1</strong>");
  result = result.replace(/__(.+?)__/g, "<strong>$1</strong>");
  // Italic
  result = result.replace(/\*(.+?)\*/g, "<em>$1</em>");
  result = result.replace(/_(.+?)_/g, "<em>$1</em>");
  // Inline code
  result = result.replace(
    /`([^`]+)`/g,
    '<code class="rounded-cf-sm bg-cf-bg-inset px-1 py-0.5 text-sm">$1</code>',
  );
  // Links
  result = result.replace(
    /\[([^\]]+)\]\(([^)]+)\)/g,
    '<a href="$2" class="text-cf-accent underline hover:opacity-80" target="_blank" rel="noopener noreferrer">$1</a>',
  );
  return result;
}

function HeadingEl(props: { level: number; text: string }) {
  const html = () => renderInline(props.text);
  const classes: Record<number, string> = {
    1: "text-2xl font-bold mt-4 mb-2",
    2: "text-xl font-semibold mt-3 mb-2",
    3: "text-lg font-medium mt-2 mb-1",
    4: "text-base font-medium mt-2 mb-1",
    5: "text-sm font-medium mt-1 mb-1",
    6: "text-sm font-medium mt-1 mb-1",
  };

  return <div class={classes[props.level] || classes[3]} innerHTML={html()} />;
}

export default function Markdown(props: MarkdownProps) {
  const blocks = () => parseBlocks(props.content);

  return (
    <div class="prose prose-sm dark:prose-invert max-w-none">
      <For each={blocks()}>
        {(node) => {
          switch (node.type) {
            case "heading":
              return <HeadingEl level={node.level ?? 1} text={node.text ?? ""} />;

            case "code":
              return (
                <div class="my-2">
                  <Show when={node.lang}>
                    <div class="rounded-t-cf-sm bg-cf-bg-surface-alt px-3 py-1 text-xs text-cf-text-secondary">
                      {node.lang}
                    </div>
                  </Show>
                  <pre
                    class={`overflow-x-auto bg-cf-bg-primary p-3 text-sm text-cf-text-primary ${
                      node.lang ? "rounded-b-cf-sm" : "rounded-cf-sm"
                    }`}
                  >
                    <code>{node.text}</code>
                  </pre>
                </div>
              );

            case "blockquote":
              return (
                <blockquote
                  class="my-2 border-l-4 border-cf-border pl-3 italic text-cf-text-secondary"
                  innerHTML={renderInline(node.text ?? "")}
                />
              );

            case "list":
              return (
                <ul class="my-1 list-disc space-y-0.5 pl-5">
                  <For each={node.children ?? []}>
                    {(item) => <li innerHTML={renderInline(item.text ?? "")} />}
                  </For>
                </ul>
              );

            case "hr":
              return <hr class="my-3 border-cf-border" />;

            case "paragraph":
            default:
              return <p class="my-1 leading-relaxed" innerHTML={renderInline(node.text ?? "")} />;
          }
        }}
      </For>
    </div>
  );
}
