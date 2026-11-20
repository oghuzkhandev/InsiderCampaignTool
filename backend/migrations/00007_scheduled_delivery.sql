-- +goose Up
ALTER TABLE campaigns ADD COLUMN scheduled_at timestamptz;
ALTER TABLE campaigns ADD CONSTRAINT campaigns_schedule_requires_approval
    CHECK (scheduled_at IS NULL OR approved_at IS NOT NULL);

-- Onaydan sonra zaman değiştirmek ayrı bir ürün akışı gerektirir.
-- +goose StatementBegin
CREATE FUNCTION campaign_schedule_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.status <> 'draft' AND NEW.scheduled_at IS DISTINCT FROM OLD.scheduled_at THEN
        RAISE EXCEPTION 'approved campaign schedule is immutable';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER campaign_schedule_immutable BEFORE UPDATE ON campaigns
    FOR EACH ROW EXECUTE FUNCTION campaign_schedule_immutable();

-- +goose Down
-- Çalışması bitmeyen planlı kampanyanın zaman bilgisini sessizce silme.
LOCK TABLE campaigns, messages IN ACCESS EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM campaigns WHERE scheduled_at IS NOT NULL AND status IN ('approved','sending')) THEN
        RAISE EXCEPTION 'Down rejected: scheduled campaigns still have unfinished delivery';
    END IF;
END;
$$;
-- +goose StatementEnd
DROP TRIGGER campaign_schedule_immutable ON campaigns;
DROP FUNCTION campaign_schedule_immutable();
ALTER TABLE campaigns DROP CONSTRAINT campaigns_schedule_requires_approval;
ALTER TABLE campaigns DROP COLUMN scheduled_at;
