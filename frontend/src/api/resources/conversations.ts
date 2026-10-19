import type { CoreClient } from "../core";
import { url } from "../factory";
import type {
  Conversation,
  ConversationMessage,
  ConversationRunState,
  CreateConversationRequest,
  SendMessageRequest,
  Session,
} from "../types";

export function createConversationsResource(c: CoreClient) {
  return {
    create: (projectId: string, data?: CreateConversationRequest) =>
      c.post<Conversation>(url`/projects/${projectId}/conversations`, data ?? {}),

    list: (projectId: string) => c.get<Conversation[]>(url`/projects/${projectId}/conversations`),

    get: (id: string) => c.get<Conversation>(url`/conversations/${id}`),

    messages: (id: string) => c.get<ConversationMessage[]>(url`/conversations/${id}/messages`),

    send: (id: string, data: SendMessageRequest) =>
      c.post<{ status: string; run_id: string; message: string }>(
        url`/conversations/${id}/messages`,
        data,
      ),

    stop: (id: string) =>
      c.post<{ status: string; conversation_id: string }>(url`/conversations/${id}/stop`),

    /** The conversation's session; undefined (204) when it has none yet. */
    session: (id: string) => c.get<Session | undefined>(url`/conversations/${id}/session`),

    /** The running turn and its pending approvals (KI-148). */
    runState: (id: string) => c.get<ConversationRunState>(url`/conversations/${id}/run`),

    fork: (id: string, data?: { from_event_id?: string }) =>
      c.post<Session>(url`/conversations/${id}/fork`, data ?? {}),

    rewind: (id: string, data?: { run_id?: string; to_event_id?: string }) =>
      c.post<Session>(url`/conversations/${id}/rewind`, data ?? {}),
  };
}
