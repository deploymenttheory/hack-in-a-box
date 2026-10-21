-- +goose Up
CREATE TABLE templates (
    id uuid PRIMARY KEY,
    owner_id uuid NOT NULL REFERENCES people(id),
    name text NOT NULL CHECK(length(name) BETWEEN 1 AND 160),
    revision bigint NOT NULL DEFAULT 1,
    latest_version integer NOT NULL DEFAULT 1,
    published_version integer,
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE template_versions (
    template_id uuid NOT NULL REFERENCES templates(id),
    version integer NOT NULL CHECK(version>0),
    configuration jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(template_id,version)
);
CREATE TRIGGER template_versions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON template_versions
FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TABLE template_publications (
    template_id uuid NOT NULL,
    version integer NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(template_id,version),
    FOREIGN KEY(template_id,version) REFERENCES template_versions(template_id,version)
);
CREATE TRIGGER template_publications_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON template_publications
FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TABLE event_definitions (
    event_id uuid PRIMARY KEY REFERENCES events(id),
    definitions jsonb NOT NULL,
    source_template_id uuid REFERENCES templates(id),
    source_version integer,
    clone_key text NOT NULL DEFAULT ''
);

-- +goose Down
DROP TABLE event_definitions;
DROP TABLE template_publications;
DROP TABLE template_versions;
DROP TABLE templates;
