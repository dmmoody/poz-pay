DROP TABLE IF EXISTS message_outbox;
DROP TABLE IF EXISTS check_register_upload_rejections;
DROP TABLE IF EXISTS check_register_upload_blobs;
DROP TABLE IF EXISTS check_register_uploads;
DROP TABLE IF EXISTS check_register_rejection_codes;
DROP TABLE IF EXISTS check_register_upload_statuses;

ALTER TABLE financial_institutions
    DROP CONSTRAINT IF EXISTS financial_institutions_routing_number_aba;

DROP INDEX IF EXISTS financial_institutions_routing_number_uq;

ALTER TABLE check_register_batches
    DROP CONSTRAINT IF EXISTS check_register_batches_source_fk;

DROP TABLE IF EXISTS check_register_batch_sources;
