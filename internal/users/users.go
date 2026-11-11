// Package users stores the people who signed in through Cloudflare Access.
package users

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

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
	// Name is the full name, or the part of the e-mail address before the at
	// sign while the profile is empty.
	Name            string `json:"name"`
	FirstName       string `json:"first_name"`
	LastName        string `json:"last_name"`
	Birthday        string `json:"birthday"`
	FavoriteColor   string `json:"favorite_color"`
	ProfileComplete bool   `json:"profile_complete"`
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

// NameExpr is the display name of a user: first and last name or the part of
// the e-mail address before the at sign.
const NameExpr = "COALESCE(NULLIF(btrim(first_name || ' ' || last_name), ''), split_part(email, '@', 1))"

// DateFormat is the layout of a birthday in the API and the database.
const DateFormat = "2006-01-02"

// DateExpr renders a nullable date column as DateFormat, empty when null.
func DateExpr(column string) string {
	return "COALESCE(to_char(" + column + ", 'YYYY-MM-DD'), '')"
}

var columns = "id, email, status, is_admin, " + NameExpr +
	", first_name, last_name, " + DateExpr("birthday") + ", favorite_color"

func scan(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.Status, &u.IsAdmin, &u.Name,
		&u.FirstName, &u.LastName, &u.Birthday, &u.FavoriteColor)
	u.ProfileComplete = u.FirstName != "" && u.LastName != "" && u.Birthday != ""
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
	ErrInvalidName   = errors.New("invalid name")
	ErrInvalidDate   = errors.New("invalid birthday")
	ErrInvalidColor  = errors.New("invalid colour")
)

var colorRE = regexp.MustCompile(`^#[0-9a-f]{6}$`)

// ParseBirthday validates an optional birthday. An empty string means none
// (nil). A date must not lie in the future or before 1900.
func ParseBirthday(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	d, err := time.Parse(DateFormat, s)
	if err != nil || d.Year() < 1900 || d.After(time.Now().AddDate(0, 0, 1)) {
		return nil, ErrInvalidDate
	}
	return &d, nil
}

// Profile is what a user enters about themselves. First name, last name and
// birthday are required, the colour is optional.
type Profile struct {
	FirstName     string `json:"first_name"`
	LastName      string `json:"last_name"`
	Birthday      string `json:"birthday"`
	FavoriteColor string `json:"favorite_color"`
}

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

// SetProfile stores the profile of a user.
func (s *Store) SetProfile(ctx context.Context, id int64, p Profile) (User, error) {
	first, last := strings.TrimSpace(p.FirstName), strings.TrimSpace(p.LastName)
	for _, n := range []string{first, last} {
		if n == "" || utf8.RuneCountInString(n) > 80 {
			return User{}, ErrInvalidName
		}
	}
	birthday, err := ParseBirthday(p.Birthday)
	if err != nil {
		return User{}, err
	}
	if birthday == nil {
		return User{}, ErrInvalidDate
	}
	color := strings.ToLower(strings.TrimSpace(p.FavoriteColor))
	if color != "" && !colorRE.MatchString(color) {
		return User{}, ErrInvalidColor
	}
	return scan(s.pool.QueryRow(ctx,
		`UPDATE users SET first_name = $2, last_name = $3, birthday = $4, favorite_color = $5
		 WHERE id = $1 RETURNING `+columns, id, first, last, birthday, color))
}

// Delete removes a user. Administrators are left alone. Circles that would be
// left without an administrator hand the role to their longest-standing member;
// circles the user was alone in are removed.
func (s *Store) Delete(ctx context.Context, id int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var admin bool
	if err := tx.QueryRow(ctx, `SELECT is_admin FROM users WHERE id = $1 FOR UPDATE`, id).Scan(&admin); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if admin {
		return ErrAdmin
	}
	if _, err := tx.Exec(ctx, `
		UPDATE circle_members m SET role = 'admin'
		WHERE (m.circle_id, m.user_id) IN (
			SELECT DISTINCT ON (o.circle_id) o.circle_id, o.user_id
			FROM circle_members o
			WHERE o.user_id <> $1
			  AND o.circle_id IN (SELECT circle_id FROM circle_members WHERE user_id = $1 AND role = 'admin')
			  AND NOT EXISTS (SELECT 1 FROM circle_members a WHERE a.circle_id = o.circle_id AND a.user_id <> $1 AND a.role = 'admin')
			ORDER BY o.circle_id, o.joined_at, o.user_id)`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM circles c WHERE EXISTS (SELECT 1 FROM circle_members WHERE circle_id = c.id AND user_id = $1)
		AND NOT EXISTS (SELECT 1 FROM circle_members WHERE circle_id = c.id AND user_id <> $1)`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM users WHERE id = $1`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
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
