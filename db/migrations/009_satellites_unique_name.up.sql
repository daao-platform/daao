-- Remove duplicate satellite records with the same name, keeping the best one:
-- prefer 'active' status, then most recently updated.
DELETE FROM satellites
WHERE id IN (
    SELECT id FROM (
        SELECT id,
               ROW_NUMBER() OVER (
                   PARTITION BY name
                   ORDER BY
                       CASE status WHEN 'active' THEN 0 WHEN 'offline' THEN 1 ELSE 2 END,
                       updated_at DESC
               ) AS rn
        FROM satellites
    ) ranked
    WHERE rn > 1
);

-- Add unique constraint so future registrations upsert instead of duplicate.
ALTER TABLE satellites ADD CONSTRAINT uq_satellites_name UNIQUE (name);
