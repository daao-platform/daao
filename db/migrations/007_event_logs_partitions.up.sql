-- Create event_logs partitions through end of 2026 and add a DEFAULT partition
-- to prevent INSERT failures when no partition matches

CREATE TABLE IF NOT EXISTS event_logs_2026_04 PARTITION OF event_logs
    FOR VALUES FROM ('2026-04-01') TO ('2026-05-01');

CREATE TABLE IF NOT EXISTS event_logs_2026_05 PARTITION OF event_logs
    FOR VALUES FROM ('2026-05-01') TO ('2026-06-01');

CREATE TABLE IF NOT EXISTS event_logs_2026_06 PARTITION OF event_logs
    FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');

CREATE TABLE IF NOT EXISTS event_logs_2026_07 PARTITION OF event_logs
    FOR VALUES FROM ('2026-07-01') TO ('2026-08-01');

CREATE TABLE IF NOT EXISTS event_logs_2026_08 PARTITION OF event_logs
    FOR VALUES FROM ('2026-08-01') TO ('2026-09-01');

CREATE TABLE IF NOT EXISTS event_logs_2026_09 PARTITION OF event_logs
    FOR VALUES FROM ('2026-09-01') TO ('2026-10-01');

CREATE TABLE IF NOT EXISTS event_logs_2026_10 PARTITION OF event_logs
    FOR VALUES FROM ('2026-10-01') TO ('2026-11-01');

CREATE TABLE IF NOT EXISTS event_logs_2026_11 PARTITION OF event_logs
    FOR VALUES FROM ('2026-11-01') TO ('2026-12-01');

CREATE TABLE IF NOT EXISTS event_logs_2026_12 PARTITION OF event_logs
    FOR VALUES FROM ('2026-12-01') TO ('2027-01-01');

-- First 3 months of 2027 as safety margin
CREATE TABLE IF NOT EXISTS event_logs_2027_01 PARTITION OF event_logs
    FOR VALUES FROM ('2027-01-01') TO ('2027-02-01');

CREATE TABLE IF NOT EXISTS event_logs_2027_02 PARTITION OF event_logs
    FOR VALUES FROM ('2027-02-01') TO ('2027-03-01');

CREATE TABLE IF NOT EXISTS event_logs_2027_03 PARTITION OF event_logs
    FOR VALUES FROM ('2027-03-01') TO ('2027-04-01');

-- DEFAULT partition catches any events outside defined ranges
CREATE TABLE IF NOT EXISTS event_logs_default PARTITION OF event_logs DEFAULT;
