-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION protect_team_repository_identity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (NEW.event_id,NEW.team_id,NEW.organization_id,NEW.organization,NEW.name,NEW.app_id,NEW.installation_id,NEW.template_repository,NEW.provenance,NEW.created_at) IS DISTINCT FROM
    (OLD.event_id,OLD.team_id,OLD.organization_id,OLD.organization,OLD.name,OLD.app_id,OLD.installation_id,OLD.template_repository,OLD.provenance,OLD.created_at)
 OR (OLD.github_repository_id IS NOT NULL AND NEW.github_repository_id IS DISTINCT FROM OLD.github_repository_id)
 OR (OLD.template_repository_id IS NOT NULL AND NEW.template_repository_id IS DISTINCT FROM OLD.template_repository_id)
 OR (OLD.attempted AND NOT NEW.attempted)
 THEN RAISE EXCEPTION 'repository identity and provenance are immutable'; END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER team_repositories_retained BEFORE DELETE OR TRUNCATE ON team_repositories FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM team_repositories)
 OR EXISTS(SELECT 1 FROM operations WHERE kind='repository.provision') THEN
  RAISE EXCEPTION 'Repository provenance or provisioning history exists; preserve it before rollback';
 END IF;
END $$;
-- +goose StatementEnd
DROP TRIGGER team_repositories_retained ON team_repositories;
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION protect_team_repository_identity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (NEW.event_id,NEW.team_id,NEW.organization_id,NEW.organization,NEW.name,NEW.app_id,NEW.installation_id,NEW.template_repository,NEW.provenance) IS DISTINCT FROM
    (OLD.event_id,OLD.team_id,OLD.organization_id,OLD.organization,OLD.name,OLD.app_id,OLD.installation_id,OLD.template_repository,OLD.provenance)
 OR (OLD.github_repository_id IS NOT NULL AND NEW.github_repository_id IS DISTINCT FROM OLD.github_repository_id)
 OR (OLD.template_repository_id IS NOT NULL AND NEW.template_repository_id IS DISTINCT FROM OLD.template_repository_id)
 THEN RAISE EXCEPTION 'repository identity and provenance are immutable'; END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
