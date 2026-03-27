import { type Component, createSignal, Show } from "solid-js";

import FeatureMapPanel from "./FeatureMapPanel";
import GoalsPanel from "./GoalsPanel";
import RoadmapPanel from "./RoadmapPanel";

interface Props {
  projectId: string;
  onSendChatMessage?: (msg: string) => void;
  onAIDiscoverStarted?: (conversationId: string) => void;
  onNavigate?: (target: string) => void;
  onError?: (err: string) => void;
}

const ConsolidatedPlanView: Component<Props> = (props) => {
  const [goalsOpen, setGoalsOpen] = createSignal(true);
  const [roadmapOpen, setRoadmapOpen] = createSignal(true);
  const [featuremapOpen, setFeaturemapOpen] = createSignal(false);

  const handleError = (msg: string) => props.onError?.(msg);

  return (
    <div class="flex flex-col h-full overflow-y-auto">
      <section>
        <button
          class="w-full flex items-center justify-between px-3 py-2 text-sm font-medium text-cf-text-primary hover:bg-cf-bg-tertiary border-b border-cf-border"
          onClick={() => setGoalsOpen(!goalsOpen())}
        >
          <span>Goals</span>
          <span class="text-xs text-cf-text-tertiary">{goalsOpen() ? "\u25B4" : "\u25BE"}</span>
        </button>
        <Show when={goalsOpen()}>
          <div class="p-2">
            <GoalsPanel
              projectId={props.projectId}
              onAIDiscoverStarted={props.onAIDiscoverStarted}
              onNavigate={props.onNavigate}
              onSendChatMessage={props.onSendChatMessage}
            />
          </div>
        </Show>
      </section>
      <section>
        <button
          class="w-full flex items-center justify-between px-3 py-2 text-sm font-medium text-cf-text-primary hover:bg-cf-bg-tertiary border-b border-cf-border"
          onClick={() => setRoadmapOpen(!roadmapOpen())}
        >
          <span>Roadmap</span>
          <span class="text-xs text-cf-text-tertiary">{roadmapOpen() ? "\u25B4" : "\u25BE"}</span>
        </button>
        <Show when={roadmapOpen()}>
          <div class="p-2">
            <RoadmapPanel
              projectId={props.projectId}
              onError={handleError}
              onNavigate={props.onNavigate}
              onSendChatMessage={props.onSendChatMessage}
            />
          </div>
        </Show>
      </section>
      <section>
        <button
          class="w-full flex items-center justify-between px-3 py-2 text-sm font-medium text-cf-text-primary hover:bg-cf-bg-tertiary border-b border-cf-border"
          onClick={() => setFeaturemapOpen(!featuremapOpen())}
        >
          <span>Feature Map</span>
          <span class="text-xs text-cf-text-tertiary">
            {featuremapOpen() ? "\u25B4" : "\u25BE"}
          </span>
        </button>
        <Show when={featuremapOpen()}>
          <div class="p-2">
            <FeatureMapPanel
              projectId={props.projectId}
              onSendChatMessage={props.onSendChatMessage}
            />
          </div>
        </Show>
      </section>
    </div>
  );
};

export default ConsolidatedPlanView;
