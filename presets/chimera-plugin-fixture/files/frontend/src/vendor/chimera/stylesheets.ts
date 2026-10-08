import type {StylesheetSink} from '@hollis-labs/plugin-registry'
import type {StylesheetLeases,StylesheetOwner} from '@hollis-labs/plugin-host-ui/vite'
/** Bridges the shared registry's stylesheet hooks to upstream generation leases.
 * The host resolver binds each declared URL to a reviewed owner/generation.
 * Registry sink keys are opaque (v2 includes host epoch/owner/generation),
 * so they are never mistaken for plugin owner IDs. */
export function leasedStylesheets(leases:StylesheetLeases,reviewed:(leaseKey:string,url:string)=>StylesheetOwner|undefined):StylesheetSink & {dispose():void}{
 const owners=new Map<string,{identity:StylesheetOwner;url:string;release:()=>void}>()
 function remove(owner:string){const previous=owners.get(owner);if(previous){previous.release();leases.releaseOwner(previous.identity);owners.delete(owner)}}
 return {ensure(owner,url){const identity=reviewed(owner,url);if(!identity||!identity.owner||!identity.generation)throw new Error('Unreviewed stylesheet owner');const previous=owners.get(owner);if(previous?.identity.generation===identity.generation&&previous.url===url)return;remove(owner);owners.set(owner,{identity,url,release:leases.acquire(identity,url)})},remove,dispose(){for(const owner of [...owners.keys()])remove(owner)}}
}
