ALTER TABLE users RENAME COLUMN email TO account;

DROP INDEX users_email_unique;
CREATE UNIQUE INDEX users_account_unique ON users (account);

ALTER TABLE login_events RENAME COLUMN email_hash TO account_hash;
