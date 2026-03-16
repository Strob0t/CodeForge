import type { Component } from "solid-js";
import { createSignal } from "solid-js";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface ChannelInputProps {
  onSend: (content: string) => void;
  placeholder?: string;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

const ChannelInput: Component<ChannelInputProps> = (props) => {
  const [text, setText] = createSignal("");

  const canSend = (): boolean => text().trim().length > 0;

  function handleSend(): void {
    const content = text().trim();
    if (!content) return;
    props.onSend(content);
    setText("");
  }

  function handleKeyDown(e: KeyboardEvent): void {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  }

  return (
    <div class="flex items-end gap-2 border-t border-cf-border bg-cf-bg-surface px-4 py-3">
      <textarea
        rows={1}
        class="flex-1 resize-none rounded-cf-md border border-cf-border bg-cf-bg-surface-alt px-3 py-2 text-sm text-cf-text-primary placeholder-cf-text-muted focus:border-cf-accent focus:ring-1 focus:ring-cf-accent focus:outline-none"
        placeholder={props.placeholder ?? "Type a message..."}
        value={text()}
        onInput={(e) => setText(e.currentTarget.value)}
        onKeyDown={handleKeyDown}
      />
      <button
        type="button"
        class="shrink-0 rounded-cf-md bg-cf-accent px-4 py-2 text-sm font-medium text-cf-accent-fg hover:bg-cf-accent-hover disabled:opacity-40 disabled:cursor-not-allowed transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cf-focus-ring focus-visible:ring-offset-2"
        disabled={!canSend()}
        onClick={handleSend}
        aria-label="Send message"
      >
        Send
      </button>
    </div>
  );
};

export default ChannelInput;
