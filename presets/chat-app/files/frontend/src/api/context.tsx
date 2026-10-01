import { createApiContext } from "@hollis-labs/design-app-runtime"
import { apiClient } from "./client"

// Typed { ApiProvider, useApi } bound to this app's concrete client.
// Pages read the client with `useApi()`; see pages/chat.tsx.
export const { ApiProvider, useApi } = createApiContext(apiClient)
