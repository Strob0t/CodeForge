import type { CoreClient } from "../core";
import { url } from "../factory";
import type {
  Conversation,
  ConversationMessage,
  CreateConversationRequest,
  SendMessageRequest,
  Session,
} from "../types";

export function createConversationsResource(c: CoreClient) {
  return {
    create: (projectId: string, data?: CreateConversationRequest) =>
      c.post<Conversation>(url`/projects/${projectId}/conversations`, data ?? {}),

    list: (projectId: string) => c.get<Conversation[]>(url`/projects/${projectId}/conversations`),

    messages: (id: string) => c.get<ConversationMessage[]>(url`/conversations/${id}/messages`),

    send: (id: string, data: SendMessageRequest) =>
      c.post<{ status: string; run_id: string; message: string }>(
        url`/conversations/${id}/messages`,
        data,
      ),

    stop: (id: string) =>
      c.post<{ status: string; conversation_id: string }>(url`/conversations/${id}/stop`),

    session: (id: string) => c.get<Session>(url`/conversations/${id}/session`),

    fork: (id: string, data?: { from_event_id?: string }) =>
      c.post<Session>(url`/conversations/${id}/fork`, data ?? {}),

    rewind: (id: string, data?: { run_id?: string; to_event_id?: string }) =>
      c.post<Session>(url`/conversations/${id}/rewind`, data ?? {}),
  };
}
