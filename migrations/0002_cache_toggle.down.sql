-- Reverses 0002_cache_toggle.
ALTER TABLE api_keys DROP COLUMN IF EXISTS cache_enabled;
