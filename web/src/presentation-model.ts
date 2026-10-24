import type { components } from './api/schema.gen'
export type Cue = components['schemas']['PresentationCue']
export type ConsolePage = components['schemas']['PresentationPage']
export type CueCommand = components['schemas']['PresentationCommand']
export function emptyCue(): Cue { return { id: crypto.randomUUID(), title: '', text: '', url: '', duration_seconds: 120, content_id: null } }
export function cueErrors(cue: Cue) {
  const errors: Partial<Record<'title'|'text'|'url'|'duration_seconds',string>>={},bytes=(value:string)=>new TextEncoder().encode(value).length
  if(!cue.title.trim()||bytes(cue.title)>200)errors.title='Give this cue a title within 200 UTF-8 bytes.'
  if(bytes(cue.text)>4000)errors.text='Keep room text within 4000 UTF-8 bytes.'
  if(!Number.isInteger(cue.duration_seconds)||cue.duration_seconds<0||cue.duration_seconds>86400)errors.duration_seconds='Use whole seconds between 0 and 86400.'
  if(cue.url){try{const url=new URL(cue.url);if(url.protocol!=='https:'||!url.hostname||url.username||url.password||bytes(cue.url)>2048)errors.url='Use a complete HTTPS link within 2048 UTF-8 bytes, without a username or password.'}catch{errors.url='Enter a complete HTTPS cue link.'}}
  return errors
}
export function cueIssue(cue: Cue){return Object.values(cueErrors(cue))[0]}
export function countdown(seconds: number) { const value = Math.max(0, Math.ceil(seconds)); return `${Math.floor(value / 60).toString().padStart(2, '0')}:${(value % 60).toString().padStart(2, '0')}` }
