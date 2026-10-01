import { AppShell, Button } from "@hollis-labs/design-components"
import { useState } from "react"
import { ChatPage } from "./pages/chat"

export function App() {
  const [conversation, setConversation] = useState(0)

  return (
    <AppShell
      nav={
        <aside className="hidden w-56 flex-col gap-4 border-r border-border-subtle p-4 md:flex">
          <h1 className="text-control font-semibold text-fg">Conversations</h1>
          <Button onClick={() => setConversation((value) => value + 1)}>New conversation</Button>
        </aside>
      }
      header={
        <header className="flex items-center justify-between border-b border-border-subtle px-4 py-3">
          <h2 className="text-control font-semibold text-fg">Chat</h2>
          <div className="flex items-center gap-3">
            <span className="hidden text-caption text-fg-muted sm:inline">Local echo demo</span>
            <Button
              className="md:hidden"
              size="sm"
              onClick={() => setConversation((value) => value + 1)}
            >
              New conversation
            </Button>
          </div>
        </header>
      }
    >
      <ChatPage key={conversation} />
    </AppShell>
  )
}
