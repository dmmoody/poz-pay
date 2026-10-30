-- A claimed row stays invisible until available_at. The relay sets that
-- timestamp to a lease while it publishes, then to a backoff after a failure.
-- attempts counts failed publishes. The relay stops claiming a row at its cap
-- so one payload cannot hold the queue.

ALTER TABLE message_outbox
    ADD COLUMN available_at timestamptz NOT NULL DEFAULT now();
