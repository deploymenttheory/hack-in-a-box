-- +goose Up
-- Provisional local version: renumber against the actual merged predecessor before publication.
CREATE TABLE clarification_questions (
 event_id uuid NOT NULL REFERENCES events(id),
 id uuid NOT NULL,
 team_id uuid NOT NULL,
 author_id uuid NOT NULL REFERENCES people(id),
 command_id uuid NOT NULL,
 request_digest text NOT NULL,
 topic text NOT NULL CHECK(topic IN ('challenge','rubric')),
 body text NOT NULL CHECK(octet_length(body) BETWEEN 1 AND 8000),
 attachments uuid[] NOT NULL DEFAULT '{}',
 source jsonb NOT NULL,
 revision bigint NOT NULL CHECK(revision>0),
 status text NOT NULL CHECK(status IN ('open','triaged','closed','answered')),
 created_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,id),
 UNIQUE(event_id,author_id,command_id),
 FOREIGN KEY(event_id,team_id) REFERENCES hack_teams(event_id,id)
);

CREATE TABLE clarification_answers (
 event_id uuid NOT NULL REFERENCES events(id),
 id uuid NOT NULL,
 question_id uuid NOT NULL,
 revision bigint NOT NULL CHECK(revision>0),
 latest_version bigint NOT NULL CHECK(latest_version>0),
 approved_version bigint,
 published_version bigint,
 PRIMARY KEY(event_id,id),
 UNIQUE(event_id,question_id),
 FOREIGN KEY(event_id,question_id) REFERENCES clarification_questions(event_id,id)
);

CREATE TABLE clarification_versions (
 event_id uuid NOT NULL,
 answer_id uuid NOT NULL,
 version bigint NOT NULL CHECK(version>0),
 document jsonb NOT NULL,
 source jsonb NOT NULL,
 author_id uuid NOT NULL REFERENCES people(id),
 authored_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,answer_id,version),
 FOREIGN KEY(event_id,answer_id) REFERENCES clarification_answers(event_id,id)
);

CREATE TABLE clarification_approvals (
 event_id uuid NOT NULL,
 answer_id uuid NOT NULL,
 version bigint NOT NULL,
 actor_id uuid NOT NULL REFERENCES people(id),
 approved_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,answer_id,version),
 FOREIGN KEY(event_id,answer_id,version) REFERENCES clarification_versions(event_id,answer_id,version)
);

CREATE TABLE clarification_publications (
 event_id uuid NOT NULL,
 answer_id uuid NOT NULL,
 version bigint NOT NULL,
 actor_id uuid NOT NULL REFERENCES people(id),
 published_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,answer_id,version),
 FOREIGN KEY(event_id,answer_id,version) REFERENCES clarification_approvals(event_id,answer_id,version)
);
ALTER TABLE clarification_answers ADD CONSTRAINT clarification_approved_version
 FOREIGN KEY(event_id,id,approved_version) REFERENCES clarification_approvals(event_id,answer_id,version);
ALTER TABLE clarification_answers ADD CONSTRAINT clarification_published_version
 FOREIGN KEY(event_id,id,published_version) REFERENCES clarification_publications(event_id,answer_id,version);

CREATE TABLE clarification_changes (
 event_id uuid NOT NULL REFERENCES events(id),
 id uuid NOT NULL,
 question_id uuid,
 answer_id uuid,
 revision bigint NOT NULL,
 version bigint,
 action text NOT NULL CHECK(action IN ('asked','triaged','closed','reopened','authored','approved','published')),
 actor_id uuid NOT NULL REFERENCES people(id),
 occurred_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,id),
 FOREIGN KEY(event_id,question_id) REFERENCES clarification_questions(event_id,id),
 FOREIGN KEY(event_id,answer_id) REFERENCES clarification_answers(event_id,id)
);

-- +goose StatementBegin
CREATE FUNCTION protect_clarification_question() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF ROW(NEW.event_id,NEW.id,NEW.team_id,NEW.author_id,NEW.command_id,NEW.request_digest,NEW.topic,NEW.body,NEW.attachments,NEW.source,NEW.created_at)
 IS DISTINCT FROM ROW(OLD.event_id,OLD.id,OLD.team_id,OLD.author_id,OLD.command_id,OLD.request_digest,OLD.topic,OLD.body,OLD.attachments,OLD.source,OLD.created_at) THEN
  RAISE EXCEPTION 'Private question authorship, source and content are immutable';
 END IF;
 RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER clarification_question_identity BEFORE UPDATE ON clarification_questions FOR EACH ROW EXECUTE FUNCTION protect_clarification_question();
-- +goose StatementBegin
CREATE FUNCTION protect_clarification_answer() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF ROW(NEW.event_id,NEW.id,NEW.question_id) IS DISTINCT FROM ROW(OLD.event_id,OLD.id,OLD.question_id)
 OR NEW.latest_version<OLD.latest_version OR NEW.revision<=OLD.revision
 OR (OLD.published_version IS NOT NULL AND (NEW.published_version IS NULL OR NEW.published_version<OLD.published_version)) THEN
  RAISE EXCEPTION 'Official answer identity and published history cannot be rewritten';
 END IF;
 RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER clarification_answer_identity BEFORE UPDATE ON clarification_answers FOR EACH ROW EXECUTE FUNCTION protect_clarification_answer();
CREATE TRIGGER clarification_questions_retained BEFORE DELETE OR TRUNCATE ON clarification_questions FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER clarification_versions_retained BEFORE UPDATE OR DELETE OR TRUNCATE ON clarification_versions FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER clarification_approvals_retained BEFORE UPDATE OR DELETE OR TRUNCATE ON clarification_approvals FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER clarification_publications_retained BEFORE UPDATE OR DELETE OR TRUNCATE ON clarification_publications FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER clarification_changes_retained BEFORE UPDATE OR DELETE OR TRUNCATE ON clarification_changes FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM clarification_questions) THEN
  RAISE EXCEPTION 'Private question and official answer history exists; rollback requires an explicit preservation plan';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE clarification_changes;
ALTER TABLE clarification_answers DROP CONSTRAINT clarification_approved_version;
ALTER TABLE clarification_answers DROP CONSTRAINT clarification_published_version;
DROP TABLE clarification_publications;
DROP TABLE clarification_approvals;
DROP TABLE clarification_versions;
DROP TABLE clarification_answers;
DROP TABLE clarification_questions;
DROP FUNCTION protect_clarification_question();
DROP FUNCTION protect_clarification_answer();
