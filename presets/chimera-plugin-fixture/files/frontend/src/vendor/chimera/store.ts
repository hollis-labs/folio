import type { Observable } from '@hollis-labs/plugin-host-ui'
/** App-owned observable; replacing scope/invocation fences the shared action gateway. */
export function store<T>(initial: T): Observable<T> & { set(value: T): void } {
 let value=initial
 const listeners=new Set<()=>void>()
 return {getSnapshot:()=>value,getServerSnapshot:()=>value,subscribe(listener){listeners.add(listener);return()=>{listeners.delete(listener)}},set(next){value=next;for(const listener of [...listeners])listener()}}
}
