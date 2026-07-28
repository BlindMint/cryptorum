-- +goose Up
-- Cryptorum retains app_user row 1 as its internal authenticated principal.
-- Preserve any legacy secondary-user data for manual reassignment, but revoke
-- its sessions and prevent creation of additional accounts.
UPDATE auth_session
SET revoked_at = COALESCE(revoked_at, CAST(strftime('%s', 'now') AS INTEGER))
WHERE user_id <> 1;

-- +goose StatementBegin
CREATE TRIGGER prevent_additional_app_users
BEFORE INSERT ON app_user
WHEN NEW.id <> 1
BEGIN
    SELECT RAISE(ABORT, 'Cryptorum supports one user');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER prevent_app_user_id_change
BEFORE UPDATE OF id ON app_user
WHEN NEW.id <> 1
BEGIN
    SELECT RAISE(ABORT, 'Cryptorum supports one user');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS prevent_app_user_id_change;
DROP TRIGGER IF EXISTS prevent_additional_app_users;
