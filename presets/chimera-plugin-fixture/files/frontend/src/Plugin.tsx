import { createStylesheetLeases } from "virtual:plugin-host-ui/stylesheets"
import { Button } from "@hollis-labs/design-components"
import type { RegistryEntry } from "@hollis-labs/plugin-host-ui"
import { PluginHostProvider, WidgetRenderer } from "@hollis-labs/plugin-host-ui/react"
import type { KindDescriptor, RegionDescriptor } from "@hollis-labs/plugin-registry"
import { useEffect, useState, useSyncExternalStore, version } from "react"
import { createPresentationComposition, memoryLayoutStorage } from "./vendor/chimera/composition.js"
import { store } from "./vendor/chimera/store.js"
import { leasedStylesheets } from "./vendor/chimera/stylesheets.js"

type App = ReturnType<typeof createPresentationComposition>
const kinds: Record<string, KindDescriptor> = {
  widget: {
    schema_version: 1,
    metadata_schema: {},
    representations: ["component"],
    regions: ["recipe.summary"],
    required_capabilities: [],
  },
}
const regions: Record<string, RegionDescriptor> = {
  "recipe.summary": {
    kinds: ["widget"],
    representations: ["component"],
    context_schema: {},
    ordering: "manifest",
  },
}
function Views({ app }: { app: App }) {
  useSyncExternalStore(app.runtime.subscribe, app.runtime.getSnapshot)
  return (
    <>
      <p>Explicit reviewed main-origin fixture policy; no sandbox or provider claim.</p>
      <section aria-label="Admitted recipe widgets">
        {app.select("recipe.summary").map((view) => (
          <WidgetRenderer key={view.id} widget={view} />
        ))}
      </section>
      <Button
        onClick={() => {
          void app.registry.unload("recipe")
        }}
      >
        Unload reviewed widget
      </Button>
    </>
  )
}
export function Plugin() {
  const [app, setApp] = useState<App>(),
    [error, setError] = useState("")
  useEffect(() => {
    let alive = true
    const abort = new AbortController(),
      leases = createStylesheetLeases(document)
    const sink = leasedStylesheets(leases, (_key, url) =>
      url === "/plugins/recipe/g1/style.css" ? { owner: "recipe", generation: "g1" } : undefined,
    )
    const scope = { appId: "recipe-shell", environmentId: "offline", clientId: "browser" }
    const next = createPresentationComposition({
      scope,
      registryOptions: { kinds, regions, runtimes: { react: version }, stylesheets: sink },
      routes: [
        { id: "overview", label: "Overview", path: "/", region: "recipe.summary" },
        { id: "evidence", label: "Evidence", path: "/evidence", region: "recipe.summary" },
      ],
      storage: memoryLayoutStorage(),
      isolation: store({
        appId: scope.appId,
        effectiveMode: "main-origin" as const,
        revision: "reviewed-recipe-g1",
      }),
      renderContext: store({ source: "authored-shell/v1" }),
      catalog: {
        reserved: (ref) => ref.owner === "host",
        kinds: [
          {
            kind: "widget",
            schemaVersion: 1,
            role: "widget",
            representations: ["component"],
            regions: ["recipe.summary"],
            validate: (
              entry: RegistryEntry,
            ): entry is RegistryEntry & { metadata: { label: string } } =>
              !!entry.metadata &&
              typeof entry.metadata === "object" &&
              Object.keys(entry.metadata).length === 1 &&
              "label" in entry.metadata &&
              entry.metadata.label === "Reviewed local widget",
            project: () => ({
              label: "Reviewed local widget",
              region: "recipe.summary",
              manifestOrder: 0,
            }),
          },
        ],
        regions: [
          {
            name: "recipe.summary",
            representation: "component",
            kinds: ["widget"],
            widgetKinds: ["widget"],
            ordering: "manifest",
          },
        ],
      },
    })
    setApp(next)
    void next.load("/plugins/registry", abort.signal).catch(() => {
      if (alive && !abort.signal.aborted) setError("Reviewed registry unavailable")
    })
    return () => {
      alive = false
      abort.abort()
      void next.dispose().finally(() => {
        sink.dispose()
        leases.dispose()
      })
    }
  }, [])
  return (
    <section className="folio-plugin" aria-label="Optional plugin recipe">
      {error ? (
        <p role="alert">{error}</p>
      ) : app ? (
        <PluginHostProvider runtime={app.runtime}>
          <Views app={app} />
        </PluginHostProvider>
      ) : (
        <p>Loading reviewed fixture</p>
      )}
    </section>
  )
}
