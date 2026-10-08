import { createPluginRegistry, type PluginRegistryOptions, type PluginRegistryResponse } from '@hollis-labs/plugin-registry'
import { createPluginHostRuntime, type PluginHostAdapter } from '@hollis-labs/plugin-host-ui'

/** Shared wire loader + shared typed presentation runtime. Catalog, approved
 * runtime exports, action/identity policy and explicit isolation remain host inputs. */
export function createPresentationHost(options: PluginRegistryOptions, adapter: Omit<PluginHostAdapter<PluginRegistryResponse>, 'registry'>) {
 const registry=createPluginRegistry(options)
 const runtime=createPluginHostRuntime({...adapter,registry})
 return {registry,runtime,async load(url='/plugins/registry',signal?:AbortSignal){
  const response=await fetch(url,{signal,credentials:'same-origin'})
  if(!response.ok)throw new Error('Registry unavailable')
  // Preserve raw JSON so upstream rejects duplicate and noncanonical keys/tokens.
  return runtime.sync(await response.text())
 },async dispose(){runtime.dispose();await registry.clear()}}
}
