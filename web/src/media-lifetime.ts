import type { components } from './api/schema.gen'
import { PrivateRequestScope } from './private-request-scope'
type Item=components['schemas']['MediaItem']
type Page=components['schemas']['MediaPage']
// Lifetime fences follow actual current projections, independently of React's
// render scheduling. These flags never replace native authorization on a read/write.
export class MediaLifetime {
 private serial=0
 private records=new Map<string,{item:Item;scope:PrivateRequestScope;epoch:number}>()
 private collection: {can_collect:boolean;revision:number}|undefined
 readonly root:PrivateRequestScope
 collector:PrivateRequestScope
 constructor(key:string,root:PrivateRequestScope,private notify:()=>void){this.root=root;this.collector=new PrivateRequestScope(`${key}:collector`,this.root)}
 acceptPage(page:Page){
  if(this.collection?.can_collect&&!page.can_collect){this.collector.invalidate();this.collector=new PrivateRequestScope(`${this.root.key}:collector:${++this.serial}`,this.root);for(const record of this.records.values())record.scope.invalidate();this.records.clear();this.notify()}
  this.collection={can_collect:page.can_collect,revision:page.revision}
  page.items.forEach(item=>this.acceptItem(item))
 }
 private expire(record:{item:Item;scope:PrivateRequestScope;epoch:number}){record.scope.invalidate();record.epoch=++this.serial;record.scope=new PrivateRequestScope(`${this.root.key}:${record.item.id}:${record.epoch}`,this.root);this.notify()}
 acceptItem(item:Item){
  const old=this.records.get(item.id)
  if(old){if((old.item.can_read_history&&!item.can_read_history)||(old.item.can_edit&&!item.can_edit)||(old.item.can_review&&!item.can_review)||(old.item.can_read_original&&!item.can_read_original)||(old.item.recording_url&&!item.recording_url))this.expire(old);old.item=item}
  else this.records.set(item.id,{item,epoch:++this.serial,scope:new PrivateRequestScope(`${this.root.key}:${item.id}:${this.serial}`,this.root)})
 }
 record(id:string){return this.records.get(id)}
 canWrite(id?:string){return !!this.collection?.can_collect&&(!id||!!this.records.get(id)?.item.can_edit)}
 deny(){this.collector.invalidate();for(const record of this.records.values())record.scope.invalidate();this.records.clear();this.collection=undefined;this.notify();this.collector=new PrivateRequestScope(`${this.root.key}:collector:${++this.serial}`,this.root)}
 end(){this.deny();this.root.invalidate()}
}
