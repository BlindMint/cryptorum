package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"cryptorum/internal/auth"
)

type UserResponse struct {
	ID               int64    `json:"id"`
	Username         string   `json:"username"`
	IsAdmin          bool     `json:"is_admin"`
	IsBootstrapAdmin bool     `json:"is_bootstrap_admin"`
	Permissions      []string `json:"permissions"`
	CreatedAt        int64    `json:"created_at"`
	UpdatedAt        int64    `json:"updated_at"`
}

func toUserResponse(user AppUser) UserResponse {
	return UserResponse{
		ID:               user.ID,
		Username:         user.Username,
		IsAdmin:          user.IsAdmin,
		IsBootstrapAdmin: user.IsBootstrapAdmin,
		Permissions:      userPermissionsOrDefault(&user),
		CreatedAt:        user.CreatedAt,
		UpdatedAt:        user.UpdatedAt,
	}
}

func ListUsersHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionManageUsers) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	users, err := listUsers()
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to load users")
		return
	}

	resp := make([]UserResponse, 0, len(users))
	for _, user := range users {
		resp = append(resp, toUserResponse(user))
	}

	jsonResponse(w, http.StatusOK, resp)
}

func CreateUserHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionManageUsers) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	var req struct {
		Username    string   `json:"username"`
		Password    string   `json:"password"`
		IsAdmin     bool     `json:"is_admin"`
		Permissions []string `json:"permissions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	username := strings.TrimSpace(req.Username)
	if username == "" || req.Password == "" {
		errorResponse(w, http.StatusBadRequest, "Username and password are required")
		return
	}

	if _, err := loadUserByUsername(username); err == nil {
		errorResponse(w, http.StatusConflict, "Username already exists")
		return
	}

	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to hash password")
		return
	}

	permissions := req.Permissions
	if req.IsAdmin {
		permissions = allDefaultPermissions
	}

	now := time.Now().Unix()
	result, err := appDB.Exec(`
		INSERT INTO app_user (
			username, password_hash, is_admin, is_bootstrap_admin,
			permissions_json, created_at, updated_at
		) VALUES (?, ?, ?, 0, ?, ?, ?)
	`, username, passwordHash, boolToInt(req.IsAdmin), mustJSON(permissions), now, now)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to create user")
		return
	}

	id, _ := result.LastInsertId()
	user, err := loadUserByID(id)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to load created user")
		return
	}

	recordAppLog("info", "users", "Created user", map[string]any{
		"user_id":  id,
		"username": username,
	})

	jsonResponse(w, http.StatusCreated, toUserResponse(*user))
}

func UpdateUserHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionManageUsers) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	userID, err := strconv.ParseInt(chi.URLParam(r, "userID"), 10, 64)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid user ID")
		return
	}

	target, err := loadUserByID(userID)
	if err != nil {
		errorResponse(w, http.StatusNotFound, "User not found")
		return
	}

	var req struct {
		Password    string   `json:"password"`
		IsAdmin     bool     `json:"is_admin"`
		Permissions []string `json:"permissions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	isAdmin := req.IsAdmin || target.IsBootstrapAdmin
	permissions := req.Permissions
	if isAdmin {
		permissions = allDefaultPermissions
	}

	passwordHash := target.PasswordHash
	if strings.TrimSpace(req.Password) != "" {
		passwordHash, err = auth.HashPassword(req.Password)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, "Failed to hash password")
			return
		}
	}

	_, err = appDB.Exec(`
		UPDATE app_user
		SET password_hash = ?, is_admin = ?, permissions_json = ?, updated_at = ?
		WHERE id = ? AND is_bootstrap_admin = 0
	`, passwordHash, boolToInt(isAdmin), mustJSON(permissions), time.Now().Unix(), userID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to update user")
		return
	}

	updated, err := loadUserByID(userID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to reload user")
		return
	}

	recordAppLog("info", "users", "Updated user", map[string]any{
		"user_id": userID,
	})

	jsonResponse(w, http.StatusOK, toUserResponse(*updated))
}

func DeleteUserHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r.Context())
	if !requirePermission(current, PermissionManageUsers) {
		errorResponse(w, http.StatusForbidden, "Permission denied")
		return
	}

	userID, err := strconv.ParseInt(chi.URLParam(r, "userID"), 10, 64)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "Invalid user ID")
		return
	}

	target, err := loadUserByID(userID)
	if err != nil {
		errorResponse(w, http.StatusNotFound, "User not found")
		return
	}
	if target.IsBootstrapAdmin {
		errorResponse(w, http.StatusForbidden, "Bootstrap admin cannot be removed")
		return
	}

	_, err = appDB.Exec(`DELETE FROM app_user WHERE id = ?`, userID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "Failed to delete user")
		return
	}

	recordAppLog("info", "users", "Deleted user", map[string]any{"user_id": userID})
	jsonResponse(w, http.StatusOK, map[string]string{"status": "deleted"})
}
