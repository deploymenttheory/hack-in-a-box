-- +goose Up
CREATE TABLE team_repositories (
 event_id uuid NOT NULL,
 team_id uuid NOT NULL,
 organization_id bigint NOT NULL CHECK(organization_id>0),
 organization text NOT NULL,
 name text NOT NULL,
 app_id bigint NOT NULL CHECK(app_id>0),
 installation_id bigint NOT NULL CHECK(installation_id>0),
 template_repository text NOT NULL,
 requested_by uuid NOT NULL REFERENCES people(id),
 provenance uuid NOT NULL DEFAULT gen_random_uuid(),
 github_repository_id bigint UNIQUE CHECK(github_repository_id>0),
 template_repository_id bigint CHECK(template_repository_id>0),
 attempted boolean NOT NULL DEFAULT false,
 state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','ready','review','ambiguous')),
 diagnostic text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(event_id,team_id),
 UNIQUE(organization_id,name),
 FOREIGN KEY(event_id,team_id) REFERENCES hack_teams(event_id,id)
);
-- +goose StatementBegin
CREATE FUNCTION protect_team_repository_identity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (NEW.event_id,NEW.team_id,NEW.organization_id,NEW.organization,NEW.name,NEW.app_id,NEW.installation_id,NEW.template_repository,NEW.provenance) IS DISTINCT FROM
    (OLD.event_id,OLD.team_id,OLD.organization_id,OLD.organization,OLD.name,OLD.app_id,OLD.installation_id,OLD.template_repository,OLD.provenance)
 OR (OLD.github_repository_id IS NOT NULL AND NEW.github_repository_id IS DISTINCT FROM OLD.github_repository_id)
 OR (OLD.template_repository_id IS NOT NULL AND NEW.template_repository_id IS DISTINCT FROM OLD.template_repository_id)
 THEN RAISE EXCEPTION 'repository identity and provenance are immutable'; END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER team_repository_identity BEFORE UPDATE ON team_repositories FOR EACH ROW EXECUTE FUNCTION protect_team_repository_identity();
-- +goose Down
DROP TABLE team_repositories;
DROP FUNCTION protect_team_repository_identity();
