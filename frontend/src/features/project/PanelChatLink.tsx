import type { Component } from "solid-js";

import { Button } from "~/ui";

interface Props {
  type: "goal" | "roadmap-step" | "feature" | "milestone";
  id: string;
  title: string;
  context?: string;
  onDiscuss: (message: string) => void;
}

const PanelChatLink: Component<Props> = (props) => {
  const handleClick = () => {
    const ref = `[${props.type}:${props.id}]`;
    const contextLine = props.context ? `\n\nContext: ${props.context}` : "";
    props.onDiscuss(`Let's discuss ${props.type} "${props.title}" ${ref}${contextLine}`);
  };

  return (
    <Button
      variant="ghost"
      size="xs"
      onClick={handleClick}
      title={`Discuss "${props.title}" in chat`}
    >
      Discuss
    </Button>
  );
};

export default PanelChatLink;
