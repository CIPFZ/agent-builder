-- +goose Up
ALTER TABLE messages ADD COLUMN usage_json TEXT;

-- +goose Down
ALTER TABLE messages DROP COLUMN usage_json;
