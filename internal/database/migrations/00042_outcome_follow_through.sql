-- +goose Up
ALTER TABLE event_privacy ADD COLUMN revision bigint NOT NULL DEFAULT 0 CHECK(revision>=0);
CREATE TABLE outcome_heads (
 id uuid PRIMARY KEY,
 author_id uuid NOT NULL REFERENCES people(id),
 event_id uuid REFERENCES events(id),
 team_id uuid,
 owner_id uuid REFERENCES people(id),
 status text NOT NULL CHECK(status IN ('proposed','reviewing','accepted','in_progress','closed')),
 current_version bigint NOT NULL DEFAULT 0 CHECK(current_version>=0),
 created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(event_id,team_id) REFERENCES hack_teams(event_id,id),
 CHECK(team_id IS NULL OR event_id IS NOT NULL)
);
CREATE INDEX outcome_heads_author ON outcome_heads(author_id,id);
CREATE INDEX outcome_heads_owner ON outcome_heads(owner_id,id);
CREATE INDEX outcome_heads_event ON outcome_heads(event_id,id);
-- +goose StatementBegin
CREATE FUNCTION outcome_current_team_member(e uuid,t uuid,actor uuid) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT EXISTS(SELECT 1 FROM hack_team_members tm JOIN hack_teams ht ON ht.event_id=tm.event_id AND ht.id=tm.team_id AND ht.state='published' JOIN competition_claims cc ON cc.event_id=tm.event_id AND cc.person_id=tm.person_id AND cc.state='confirmed' JOIN people p ON p.id=tm.person_id AND p.active WHERE tm.event_id=e AND tm.team_id=t AND tm.person_id=actor AND tm.snapshot='published');
$$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION outcome_eligible_owner(target uuid,owner uuid) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT EXISTS(SELECT 1 FROM outcome_heads h JOIN people p ON p.id=owner AND p.active
 WHERE h.id=target AND (h.author_id=owner
 OR EXISTS(SELECT 1 FROM event_memberships m WHERE m.event_id=h.event_id AND m.person_id=owner AND m.role='organiser' AND m.active)
 OR outcome_current_team_member(h.event_id,h.team_id,owner)));
$$;
-- +goose StatementEnd
CREATE TABLE outcome_versions (
 outcome_id uuid NOT NULL REFERENCES outcome_heads(id),
 version bigint NOT NULL CHECK(version>0),
 command_id uuid NOT NULL,
 actor_id uuid NOT NULL REFERENCES people(id),
 expected_version bigint NOT NULL CHECK(expected_version>=0),
 command jsonb NOT NULL,
 document jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(outcome_id,version),
 UNIQUE(outcome_id,command_id)
);
CREATE TRIGGER outcome_versions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON outcome_versions FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
-- +goose StatementBegin
CREATE FUNCTION outcome_readable(target uuid,actor uuid,at_time timestamptz) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT EXISTS(
 SELECT 1 FROM outcome_heads h JOIN people p ON p.id=actor AND p.active
 LEFT JOIN events e ON e.id=h.event_id LEFT JOIN event_privacy ep ON ep.event_id=e.id
 LEFT JOIN outcome_versions v ON v.outcome_id=h.id AND v.version=h.current_version
 WHERE h.id=target
 AND (h.event_id IS NULL OR NOT COALESCE((v.document->'definition'->>'theme_sensitive')::boolean,false)
 OR EXISTS(SELECT 1 FROM event_memberships m WHERE m.event_id=h.event_id AND m.person_id=actor AND m.role='organiser' AND m.active)
 OR e.published_at IS NOT NULL AND at_time>=e.published_at)
 AND (v.document->'definition'->>'kind' IS DISTINCT FROM 'team_outcome'
 OR e.published_at IS NOT NULL AND at_time>=e.published_at AND at_time>=(e.configuration->>'live_start')::timestamptz)
 AND (e.completed_at IS NULL OR at_time<e.completed_at+make_interval(days=>COALESCE(ep.retention_days,90)))
 AND (
  h.owner_id=actor AND outcome_eligible_owner(h.id,actor)
  OR (h.author_id=actor AND h.team_id IS NULL)
  OR EXISTS(SELECT 1 FROM event_memberships m WHERE m.person_id=actor AND m.role='organiser' AND m.active AND (m.event_id=h.event_id OR h.event_id IS NULL))
  OR outcome_current_team_member(h.event_id,h.team_id,actor)
 )
 );
$$;
-- +goose StatementEnd
CREATE TABLE outcome_evidence (
 outcome_id uuid NOT NULL,
 id uuid NOT NULL,
 outcome_version bigint NOT NULL,
 actor_id uuid NOT NULL REFERENCES people(id),
 document jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(outcome_id,id),
 FOREIGN KEY(outcome_id,outcome_version) REFERENCES outcome_versions(outcome_id,version)
);
CREATE TRIGGER outcome_evidence_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON outcome_evidence FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TABLE outcome_project_heads (
 outcome_id uuid PRIMARY KEY REFERENCES outcome_heads(id),
 current_version bigint NOT NULL DEFAULT 0 CHECK(current_version>=0),
 published_version bigint,
 CHECK(published_version IS NULL OR published_version>0)
);
CREATE TABLE outcome_project_versions (
 outcome_id uuid NOT NULL REFERENCES outcome_project_heads(outcome_id),
 version bigint NOT NULL CHECK(version>0),
 source_version bigint NOT NULL,
 command_id uuid NOT NULL,
 actor_id uuid NOT NULL REFERENCES people(id),
 expected_version bigint NOT NULL CHECK(expected_version>=0),
 command jsonb NOT NULL,
 document jsonb NOT NULL,
 state text NOT NULL CHECK(state IN ('draft','published','withdrawn')),
 published_fields jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(outcome_id,version),
 UNIQUE(outcome_id,command_id),
 FOREIGN KEY(outcome_id,source_version) REFERENCES outcome_versions(outcome_id,version)
);
CREATE TRIGGER outcome_project_versions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON outcome_project_versions FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
-- +goose StatementBegin
CREATE FUNCTION outcome_project_readable(target uuid,actor uuid,at_time timestamptz) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT EXISTS(
 SELECT 1 FROM outcome_project_heads ph
 JOIN outcome_project_versions pv ON pv.outcome_id=ph.outcome_id AND pv.version=ph.published_version AND pv.state='published'
 JOIN outcome_heads h ON h.id=ph.outcome_id
 JOIN outcome_versions current_source ON current_source.outcome_id=h.id AND current_source.version=h.current_version
 JOIN events e ON e.id=h.event_id
 JOIN people p ON p.id=actor AND p.active
 WHERE ph.outcome_id=target
 AND (NOT COALESCE((current_source.document->'definition'->>'theme_sensitive')::boolean,false) OR e.published_at IS NOT NULL AND at_time>=e.published_at)
 AND (current_source.document->'definition'->>'kind' IS DISTINCT FROM 'team_outcome' OR e.published_at IS NOT NULL AND at_time>=e.published_at AND at_time>=(e.configuration->>'live_start')::timestamptz)
 AND (
  pv.document->'fields'->>'audience'='organisation'
  OR pv.document->'fields'->>'audience'='event' AND (
   p.platform_admin OR EXISTS(SELECT 1 FROM event_memberships m WHERE m.event_id=e.id AND m.person_id=actor AND m.active)
   OR EXISTS(SELECT 1 FROM competition_claims cc WHERE cc.event_id=e.id AND cc.person_id=actor AND cc.state='confirmed'))
  OR pv.document->'fields'->>'audience'='team' AND outcome_current_team_member(h.event_id,h.team_id,actor)
 )
 );
$$;
-- +goose StatementEnd
CREATE TABLE outcome_review_heads (
 id uuid PRIMARY KEY,
 event_id uuid NOT NULL REFERENCES events(id),
 outcome_id uuid NOT NULL REFERENCES outcome_heads(id),
 days integer NOT NULL CHECK(days IN (30,60,90)),
 current_version bigint NOT NULL DEFAULT 0 CHECK(current_version>=0),
 state text NOT NULL CHECK(state IN ('draft','approved','cancelled')),
 UNIQUE(outcome_id,days),
 UNIQUE(event_id,id)
);
CREATE INDEX outcome_review_event ON outcome_review_heads(event_id,id);
CREATE TABLE outcome_review_versions (
 review_id uuid NOT NULL REFERENCES outcome_review_heads(id),
 version bigint NOT NULL CHECK(version>0),
 command_id uuid NOT NULL,
 actor_id uuid NOT NULL REFERENCES people(id),
 expected_version bigint NOT NULL CHECK(expected_version>=0),
 command jsonb NOT NULL,
 document jsonb NOT NULL,
 proof jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(review_id,version),
 UNIQUE(review_id,command_id)
);
CREATE TRIGGER outcome_review_versions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON outcome_review_versions FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();

-- Native outcome approvals extend only the retained capability used by jobs;
-- the original generic retained proof remains byte-for-byte intact.
-- +goose StatementBegin
CREATE FUNCTION retained_communication_occurrence_before_outcomes(e uuid,o uuid,at_time timestamptz) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT EXISTS(
 SELECT 1 FROM communication_occurrences co
 JOIN communication_automation_versions av ON av.event_id=co.event_id AND av.id=co.binding_id AND av.version=co.binding_version
 JOIN events ev ON ev.id=co.event_id
 LEFT JOIN event_privacy ep ON ep.event_id=ev.id
 JOIN communication_facts f ON f.event_id=co.event_id AND f.id=co.source_id AND f.revision=co.source_revision
 JOIN people recipient ON recipient.id=f.owner_id AND recipient.active
 JOIN event_memberships authority ON authority.event_id=av.event_id AND authority.person_id=av.actor_id AND authority.role='organiser' AND authority.active
 JOIN people approver ON approver.id=av.actor_id AND approver.active
 WHERE co.event_id=e AND co.id=o AND co.source_kind='outcome.follow_up'
 AND co.state IN ('pending','queued','partially_queued') AND COALESCE(co.review_until,co.deadline)>=at_time
 AND av.version=(SELECT max(v.version) FROM communication_automation_versions v WHERE v.event_id=e AND v.id=av.id)
 AND COALESCE((av.document->>'cancelled')::boolean,false)=false AND av.document->>'approved_at' IS NOT NULL
 AND av.document->'definition'->>'trigger'='outcome.follow_up'
 AND (av.document->'definition'->>'retain_after_archive')::boolean=true
 AND (av.document->'definition'->>'retain_until')::timestamptz>at_time
 AND ev.stage IN ('completed','archived') AND ev.completed_at IS NOT NULL
 AND (av.document->'definition'->>'retain_until')::timestamptz<=ev.completed_at+make_interval(days=>COALESCE(ep.retention_days,90))
 AND at_time<ev.completed_at+make_interval(days=>COALESCE(ep.retention_days,90))
 AND f.kind='outcome_review' AND f.status='open' AND f.owner_id IS NOT NULL
 );
$$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION outcome_review_sendable(e uuid,r uuid,v bigint,at_time timestamptz) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT EXISTS(
 SELECT 1 FROM outcome_review_heads rh
 JOIN outcome_review_versions rv ON rv.review_id=rh.id AND rv.version=rh.current_version
 JOIN outcome_heads oh ON oh.id=rh.outcome_id
 JOIN events ev ON ev.id=rh.event_id
 LEFT JOIN event_privacy ep ON ep.event_id=ev.id
 JOIN people owner ON owner.id=oh.owner_id AND owner.active
 JOIN event_memberships authority ON authority.event_id=ev.id AND authority.person_id=rv.actor_id AND authority.role='organiser' AND authority.active
 JOIN people approver ON approver.id=rv.actor_id AND approver.active
 WHERE rh.event_id=e AND rh.id=r AND rh.current_version=v AND rh.state='approved'
 AND rv.document->>'state'='approved'
 AND oh.current_version=(rv.proof->>'outcome_version')::bigint
 AND oh.status IN ('accepted','in_progress')
 AND oh.owner_id=(rv.proof->>'owner_id')::uuid
 AND outcome_eligible_owner(oh.id,oh.owner_id)
 AND outcome_readable(oh.id,oh.owner_id,at_time)
 AND COALESCE(ep.revision,0)=(rv.proof->>'privacy_revision')::bigint
 AND (rv.proof->>'retain_until')::timestamptz>at_time
 );
$$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION retained_communication_occurrence(e uuid,o uuid,at_time timestamptz) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT CASE WHEN EXISTS(
 SELECT 1 FROM communication_occurrences co JOIN communication_automation_versions av ON av.event_id=co.event_id AND av.id=co.binding_id AND av.version=co.binding_version
 WHERE co.event_id=e AND co.id=o AND av.document->'native_outcome_approval' IS NOT NULL AND av.document->'native_outcome_approval'<>'null'::jsonb
 ) THEN
 retained_communication_occurrence_before_outcomes(e,o,at_time) AND EXISTS(
 SELECT 1 FROM communication_occurrences co JOIN communication_automation_versions av ON av.event_id=co.event_id AND av.id=co.binding_id AND av.version=co.binding_version
 JOIN outcome_review_versions rv ON rv.review_id=co.source_id AND rv.version=co.source_revision
 WHERE co.event_id=e AND co.id=o
 AND av.document->'native_outcome_approval'->>'kind'='native_outcome_review'
 AND av.document->'native_outcome_approval'->>'binding'=rv.proof->>'binding'
 AND outcome_review_sendable(e,co.source_id,co.source_revision,at_time)
 ) ELSE retained_communication_occurrence_before_outcomes(e,o,at_time) END;
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM outcome_heads) OR EXISTS(SELECT 1 FROM outcome_versions) OR EXISTS(SELECT 1 FROM outcome_project_versions) OR EXISTS(SELECT 1 FROM outcome_review_versions) THEN
  RAISE EXCEPTION 'Outcome, publication or approved review history exists; preserve it before rollback';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE outcome_review_versions;
DROP TABLE outcome_review_heads;
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION retained_communication_occurrence(e uuid,o uuid,at_time timestamptz) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT EXISTS(
 SELECT 1 FROM communication_occurrences co
 JOIN communication_automation_versions av ON av.event_id=co.event_id AND av.id=co.binding_id AND av.version=co.binding_version
 JOIN events ev ON ev.id=co.event_id
 LEFT JOIN event_privacy ep ON ep.event_id=ev.id
 JOIN communication_facts f ON f.event_id=co.event_id AND f.id=co.source_id AND f.revision=co.source_revision
 JOIN people recipient ON recipient.id=f.owner_id AND recipient.active
 JOIN event_memberships authority ON authority.event_id=av.event_id AND authority.person_id=av.actor_id AND authority.role='organiser' AND authority.active
 JOIN people approver ON approver.id=av.actor_id AND approver.active
 WHERE co.event_id=e AND co.id=o AND co.source_kind='outcome.follow_up'
 AND co.state IN ('pending','queued','partially_queued') AND COALESCE(co.review_until,co.deadline)>=at_time
 AND av.version=(SELECT max(v.version) FROM communication_automation_versions v WHERE v.event_id=e AND v.id=av.id)
 AND COALESCE((av.document->>'cancelled')::boolean,false)=false AND av.document->>'approved_at' IS NOT NULL
 AND av.document->'definition'->>'trigger'='outcome.follow_up'
 AND (av.document->'definition'->>'retain_after_archive')::boolean=true
 AND (av.document->'definition'->>'retain_until')::timestamptz>at_time
 AND ev.stage IN ('completed','archived') AND ev.completed_at IS NOT NULL
 AND (av.document->'definition'->>'retain_until')::timestamptz<=ev.completed_at+make_interval(days=>COALESCE(ep.retention_days,90))
 AND at_time<ev.completed_at+make_interval(days=>COALESCE(ep.retention_days,90))
 AND f.kind='outcome_review' AND f.status='open' AND f.owner_id IS NOT NULL
 );
$$;
-- +goose StatementEnd
DROP FUNCTION retained_communication_occurrence_before_outcomes(uuid,uuid,timestamptz);
DROP FUNCTION outcome_review_sendable(uuid,uuid,bigint,timestamptz);
DROP FUNCTION outcome_project_readable(uuid,uuid,timestamptz);
DROP TABLE outcome_project_versions;
DROP TABLE outcome_project_heads;
DROP TABLE outcome_evidence;
DROP FUNCTION outcome_readable(uuid,uuid,timestamptz);
DROP TABLE outcome_versions;
DROP FUNCTION outcome_eligible_owner(uuid,uuid);
DROP FUNCTION outcome_current_team_member(uuid,uuid,uuid);
DROP TABLE outcome_heads;
ALTER TABLE event_privacy DROP COLUMN revision;
