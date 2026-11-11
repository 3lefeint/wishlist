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
