-- The arguments a tool call was given, cut to 500 characters, so owners see what an assistant asked.
ALTER TABLE app.mcp_call ADD COLUMN args text NOT NULL DEFAULT '';
