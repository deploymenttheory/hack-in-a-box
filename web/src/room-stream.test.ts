import maximumFrame from '../e2e/fixtures/max-display-frame.sse?raw'
import { expect, test } from 'vitest'
import { consumeFrames, parseSnapshot, safeDestination } from './room-stream'

test('decodes SSE across split CRLF, UTF-8 and frame boundaries without conflating snapshot revisions',async()=>{
 const bytes=new TextEncoder().encode('id: 7\r\nevent: snapshot\r\ndata: café\r\n\r\nid: 7\nevent: snapshot\ndata: removed\n\nevent: revoked\ndata: {}\n\n')
 const response=new Response(new ReadableStream({start(controller){for(const byte of bytes)controller.enqueue(new Uint8Array([byte]));controller.close()}}),{headers:{'Content-Type':'text/event-stream'}})
 const frames:string[][]=[]
 await consumeFrames(response,new AbortController().signal,(event,data)=>frames.push([event,data]))
 expect(frames).toEqual([['snapshot','café'],['snapshot','removed'],['revoked','{}']])
})
test('rejects oversized or non-stream responses without rendering their data',async()=>{
 let rendered=false
 await expect(consumeFrames(new Response('x'.repeat(600_000),{headers:{'Content-Type':'text/event-stream'}}),new AbortController().signal,()=>{rendered=true})).rejects.toThrow('oversized_display_frame')
 await expect(consumeFrames(new Response('<html>not a display</html>'),new AbortController().signal,()=>{rendered=true})).rejects.toThrow('invalid_display_stream')
 expect(rendered).toBe(false)
})
test('canonical room destinations exclude executable URLs and embedded credentials',()=>{
 expect(safeDestination('https://EXAMPLE.com:443/challenge')).toBe('https://example.com/challenge')
 for(const value of ['javascript:alert(1)','data:text/html,test','http://example.com','https://name:password@example.com','/relative'])expect(safeDestination(value)).toBeUndefined()
})

test('accepts a coalesced network chunk of individually bounded frames',async()=>{
 let frames=0
 await consumeFrames(new Response('event: snapshot\ndata: safe\n\n'.repeat(2000),{headers:{'Content-Type':'text/event-stream'}}),new AbortController().signal,()=>{frames++})
 expect(frames).toBe(2000)
})

test('accepts Go JSON HTML escaping at the maximum legal current and next cue sizes',async()=>{
 const frame=maximumFrame
 expect(frame.length).toBeGreaterThan(78_000)
 let parsed=false
 await consumeFrames(new Response(frame,{headers:{'Content-Type':'text/event-stream'}}),new AbortController().signal,(event,data)=>{
  expect(event).toBe('snapshot')
  const snapshot=parseSnapshot(JSON.parse(data),'00000000-0000-4000-8000-000000000001')
  expect(snapshot.current?.text).toBe('<'.repeat(4000));expect(snapshot.next?.text).toBe('<'.repeat(4000));parsed=true
 })
 expect(parsed).toBe(true)
})

test('preserves bounded published results with maximum legal escaped names beside both cues', async () => {
 const eventID = '00000000-0000-4000-8000-000000000001'
 const base = parseSnapshot(JSON.parse(maximumFrame.match(/^data: (.*)$/m)![1]), eventID)
 const id = (index: number) => `00000000-0000-4000-8000-${index.toString(16).padStart(12, '0')}`
 const team = (index: number) => ({ id: id(index), name: '<'.repeat(160) })
 const finalist = { id: id(900), kind: 'finalists', version: 1, finalists: Array.from({length:100}, (_,i)=>team(i+1)), reserves: Array.from({length:100}, (_,i)=>team(i+101)), awards: [], published_at: base.server_now }
 const awards = { id: id(901), kind: 'awards', version: 2, finalists: [], reserves: [], awards: Array.from({length:50}, (_,i)=>({ prize_id:id(i+500), name:'<'.repeat(160), category_id:id(800), team:team(1), amount:'9'.repeat(64), currency:'GBP' })), published_at:base.server_now }
 const payload = {...base, results:[finalist,awards]}
 // Match Go encoding/json HTML escaping; these are legal authored names.
 const escaped = JSON.stringify(payload).replace(/[<>&]/g, c => `\\u${c.charCodeAt(0).toString(16).padStart(4,'0')}`)
 const frame = `id: 7\nevent: snapshot\ndata: ${escaped}\n\n`
 expect(frame.length).toBeGreaterThan(128*1024)
 expect(frame.length).toBeLessThan(512*1024)
 let received = false
 await consumeFrames(new Response(frame,{headers:{'Content-Type':'text/event-stream'}}),new AbortController().signal,(_,data)=>{
  const safe = parseSnapshot(JSON.parse(data),eventID)
  expect(safe.results?.map(r=>r.kind)).toEqual(['finalists','awards'])
  expect(safe.results?.[0].finalists).toHaveLength(100)
  expect(safe.results?.[0].reserves).toHaveLength(100)
  expect(safe.results?.[1].awards).toHaveLength(50)
  expect(safe.results?.[1].awards[0].amount).toBe('9'.repeat(64))
  received = true
 })
 expect(received).toBe(true)
 expect(()=>parseSnapshot({...payload,results:[{...awards,reason:'PRIVATE-BALLOT-PROSE'}]},eventID)).toThrow()
 expect(()=>parseSnapshot({...payload,results:[finalist,awards,awards]},eventID)).toThrow()
})
