-- +goose Up
ALTER TABLE monitoring_settings
ADD COLUMN IF NOT EXISTS auto_incident_close_on_up INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE monitoring_settings
DROP COLUMN IF EXISTS auto_incident_close_on_up;
