import { createSignal, For, type JSX } from "solid-js";

interface DragListProps<T> {
  items: T[];
  getId: (item: T) => string;
  onReorder: (reorderedItems: T[]) => void;
  renderItem: (item: T, dragHandleProps: DragHandleProps) => JSX.Element;
}

export interface DragHandleProps {
  draggable: true;
  onDragStart: (e: DragEvent) => void;
  onDragEnd: (e: DragEvent) => void;
}

export default function DragList<T>(props: DragListProps<T>) {
  const [dragIdx, setDragIdx] = createSignal<number | null>(null);
  const [overIdx, setOverIdx] = createSignal<number | null>(null);

  const handleDragStart = (index: number) => (e: DragEvent) => {
    setDragIdx(index);
    if (e.dataTransfer) {
      e.dataTransfer.effectAllowed = "move";
      e.dataTransfer.setData("text/plain", String(index));
    }
  };

  const handleDragEnd = () => {
    setDragIdx(null);
    setOverIdx(null);
  };

  const handleDragOver = (index: number) => (e: DragEvent) => {
    e.preventDefault();
    if (e.dataTransfer) {
      e.dataTransfer.dropEffect = "move";
    }
    setOverIdx(index);
  };

  const handleDrop = (targetIdx: number) => (e: DragEvent) => {
    e.preventDefault();
    const sourceIdx = dragIdx();
    if (sourceIdx === null || sourceIdx === targetIdx) {
      setDragIdx(null);
      setOverIdx(null);
      return;
    }

    const items = [...props.items];
    const [moved] = items.splice(sourceIdx, 1);
    items.splice(targetIdx, 0, moved);

    setDragIdx(null);
    setOverIdx(null);
    props.onReorder(items);
  };

  return (
    <div class="space-y-1">
      <For each={props.items}>
        {(item, index) => {
          const dragHandleProps: DragHandleProps = {
            draggable: true,
            onDragStart: handleDragStart(index()),
            onDragEnd: handleDragEnd,
          };

          return (
            <div
              onDragOver={handleDragOver(index())}
              onDrop={handleDrop(index())}
              class={`transition-all ${
                overIdx() === index() && dragIdx() !== null && dragIdx() !== index()
                  ? "border-t-2 border-blue-400"
                  : "border-t-2 border-transparent"
              } ${dragIdx() === index() ? "opacity-40" : ""}`}
            >
              {props.renderItem(item, dragHandleProps)}
            </div>
          );
        }}
      </For>
    </div>
  );
}
