-- Safe allowlist only: no payloads, destinations, cue/body text, ciphertext or keys.
WITH intent(kind,id,event_id,state,revision,due_at,approved,source_id,source_version,updated_at) AS (
 SELECT 'operation:'||kind,id,event_id,state,revision,due_at,approved,target_id,attempts::bigint,updated_at
 FROM operations WHERE state IN ('pending','running','ambiguous','permanent')
 UNION ALL
 SELECT 'schedule',id,event_id,'scheduled',revision,due_at,approved,operation_id,0,NULL::timestamptz
 FROM schedule_items WHERE executed_at IS NULL AND NOT cancelled
 UNION ALL
 SELECT 'smtp_test',s.operation_id,s.event_id,s.state,0,NULL::timestamptz,NULL::boolean,s.test_request_id,0,s.updated_at
 FROM smtp_message_snapshots s JOIN operations o ON o.id=s.operation_id
 WHERE o.state IN ('pending','running','ambiguous','permanent')
 UNION ALL
 SELECT 'smtp_campaign',s.operation_id,s.event_id,s.state,0,NULL::timestamptz,NULL::boolean,s.campaign_id,s.campaign_version,s.updated_at
 FROM smtp_campaign_messages s JOIN operations o ON o.id=s.operation_id
 WHERE o.state IN ('pending','running','ambiguous','permanent')
 UNION ALL
 SELECT 'email_verification',id,NULL::uuid,delivery_state,session_revision,expires_at,NULL::boolean,person_id,0,requested_at
 FROM email_verifications WHERE delivery_state IN ('prepared','retryable','ambiguous')
 UNION ALL
 SELECT 'repository',team_id,event_id,state,0,NULL::timestamptz,NULL::boolean,provenance,0,updated_at
 FROM team_repositories WHERE state<>'ready'
 UNION ALL
 SELECT 'repository_access',e.id,e.event_id,e.state,0,NULL::timestamptz,p.approved_at IS NOT NULL,e.plan_id,p.version,e.updated_at
 FROM repository_access_effects e JOIN repository_access_plans p ON p.id=e.plan_id
 WHERE e.state IN ('planned','ambiguous','failed')
 UNION ALL
 SELECT 'communication_occurrence',id,event_id,state,review_revision,due_at,NULL::boolean,binding_id,binding_version,updated_at
 FROM communication_occurrences WHERE state IN ('pending','paused','blocked','queued','partially_queued')
 UNION ALL
 SELECT 'outcome_review',h.id,h.event_id,h.state,h.current_version,(v.proof->>'due_at')::timestamptz,true,h.outcome_id,(v.document->>'source_version')::bigint,v.created_at
 FROM outcome_review_heads h JOIN outcome_review_versions v ON v.review_id=h.id AND v.version=h.current_version WHERE h.state='approved'
 UNION ALL
 SELECT 'content_release',id,event_id,state,revision,scheduled_at,true,id,version,released_at
 FROM content_items WHERE state='approved' AND scheduled_at IS NOT NULL
)
SELECT COALESCE(jsonb_agg(jsonb_build_object('kind',kind,'id',id,'event_id',event_id,'state',state,'revision',revision,'due_at',due_at,'approved',approved,'source_id',source_id,'source_version',CASE WHEN kind LIKE 'operation:%' THEN 0 ELSE source_version END,'attempts',CASE WHEN kind LIKE 'operation:%' THEN source_version ELSE 0 END,'updated_at',updated_at) ORDER BY kind,id),'[]'::jsonb)::text FROM intent;
