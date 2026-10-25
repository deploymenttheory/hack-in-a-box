import { api, unwrap } from './api/client'
import type { JudgeCommand, JudgePracticeCommand } from './judge-model'
export function sendJudgeCommand(eventID:string,revision:number,csrf:string,body:JudgeCommand|JudgePracticeCommand,runID?:string,teamID?:string){
 const params={path:{event_id:eventID},header:{'If-Match':`"${revision}"`,'X-CSRF-Token':csrf}}
 return runID&&teamID?api.POST('/api/v1/events/{event_id}/judging/{run_id}/teams/{team_id}',{params:{...params,path:{...params.path,run_id:runID,team_id:teamID}},body:body as JudgeCommand}).then(unwrap):api.POST('/api/v1/events/{event_id}/judge-practice',{params,body:body as JudgePracticeCommand}).then(unwrap)
}
export function originalJudgeReceipt(eventID:string,requestID:string,signal:AbortSignal){return api.GET('/api/v1/events/{event_id}/judging-receipts/{request_id}',{cache:'no-store',signal,params:{path:{event_id:eventID,request_id:requestID}}}).then(unwrap)}
export function currentJudgeStates(eventID:string,runID:string,teamID:string,signal:AbortSignal){return api.GET('/api/v1/events/{event_id}/judging/{run_id}/teams/{team_id}/states',{cache:'no-store',signal,params:{path:{event_id:eventID,run_id:runID,team_id:teamID}}}).then(unwrap)}
