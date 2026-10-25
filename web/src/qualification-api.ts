import {api,unwrap} from './api/client'
import type {QualificationCommand} from './qualification-model'

export const readQualification=(eventID:string,runID:string,signal:AbortSignal)=>api.GET('/api/v1/events/{event_id}/mentor-qualification/{run_id}',{signal,cache:'no-store',params:{path:{event_id:eventID,run_id:runID}}}).then(unwrap)
export const sendQualification=(eventID:string,runID:string,revision:number,csrf:string,body:QualificationCommand)=>api.POST('/api/v1/events/{event_id}/mentor-qualification/{run_id}',{params:{path:{event_id:eventID,run_id:runID},header:{'If-Match':`"${revision}"`,'X-CSRF-Token':csrf}},body}).then(unwrap)
export const originalQualificationReceipt=(eventID:string,runID:string,requestID:string,signal:AbortSignal)=>api.GET('/api/v1/events/{event_id}/mentor-qualification/{run_id}/commands/{request_id}',{signal,cache:'no-store',params:{path:{event_id:eventID,run_id:runID,request_id:requestID}}}).then(unwrap)
export const readQualificationHistory=(eventID:string,runID:string,signal:AbortSignal,before?:number)=>api.GET('/api/v1/events/{event_id}/mentor-qualification/{run_id}/history',{signal,cache:'no-store',params:{path:{event_id:eventID,run_id:runID},query:{before,limit:20}}}).then(unwrap)
