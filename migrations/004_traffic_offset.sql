ALTER TABLE nodes
    ADD COLUMN traffic_offset_bytes INTEGER NOT NULL DEFAULT 0;

ALTER TABLE nodes
    ADD COLUMN traffic_offset_cycle TEXT NOT NULL DEFAULT '';
