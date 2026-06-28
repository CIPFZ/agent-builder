-- +goose Up
-- +goose StatementBegin
ALTER TABLE messages ADD COLUMN metadata_json TEXT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE messages DROP COLUMN metadata_json;
-- +goose StatementEnd
