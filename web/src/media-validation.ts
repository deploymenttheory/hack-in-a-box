import { useLayoutEffect, useReducer, useState, type RefObject } from 'react'
import type { components } from './api/schema.gen'
import { focusFirstInvalid } from './components/ui/fields'
export type MediaFieldErrors=Partial<Record<'title'|'caption'|'reason'|'people'|'recording_url'|'file',string>>
export function validateMediaReason(reason:string):MediaFieldErrors{
 return !reason.trim()||new TextEncoder().encode(reason).length>2000?{reason:'Provide a reason up to 2000 UTF-8 bytes.'}:{}
}
export function validateMediaDraft(metadata:components['schemas']['MediaMetadata'],reason:string):MediaFieldErrors{
 const errors=validateMediaReason(reason),bytes=(value:string)=>new TextEncoder().encode(value).length
 if(!metadata.title.trim()||bytes(metadata.title)>200)errors.title='Provide a media title up to 200 UTF-8 bytes.'
 if(bytes(metadata.caption)>4000)errors.caption='Keep the caption within 4000 UTF-8 bytes.'
 if(metadata.people.length>100)errors.people='Tag no more than 100 identifiable participants.'
 if(metadata.kind==='recording'){try{const url=new URL(metadata.recording_url);if(url.protocol!=='https:'||url.username||url.password||/\s/.test(metadata.recording_url)||bytes(metadata.recording_url)>2048)throw new Error()}catch{errors.recording_url='Provide a valid HTTPS recording destination without credentials or spaces.'}}
 return errors
}
// Only an invalid submit requests focus. Editing/clearing errors never steals it.
// Field owns aria-invalid and the error description; transport feedback is separate.
export function useMediaValidation(root:RefObject<HTMLElement|null>){
 const [errors,setErrors]=useState<MediaFieldErrors>({}),[attempt,invalidSubmit]=useReducer(value=>value+1,0)
 useLayoutEffect(()=>{if(attempt&&root.current)focusFirstInvalid(root.current)},[attempt,root])
 return {errors,clear:()=>setErrors({}),check:(next:MediaFieldErrors)=>{setErrors(next);if(Object.keys(next).length){invalidSubmit();return false}return true}}
}
