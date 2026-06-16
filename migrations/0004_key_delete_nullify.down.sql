ALTER TABLE request_logs
    DROP CONSTRAINT request_logs_api_key_id_fkey,
    ADD CONSTRAINT request_logs_api_key_id_fkey
        FOREIGN KEY (api_key_id) REFERENCES api_keys (id);

ALTER TABLE semantic_cache
    DROP CONSTRAINT semantic_cache_api_key_id_fkey,
    ADD CONSTRAINT semantic_cache_api_key_id_fkey
        FOREIGN KEY (api_key_id) REFERENCES api_keys (id);
