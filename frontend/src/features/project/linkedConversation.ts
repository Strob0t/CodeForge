import { api } from "~/api/client";

/**
 * Whether a linked conversation (/projects/<id>?conversation=<cid>, a search
 * hit) belongs to the project whose page opens it; another project's
 * conversation does not open in this project's chat. A failed load (unknown,
 * or not the caller's tenant) is passed on.
 */
export async function conversationInProject(
  projectId: string,
  conversationId: string,
): Promise<boolean> {
  const conversation = await api.conversations.get(conversationId);
  return conversation.project_id === projectId;
}
