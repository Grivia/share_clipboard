ALTER TABLE devices
    ADD COLUMN role TEXT NOT NULL DEFAULT 'member';

ALTER TABLE devices
    ADD CONSTRAINT devices_role_check
    CHECK (role IN ('super_admin', 'admin', 'member'));

WITH ranked_devices AS (
    SELECT id,
           row_number() OVER (
               PARTITION BY user_id
               ORDER BY (revoked_at IS NULL) DESC, first_login_at, id
           ) AS position
    FROM devices
)
UPDATE devices
SET role = 'super_admin'
WHERE id IN (
    SELECT id FROM ranked_devices WHERE position = 1
);

CREATE UNIQUE INDEX devices_user_super_admin_unique
    ON devices (user_id) WHERE role = 'super_admin';
