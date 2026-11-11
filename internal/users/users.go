// Package users stores the people who signed in through Cloudflare Access.
package users

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Status values of a user.
const (
	StatusPending  = "pending"
	StatusApproved = "approved"
	StatusBlocked  = "blocked"
)

// ErrPendingLimit means the cap on pending registrations is reached.
var ErrPendingLimit = errors.New("too many pending registrations")

// User is a registered person.
type User struct {
	ID      int64  `json:"id"`
	Email   string `json:"email"`
	Status  string `json:"status"`
	IsAdmin bool   `json:"is_admin"`
}

// Store reads and writes users.
type Store struct {
	pool       *pgxpool.Pool
	admins     map[string]bool
	maxPending int
}

// NewStore returns a store. Addresses in adminEmails are approved administrators
// from their first sign-in on. maxPending caps new pending registrations.
func NewStore(pool *pgxpool.Pool, adminEmails []string, maxPending int) *Store {
	admins := make(map[string]bool, len(adminEmails))
	for _, e := range adminEmails {
		if n := Normalize(e); n != "" {
			admins[n] = true
		}
	}
	return &Store{pool: pool, admins: admins, maxPending: maxPending}
}

// Normalize lower-cases and trims an e-mail address; it is the identity of a user.
func Normalize(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

const columns = "id, email, status, is_admin"

func scan(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.Status, &u.IsAdmin)
	return u, err
}

// SignIn records a sign-in. A known user gets a new last-login time; an unknown
// one is created as pending (or as approved administrator when configured).
// It returns ErrPendingLimit when a new pending user would exceed the cap.
func (s *Store) SignIn(ctx context.Context, email string) (User, error) {
	email = Normalize(email)
	if email == "" {
		return User{}, errors.New("empty e-mail")
	}
	admin := s.admins[email]

	if admin {
		return scan(s.pool.QueryRow(ctx, `
			INSERT INTO users (email, status, is_admin) VALUES ($1, 'approved', true)
			ON CONFLICT (email) DO UPDATE
			SET status = CASE WHEN users.status = 'blocked' THEN users.status ELSE 'approved' END,
			    is_admin = true, last_login_at = now()
			RETURNING `+columns, email))
	}

	u, err := scan(s.pool.QueryRow(ctx,
		`UPDATE users SET last_login_at = now() WHERE email = $1 RETURNING `+columns, email))
	if err == nil {
		return u, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return User{}, err
	}

	var pending int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE status = 'pending'`).Scan(&pending); err != nil {
		return User{}, err
	}
	if pending >= s.maxPending {
		return User{}, ErrPendingLimit
	}
	return scan(s.pool.QueryRow(ctx, `
		INSERT INTO users (email) VALUES ($1)
		ON CONFLICT (email) DO UPDATE SET last_login_at = now()
		RETURNING `+columns, email))
}

// Errors returned by the administration methods.
var (
	ErrNotFound      = errors.New("user not found")
	ErrAdmin         = errors.New("administrators cannot be changed")
	ErrInvalidStatus = errors.New("invalid status")
)

// List returns users ordered by registration, optionally filtered by status.
func (s *Store) List(ctx context.Context, status string) ([]User, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+columns+` FROM users
		WHERE $1 = '' OR status = $1
		ORDER BY created_at, id`, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		u, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// SetStatus approves or blocks a user. Administrators are left alone.
func (s *Store) SetStatus(ctx context.Context, id int64, status string) (User, error) {
	if status != StatusApproved && status != StatusBlocked {
		return User{}, ErrInvalidStatus
	}
	u, err := scan(s.pool.QueryRow(ctx,
		`UPDATE users SET status = $2 WHERE id = $1 AND NOT is_admin RETURNING `+columns, id, status))
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, s.missingOrAdmin(ctx, id)
	}
	return u, err
}

// Delete removes a user. Administrators are left alone.
func (s *Store) Delete(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM users WHERE id = $1 AND NOT is_admin`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return s.missingOrAdmin(ctx, id)
	}
	return nil
}

func (s *Store) missingOrAdmin(ctx context.Context, id int64) error {
	var admin bool
	err := s.pool.QueryRow(ctx, `SELECT is_admin FROM users WHERE id = $1`, id).Scan(&admin)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return ErrAdmin
}
