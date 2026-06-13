-- 0002_cache_toggle: per-key response-cache toggle.
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS cache_enabled bool NOT NULL DEFAULT true;
