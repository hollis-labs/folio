import { EmptyState } from "@hollis-labs/design-components"
import { ChatInput, type ChatItem, ChatStream } from "@hollis-labs/kit-chat"
import { useEffect, useRef, useState } from "react"
import { useApi } from "../api/context"

// Draft, transcript, and transport belong to the app; the kit draws them.
export function ChatPage() {
  const api = useApi()
  const [draft, setDraft] = useState("")
  const [items, setItems] = useState<ChatItem[]>([])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const mounted = useRef(true)

  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
    }
  }, [])

  async function send(value: string) {
    const message = value.trim()
    if (!message || busy) return
    setDraft("")
    setError(null)
    setBusy(true)
    setItems((current) => [
      ...current,
      { kind: "message", id: crypto.randomUUID(), role: "user", author: "You", content: message },
    ])
    try {
      const { reply } = await api.sendMessage(message)
      if (mounted.current) {
        setItems((current) => [
          ...current,
          {
            kind: "message",
            id: crypto.randomUUID(),
            role: "assistant",
            author: "Echo",
            content: reply,
          },
        ])
      }
    } catch (cause) {
      if (mounted.current) setError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      if (mounted.current) setBusy(false)
    }
  }

  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col">
      <ChatStream
        items={items}
        status={error ? { status: "error", message: error } : { status: "idle" }}
        empty={
          <EmptyState
            variant="empty"
            title="Start a conversation"
            description="Send a message to try the local echo."
          />
        }
      />
      <div className="border-t border-border-subtle p-4">
        <ChatInput
          value={draft}
          onValueChange={setDraft}
          onSubmit={(value) => void send(value)}
          busy={busy}
          placeholder="Write a message…"
          aria-label="Message"
        />
      </div>
    </div>
  )
}
