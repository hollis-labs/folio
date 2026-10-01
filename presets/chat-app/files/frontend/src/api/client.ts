import { createApiClient } from "@hollis-labs/design-app-runtime"

const http = createApiClient({ baseUrl: "" })

export interface ChatReply {
  reply: string
}

// Replace this app-owned echo endpoint with your transport when ready.
export const apiClient = {
  sendMessage: (message: string) => http.post<ChatReply>("/api/messages", { message }),
}

export type AppApiClient = typeof apiClient
