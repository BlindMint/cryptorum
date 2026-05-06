package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	PermissionManageUsers     = "manage_users"
	PermissionManageLibraries = "manage_libraries"
	PermissionManageMetadata  = "manage_metadata"
	PermissionViewAdmin       = "view_admin"
	PermissionViewLogs        = "view_logs"
	PermissionManageJobs      = "manage_jobs"
	PermissionDownloadBooks   = "download_books"
	PermissionRestoreBackups  = "restore_backups"
	PermissionCreateBackups   = "create_backups"
)

var allDefaultPermissions = []string{
	PermissionManageUsers,
	PermissionManageLibraries,
	PermissionManageMetadata,
	PermissionViewAdmin,
	PermissionViewLogs,
	PermissionManageJobs,
	PermissionDownloadBooks,
	PermissionRestoreBackups,
	PermissionCreateBackups,
}

type AppUser struct {
	ID               int64    `json:"id"`
	Username         string   `json:"username"`
	PasswordHash     string   `json:"-"`
	IsAdmin          bool     `json:"is_admin"`
	IsBootstrapAdmin bool     `json:"is_bootstrap_admin"`
	Permissions      []string `json:"permissions"`
	CreatedAt        int64    `json:"created_at"`
	UpdatedAt        int64    `json:"updated_at"`
}

func ensureBootstrapUser() (*AppUser, error) {
	username := strings.TrimSpace(appConfig.Auth.Username)
	if username == "" {
		username = "admin"
	}

	passwordHash := strings.TrimSpace(appConfig.Auth.PasswordHash)
	now := time.Now().Unix()

	_, err := appDB.Exec(`
		INSERT INTO app_user (
			id, username, password_hash, is_admin, is_bootstrap_admin,
			permissions_json, created_at, updated_at
		) VALUES (?, ?, ?, 1, 1, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			username = excluded.username,
			password_hash = COALESCE(NULLIF(excluded.password_hash, ''), app_user.password_hash),
			is_admin = 1,
			is_bootstrap_admin = 1,
			updated_at = excluded.updated_at
	`, 1, username, passwordHash, mustJSON(allDefaultPermissions), now, now)
	if err != nil {
		return nil, err
	}

	_, _ = appDB.Exec(`UPDATE app_user SET is_admin = 1, is_bootstrap_admin = 1 WHERE id = 1`)
	return loadUserByID(1)
}

func loadUserByID(userID int64) (*AppUser, error) {
	var user AppUser
	var permissionsJSON sql.NullString
	err := appDB.QueryRow(`
		SELECT id, username, COALESCE(password_hash, ''), is_admin, is_bootstrap_admin,
		       COALESCE(permissions_json, '[]'), created_at, updated_at
		FROM app_user
		WHERE id = ?
	`, userID).Scan(
		&user.ID, &user.Username, &user.PasswordHash, &user.IsAdmin, &user.IsBootstrapAdmin,
		&permissionsJSON, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	_ = json.Unmarshal([]byte(permissionsJSON.String), &user.Permissions)
	return &user, nil
}

func loadUserByUsername(username string) (*AppUser, error) {
	var user AppUser
	var permissionsJSON sql.NullString
	err := appDB.QueryRow(`
		SELECT id, username, COALESCE(password_hash, ''), is_admin, is_bootstrap_admin,
		       COALESCE(permissions_json, '[]'), created_at, updated_at
		FROM app_user
		WHERE username = ?
	`, username).Scan(
		&user.ID, &user.Username, &user.PasswordHash, &user.IsAdmin, &user.IsBootstrapAdmin,
		&permissionsJSON, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	_ = json.Unmarshal([]byte(permissionsJSON.String), &user.Permissions)
	return &user, nil
}

func listUsers() ([]AppUser, error) {
	rows, err := appDB.Query(`
		SELECT id, username, COALESCE(password_hash, ''), is_admin, is_bootstrap_admin,
		       COALESCE(permissions_json, '[]'), created_at, updated_at
		FROM app_user
		ORDER BY id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := []AppUser{}
	for rows.Next() {
		var user AppUser
		var permissionsJSON sql.NullString
		if err := rows.Scan(
			&user.ID, &user.Username, &user.PasswordHash, &user.IsAdmin, &user.IsBootstrapAdmin,
			&permissionsJSON, &user.CreatedAt, &user.UpdatedAt,
		); err != nil {
			continue
		}
		_ = json.Unmarshal([]byte(permissionsJSON.String), &user.Permissions)
		users = append(users, user)
	}
	return users, nil
}

func mustJSON(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func userPermissionSet(user *AppUser) map[string]bool {
	set := make(map[string]bool)
	if user == nil {
		return set
	}
	for _, perm := range user.Permissions {
		set[perm] = true
	}
	return set
}

func userHasPermission(user *AppUser, permission string) bool {
	if user == nil {
		return false
	}
	if user.IsAdmin || user.IsBootstrapAdmin {
		return true
	}
	return userPermissionSet(user)[permission]
}

func requirePermission(user *AppUser, permission string) bool {
	return userHasPermission(user, permission)
}

func userPermissionsOrDefault(user *AppUser) []string {
	if user == nil {
		return nil
	}
	if user.IsAdmin || user.IsBootstrapAdmin {
		return append([]string{}, allDefaultPermissions...)
	}
	if len(user.Permissions) == 0 {
		return []string{}
	}
	return append([]string{}, user.Permissions...)
}

func updateUserPermissions(userID int64, isAdmin bool, permissions []string) error {
	_, err := appDB.Exec(`
		UPDATE app_user
		SET is_admin = ?, permissions_json = ?, updated_at = ?
		WHERE id = ? AND is_bootstrap_admin = 0
	`, boolToInt(isAdmin), mustJSON(permissions), time.Now().Unix(), userID)
	return err
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func ensureBootstrapOwnership() error {
	_, err := appDB.Exec(`UPDATE library SET owner_user_id = 1 WHERE owner_user_id IS NULL OR owner_user_id = 0`)
	return err
}

func loadCurrentUser(r *http.Request) (*AppUser, error) {
	if appConfig.Auth.Mode == "none" {
		return loadUserByID(1)
	}
	session := getSessionFromContext(r.Context())
	if session == nil {
		return nil, fmt.Errorf("no session")
	}
	return loadUserByID(session.UserID)
}

func currentUserFromContext(ctx context.Context) *AppUser {
	return getUserFromContext(ctx)
}

func currentUserID(ctx context.Context) int64 {
	user := currentUserFromContext(ctx)
	if user == nil {
		return 1
	}
	return user.ID
}

func userCanAccessAllData(user *AppUser) bool {
	return user != nil && (user.IsAdmin || user.IsBootstrapAdmin)
}

func userOwnershipClause(user *AppUser, alias string) (string, []interface{}) {
	if userCanAccessAllData(user) {
		return "1 = 1", nil
	}
	return fmt.Sprintf("%s.owner_user_id = ?", alias), []interface{}{user.ID}
}

func userOwnsLibrary(user *AppUser, libraryOwnerID int64) bool {
	return userCanAccessAllData(user) || user.ID == libraryOwnerID
}

func mustInt64(value string) int64 {
	parsed, _ := strconv.ParseInt(value, 10, 64)
	return parsed
}

func canAccessLibrary(user *AppUser, libraryID int64) (bool, error) {
	if userCanAccessAllData(user) {
		return true, nil
	}

	var exists bool
	if err := appDB.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM library WHERE id = ? AND owner_user_id = ?)`,
		libraryID, user.ID,
	).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func canAccessBook(user *AppUser, bookID int64) (bool, error) {
	if userCanAccessAllData(user) {
		return true, nil
	}

	var exists bool
	if err := appDB.QueryRow(`
		SELECT EXISTS(
			SELECT 1
			FROM book b
			JOIN library l ON b.library_id = l.id
			WHERE b.id = ? AND l.owner_user_id = ?
		)`, bookID, user.ID).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func canAccessShelf(user *AppUser, shelfID int64) (bool, error) {
	if userCanAccessAllData(user) {
		return true, nil
	}

	var exists bool
	if err := appDB.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM shelf WHERE id = ? AND owner_user_id = ?)`,
		shelfID, user.ID,
	).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}
