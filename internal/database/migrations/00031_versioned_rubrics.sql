-- +goose Up
-- Provisional development slot; verify the real preceding-schema upgrade on the final publication base.
CREATE TABLE event_rubrics (
 event_id uuid PRIMARY KEY REFERENCES events(id),
 id uuid NOT NULL,
 revision bigint NOT NULL DEFAULT 0 CHECK(revision>=0),
 latest_version bigint NOT NULL DEFAULT 0 CHECK(latest_version>=0),
 approved_version bigint,
 published_version bigint,
 UNIQUE(event_id,id)
);
CREATE TABLE rubric_versions (
 event_id uuid NOT NULL,
 rubric_id uuid NOT NULL,
 version bigint NOT NULL CHECK(version>0),
 document jsonb NOT NULL,
 public_document jsonb NOT NULL,
 public_digest text NOT NULL,
 actor_id uuid NOT NULL REFERENCES people(id),
 created_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,rubric_id,version),
 FOREIGN KEY(event_id,rubric_id) REFERENCES event_rubrics(event_id,id)
);
CREATE TABLE rubric_approvals (
 event_id uuid NOT NULL,
 rubric_id uuid NOT NULL,
 version bigint NOT NULL,
 actor_id uuid NOT NULL REFERENCES people(id),
 reason text NOT NULL,
 approved_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,rubric_id,version),
 FOREIGN KEY(event_id,rubric_id,version) REFERENCES rubric_versions(event_id,rubric_id,version)
);
ALTER TABLE event_rubrics ADD CONSTRAINT rubric_approved_version FOREIGN KEY(event_id,id,approved_version) REFERENCES rubric_approvals(event_id,rubric_id,version);
ALTER TABLE event_rubrics ADD CONSTRAINT rubric_published_version FOREIGN KEY(event_id,id,published_version) REFERENCES rubric_approvals(event_id,rubric_id,version);
CREATE TABLE rubric_impact_reviews (
 event_id uuid NOT NULL,
 rubric_id uuid NOT NULL,
 version bigint NOT NULL,
 impact_review text NOT NULL,
 notice text NOT NULL,
 actor_id uuid NOT NULL REFERENCES people(id),
 reviewed_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,rubric_id,version),
 FOREIGN KEY(event_id,rubric_id,version) REFERENCES rubric_approvals(event_id,rubric_id,version)
);
CREATE TABLE rubric_publications (
 event_id uuid NOT NULL,
 rubric_id uuid NOT NULL,
 version bigint NOT NULL,
 challenge_id uuid NOT NULL,
 challenge_version bigint NOT NULL,
 public_digest text NOT NULL,
 published_at timestamptz NOT NULL,
 reviewed_notice text NOT NULL DEFAULT '',
 PRIMARY KEY(event_id,challenge_id,challenge_version),
 FOREIGN KEY(event_id,rubric_id,version) REFERENCES rubric_approvals(event_id,rubric_id,version),
 FOREIGN KEY(event_id,challenge_id,challenge_version) REFERENCES content_versions(event_id,content_id,version)
);
CREATE TABLE rubric_notice_recipients (
 event_id uuid NOT NULL,
 challenge_id uuid NOT NULL,
 challenge_version bigint NOT NULL,
 person_id uuid NOT NULL REFERENCES people(id),
 PRIMARY KEY(event_id,challenge_id,challenge_version,person_id),
 FOREIGN KEY(event_id,challenge_id,challenge_version) REFERENCES rubric_publications(event_id,challenge_id,challenge_version)
);
CREATE TABLE rubric_assessment_runs (
 event_id uuid NOT NULL,
 id uuid NOT NULL,
 rubric_id uuid NOT NULL,
 rubric_version bigint NOT NULL,
 purpose text NOT NULL CHECK(purpose IN ('checkpoint','mentor_final','judge_final')),
 status text NOT NULL CHECK(status IN ('current','superseded')),
 previous_id uuid,
 reason text NOT NULL,
 actor_id uuid NOT NULL REFERENCES people(id),
 created_at timestamptz NOT NULL,
 started_at timestamptz,
 PRIMARY KEY(event_id,id),
 FOREIGN KEY(event_id,rubric_id,rubric_version) REFERENCES rubric_approvals(event_id,rubric_id,version),
 FOREIGN KEY(event_id,previous_id) REFERENCES rubric_assessment_runs(event_id,id)
);
CREATE UNIQUE INDEX rubric_current_run ON rubric_assessment_runs(event_id,purpose) WHERE status='current';
-- +goose StatementBegin
CREATE FUNCTION protect_rubric_run_identity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF ROW(NEW.event_id,NEW.id,NEW.rubric_id,NEW.rubric_version,NEW.purpose,NEW.previous_id,NEW.reason,NEW.actor_id,NEW.created_at)
 IS DISTINCT FROM ROW(OLD.event_id,OLD.id,OLD.rubric_id,OLD.rubric_version,OLD.purpose,OLD.previous_id,OLD.reason,OLD.actor_id,OLD.created_at)
 OR (OLD.started_at IS NOT NULL AND NEW.started_at IS DISTINCT FROM OLD.started_at)
 OR (OLD.status='superseded' AND NEW.status<>'superseded') THEN
  RAISE EXCEPTION 'Assessment run identity and frozen criteria are immutable';
 END IF;
 RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER rubric_runs_retained BEFORE DELETE OR TRUNCATE ON rubric_assessment_runs FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER rubric_run_identity BEFORE UPDATE ON rubric_assessment_runs FOR EACH ROW EXECUTE FUNCTION protect_rubric_run_identity();
CREATE TABLE rubric_changes (
 event_id uuid NOT NULL,
 rubric_id uuid NOT NULL,
 revision bigint NOT NULL,
 version bigint NOT NULL,
 action text NOT NULL CHECK(action IN ('saved','approved','impact_reviewed','published','run_created','run_restarted')),
 actor_id uuid REFERENCES people(id),
 reason text NOT NULL,
 occurred_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,rubric_id,revision),
 FOREIGN KEY(event_id,rubric_id,version) REFERENCES rubric_versions(event_id,rubric_id,version)
);
CREATE TRIGGER rubric_versions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON rubric_versions FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER rubric_approvals_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON rubric_approvals FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER rubric_reviews_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON rubric_impact_reviews FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER rubric_publications_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON rubric_publications FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER rubric_notices_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON rubric_notice_recipients FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER rubric_changes_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON rubric_changes FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();

-- +goose Down
DROP TABLE rubric_changes;
DROP TABLE rubric_assessment_runs;
DROP FUNCTION protect_rubric_run_identity();
DROP TABLE rubric_notice_recipients;
DROP TABLE rubric_publications;
DROP TABLE rubric_impact_reviews;
ALTER TABLE event_rubrics DROP CONSTRAINT rubric_published_version;
ALTER TABLE event_rubrics DROP CONSTRAINT rubric_approved_version;
DROP TABLE rubric_approvals;
DROP TABLE rubric_versions;
DROP TABLE event_rubrics;
