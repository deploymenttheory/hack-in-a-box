-- +goose Up
CREATE TABLE registration_forms (
    event_id uuid NOT NULL REFERENCES events(id),
    id uuid NOT NULL,
    revision bigint NOT NULL DEFAULT 1,
    latest_version integer NOT NULL DEFAULT 1,
    published_version integer,
    PRIMARY KEY(event_id,id)
);
CREATE TABLE registration_form_versions (
    event_id uuid NOT NULL,
    form_id uuid NOT NULL,
    version integer NOT NULL,
    definition jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(event_id,form_id,version),
    FOREIGN KEY(event_id,form_id) REFERENCES registration_forms(event_id,id)
);
CREATE TRIGGER registration_form_versions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON registration_form_versions
FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TABLE registration_form_publications (
    event_id uuid NOT NULL,
    form_id uuid NOT NULL,
    version integer NOT NULL,
    PRIMARY KEY(event_id,form_id,version),
    FOREIGN KEY(event_id,form_id,version) REFERENCES registration_form_versions(event_id,form_id,version)
);
CREATE TABLE form_responses (
    event_id uuid NOT NULL,
    person_id uuid NOT NULL REFERENCES people(id),
    form_id uuid NOT NULL,
    revision bigint NOT NULL DEFAULT 1,
    PRIMARY KEY(event_id,person_id,form_id),
    FOREIGN KEY(event_id,form_id) REFERENCES registration_forms(event_id,id)
);
CREATE TABLE form_response_versions (
    event_id uuid NOT NULL,
    person_id uuid NOT NULL,
    form_id uuid NOT NULL,
    revision bigint NOT NULL,
    form_version integer NOT NULL,
    answers jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(event_id,person_id,form_id,revision),
    FOREIGN KEY(event_id,person_id,form_id) REFERENCES form_responses(event_id,person_id,form_id) ON DELETE CASCADE,
    FOREIGN KEY(event_id,form_id,form_version) REFERENCES registration_form_versions(event_id,form_id,version)
);
-- Answers are append-only in the domain, but deletable under privacy retention.

-- +goose Down
DROP TABLE form_response_versions;
DROP TABLE form_responses;
DROP TABLE registration_form_publications;
DROP TABLE registration_form_versions;
DROP TABLE registration_forms;
