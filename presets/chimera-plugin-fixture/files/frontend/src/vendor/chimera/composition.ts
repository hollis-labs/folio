import { createSlotCatalog, createPluginLayoutStore, orderContributions, type SlotCatalogDefinitions, type HostScope, type PluginHostAdapter, type LayoutStorage, type ContributionView } from '@hollis-labs/plugin-host-ui'
import type { PluginRegistryOptions, PluginRegistryResponse } from '@hollis-labs/plugin-registry'
import {createPresentationHost} from './runtime.js'
import {store} from './store.js'

/** App-owned destinations. No plugin may register a global route or viewport. */
export interface PresentationRoute {id:string;label:string;path:string;region:string}
export function memoryLayoutStorage():LayoutStorage {
 const values=new Map<string,string>()
 return {read:key=>values.get(key)??null,write:(key,value)=>{values.set(key,value)},remove:key=>{values.delete(key)}}
}
export interface CompositionOptions {
 frameController?:PluginHostAdapter<PluginRegistryResponse>['frameController']
 scope:HostScope
 registryOptions:PluginRegistryOptions
 catalog:SlotCatalogDefinitions
 routes:readonly PresentationRoute[]
 storage:LayoutStorage
 isolation:PluginHostAdapter<PluginRegistryResponse>['isolation']
 renderContext:PluginHostAdapter<PluginRegistryResponse>['renderContext']
 actions?:PluginHostAdapter<PluginRegistryResponse>['actions']
 diagnostics?:PluginHostAdapter<PluginRegistryResponse>['diagnostics']
}
/** Thin integration of shared catalogue, registry, ordering and layout stores.
 * App policy remains explicit; no default catalogue, route vocabulary or loader. */
export function createPresentationComposition(options:CompositionOptions){
 const catalog=createSlotCatalog(options.catalog)
 const routes=Object.freeze(options.routes.map(route=>Object.freeze({...route})))
 const ids=new Set<string>(),paths=new Set<string>()
 for(const route of routes){
  if(!route.id.trim()||!route.label.trim()||!/^\/(?:[A-Za-z0-9_-]+(?:\/[A-Za-z0-9_-]+)*)?$/.test(route.path)||ids.has(route.id)||paths.has(route.path)||!catalog.regions.some(region=>region.name===route.region))throw new Error('Invalid host route policy')
  ids.add(route.id);paths.add(route.path)
 }
 const diagnostics=options.diagnostics??(()=>{})
 const layouts=new Map(catalog.regions.map(region=>[region.name,createPluginLayoutStore(options.storage,options.scope,region.name,1,diagnostics)]))
 const panels=store<readonly ContributionView[]>([])
 const app=createPresentationHost(options.registryOptions,{scope:options.scope,catalog,isolation:options.isolation,renderContext:options.renderContext,actions:options.actions,frameController:options.frameController,diagnostics,
 panels:{reconcile(_scope,views){panels.set(Object.freeze([...views]))},releaseScope(){panels.set([])}}})
 const refresh=()=>{for(const [region,layout] of layouts){const active=app.runtime.getSnapshot().views.filter(view=>view.region===region&&app.runtime.isCurrent(view));layout.reconcile(active.map(view=>view.id))}}
 const releaseRetain=app.runtime.retain()
 const release=app.runtime.subscribe(refresh)
 function select(region:string){const layout=layouts.get(region);if(!layout)return [];
  const snapshot=layout.getSnapshot();return orderContributions(app.runtime.getSnapshot().views.filter(view=>view.region===region&&view.availability==='available'&&app.runtime.isCurrent(view)),snapshot.order,catalog.ordering(region)).filter(view=>snapshot.visibility[view.id]!==false)
 }
 return {...app,catalog,routes,layouts,panels,select,route:(id:string)=>routes.find(route=>route.id===id),async dispose(){release();releaseRetain();await app.dispose()}}
}
