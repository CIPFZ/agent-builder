-- +goose Up
ALTER TABLE runtime_prompt_assemblies
    ADD COLUMN sections_json TEXT;

-- +goose Down
ALTER TABLE runtime_prompt_assemblies DROP COLUMN sections_json;
