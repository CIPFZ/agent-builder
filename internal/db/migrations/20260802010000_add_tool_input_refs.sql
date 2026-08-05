ALTER TABLE runtime_tool_calls ADD COLUMN input_ref TEXT;
ALTER TABLE runtime_tool_calls ADD COLUMN input_byte_length INTEGER NOT NULL DEFAULT 0;
ALTER TABLE runtime_tool_calls ADD COLUMN command_ref TEXT;
ALTER TABLE runtime_tool_calls ADD COLUMN command_byte_length INTEGER NOT NULL DEFAULT 0;
