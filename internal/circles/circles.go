// Package circles stores circles, their members, invite links and the persons
// whose gift lists they hold.
package circles

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/3lefeint/wishlist/internal/users"
)

// Errors returned by the store. A circle the user is not a member of answers
// ErrNotFound, so its existence is not revealed.
var (
	ErrNotFound    = errors.New("not found")
	ErrForbidden   = errors.New("forbidden")
	ErrInvalidName = errors.New("invalid name")
	ErrLastAdmin   = errors.New("the last administrator cannot leave while other members remain")
	ErrInvite      = errors.New("invite link is invalid or expired")
	ErrNotMember   = errors.New("user is not a member of the circle")
	ErrLinked      = errors.New("persons with an account cannot be changed this way")
	ErrOperator    = errors.New("the operator account cannot be a circle member")
)

// InviteLifetime is how long an invite link works.
const InviteLifetime = 7 * 24 * time.Hour

// Store reads and writes circles.
type Store struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

// NewStore returns a store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, now: time.Now}
}

// Circle is a circle as seen by one of its members.
type Circle struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Role        string `json:"role"`
	MemberCount int    `json:"member_count"`
}

// Member is a user in a circle.
type Member struct {
	UserID        int64  `json:"user_id"`
	Name          string `json:"name"`
	Email         string `json:"email"`
	Role          string `json:"role"`
	Birthday      string `json:"birthday"`
	FavoriteColor string `json:"favorite_color"`
}

// Person owns a gift list.
type Person struct {
	ID            int64   `json:"id"`
	Name          string  `json:"name"`
	UserID        *int64  `json:"user_id"`
	Guardians     []int64 `json:"guardians"`
	Birthday      string  `json:"birthday"`
	FavoriteColor string  `json:"favorite_color"`
	HouseholdID   *int64  `json:"household_id"`
}

// personBirthday and personColor take the values of the linked user for a
// person with an account, the person's own birthday otherwise. Both expect the
// persons table as p and a left join of the users table.
var (
	personBirthday = users.DateExpr("CASE WHEN p.user_id IS NULL THEN p.birthday ELSE users.birthday END")
	personColor    = "COALESCE(users.favorite_color, '')"
)

// Invite is the current invite link of a circle.
type Invite struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Detail is a circle with its members and persons.
type Detail struct {
	Circle
	Members    []Member    `json:"members"`
	Persons    []Person    `json:"persons"`
	Households []Household `json:"households"`
	Invite     *Invite     `json:"invite"`
}

func cleanName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 80 {
		return "", ErrInvalidName
	}
	return name, nil
}

type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// role returns the role of a user in a circle, or ErrNotFound for a non-member.
func role(ctx context.Context, q querier, circleID, userID int64) (string, error) {
	var r string
	err := q.QueryRow(ctx, `SELECT role FROM circle_members WHERE circle_id = $1 AND user_id = $2`, circleID, userID).Scan(&r)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return r, err
}

// actorRole returns the role of the acting user in a circle. The operator account acts as
// administrator of every existing circle without being a member; everyone else needs a membership.
func actorRole(ctx context.Context, q querier, circleID, userID int64) (string, error) {
	var r string
	err := q.QueryRow(ctx, `
		SELECT CASE WHEN u.is_admin THEN 'admin' ELSE m.role END
		FROM users u
		JOIN circles c ON c.id = $1
		LEFT JOIN circle_members m ON m.circle_id = c.id AND m.user_id = u.id
		WHERE u.id = $2 AND (u.is_admin OR m.user_id IS NOT NULL)`, circleID, userID).Scan(&r)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return r, err
}

func isOperator(ctx context.Context, q querier, userID int64) (bool, error) {
	var op bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE id = $1 AND is_admin)`, userID).Scan(&op)
	return op, err
}

// Create creates a circle with the user as its administrator.
func (s *Store) Create(ctx context.Context, userID int64, name string) (Circle, error) {
	name, err := cleanName(name)
	if err != nil {
		return Circle{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Circle{}, err
	}
	defer tx.Rollback(ctx)
	c := Circle{Name: name, Role: "admin", MemberCount: 1}
	if err := tx.QueryRow(ctx, `INSERT INTO circles (name, created_by) VALUES ($1, $2) RETURNING id`, name, userID).Scan(&c.ID); err != nil {
		return Circle{}, err
	}
	if err := addMember(ctx, tx, c.ID, userID, "admin"); err != nil {
		return Circle{}, err
	}
	return c, tx.Commit(ctx)
}

func addMember(ctx context.Context, tx pgx.Tx, circleID, userID int64, role string) error {
	if _, err := tx.Exec(ctx, `INSERT INTO circle_members (circle_id, user_id, role) VALUES ($1, $2, $3)`, circleID, userID, role); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO persons (circle_id, user_id, created_by) VALUES ($1, $2, $2)`, circleID, userID)
	return err
}

// List returns the circles of a user.
func (s *Store) List(ctx context.Context, userID int64) ([]Circle, error) {
	// The operator account sees every circle as administrator without being a member.
	rows, err := s.pool.Query(ctx, `
		SELECT c.id, c.name, CASE WHEN u.is_admin THEN 'admin' ELSE m.role END,
		       (SELECT count(*) FROM circle_members WHERE circle_id = c.id)
		FROM users u
		CROSS JOIN circles c
		LEFT JOIN circle_members m ON m.circle_id = c.id AND m.user_id = u.id
		WHERE u.id = $1 AND (u.is_admin OR m.user_id IS NOT NULL)
		ORDER BY c.name, c.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Circle{}
	for rows.Next() {
		var c Circle
		if err := rows.Scan(&c.ID, &c.Name, &c.Role, &c.MemberCount); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Get returns a circle with members, persons and, for administrators, the invite.
func (s *Store) Get(ctx context.Context, userID, circleID int64) (Detail, error) {
	r, err := actorRole(ctx, s.pool, circleID, userID)
	if err != nil {
		return Detail{}, err
	}
	return s.load(ctx, circleID, r)
}

// load reads a circle for a viewer with the given role; only a circle
// administrator gets the invite link.
func (s *Store) load(ctx context.Context, circleID int64, r string) (Detail, error) {
	d := Detail{}
	d.ID, d.Role = circleID, r
	err := s.pool.QueryRow(ctx, `SELECT name FROM circles WHERE id = $1`, circleID).Scan(&d.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, ErrNotFound
	}
	if err != nil {
		return d, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT u.id, `+users.NameExpr+`, u.email, m.role, `+users.DateExpr("u.birthday")+`, u.favorite_color
		FROM circle_members m JOIN users u ON u.id = m.user_id
		WHERE m.circle_id = $1 ORDER BY m.joined_at, u.id`, circleID)
	if err != nil {
		return d, err
	}
	d.Members = []Member{}
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.UserID, &m.Name, &m.Email, &m.Role, &m.Birthday, &m.FavoriteColor); err != nil {
			rows.Close()
			return d, err
		}
		d.Members = append(d.Members, m)
	}
	rows.Close()
	d.MemberCount = len(d.Members)

	prow, err := s.pool.Query(ctx, `
		SELECT p.id, CASE WHEN p.user_id IS NULL THEN p.name ELSE `+users.NameExpr+` END, p.user_id,
		       COALESCE((SELECT array_agg(g.user_id ORDER BY g.user_id) FROM person_guardians g WHERE g.person_id = p.id), '{}'),
		       `+personBirthday+`, `+personColor+`, p.household_id
		FROM persons p LEFT JOIN users ON users.id = p.user_id
		WHERE p.circle_id = $1 ORDER BY p.user_id IS NULL, lower(CASE WHEN p.user_id IS NULL THEN p.name ELSE `+users.NameExpr+` END), p.id`, circleID)
	if err != nil {
		return d, err
	}
	defer prow.Close()
	d.Persons = []Person{}
	for prow.Next() {
		var p Person
		if err := prow.Scan(&p.ID, &p.Name, &p.UserID, &p.Guardians, &p.Birthday, &p.FavoriteColor, &p.HouseholdID); err != nil {
			return d, err
		}
		d.Persons = append(d.Persons, p)
	}
	if err := prow.Err(); err != nil {
		return d, err
	}
	prow.Close()

	hrows, err := s.pool.Query(ctx, `SELECT id, name FROM households WHERE circle_id = $1 ORDER BY lower(name), id`, circleID)
	if err != nil {
		return d, err
	}
	defer hrows.Close()
	d.Households = []Household{}
	for hrows.Next() {
		var h Household
		if err := hrows.Scan(&h.ID, &h.Name); err != nil {
			return d, err
		}
		d.Households = append(d.Households, h)
	}
	if err := hrows.Err(); err != nil {
		return d, err
	}
	hrows.Close()

	if r == "admin" {
		var inv Invite
		err := s.pool.QueryRow(ctx, `
			SELECT token, expires_at FROM circle_invites
			WHERE circle_id = $1 AND revoked_at IS NULL AND expires_at > $2
			ORDER BY id DESC LIMIT 1`, circleID, s.now()).Scan(&inv.Token, &inv.ExpiresAt)
		if err == nil {
			d.Invite = &inv
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return d, err
		}
	}
	return d, nil
}

// Delete removes a circle. Only administrators may.
func (s *Store) Delete(ctx context.Context, userID, circleID int64) error {
	r, err := actorRole(ctx, s.pool, circleID, userID)
	if err != nil {
		return err
	}
	if r != "admin" {
		return ErrForbidden
	}
	_, err = s.pool.Exec(ctx, `DELETE FROM circles WHERE id = $1`, circleID)
	return err
}

// RemoveMember removes a member; a member may remove themselves, an
// administrator may remove anyone. The person entry of the removed user and
// their guardianships go with them; persons and gifts they created stay.
func (s *Store) RemoveMember(ctx context.Context, actorID, circleID, targetID int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	ar, err := actorRole(ctx, tx, circleID, actorID)
	if err != nil {
		return err
	}
	if actorID != targetID && ar != "admin" {
		return ErrForbidden
	}
	if err := removeMember(ctx, tx, circleID, targetID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// removeMember removes a member inside a transaction. The last member takes the
// circle with them; the last administrator cannot leave while others remain.
func removeMember(ctx context.Context, tx pgx.Tx, circleID, targetID int64) error {
	tr, err := role(ctx, tx, circleID, targetID)
	if err != nil {
		return err
	}
	var others, otherAdmins int
	if err := tx.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE role = 'admin')
		FROM circle_members WHERE circle_id = $1 AND user_id <> $2`, circleID, targetID).Scan(&others, &otherAdmins); err != nil {
		return err
	}
	if others == 0 {
		_, err := tx.Exec(ctx, `DELETE FROM circles WHERE id = $1`, circleID)
		return err
	}
	if tr == "admin" && otherAdmins == 0 {
		return ErrLastAdmin
	}
	if err := dropMemberData(ctx, tx, circleID, targetID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `DELETE FROM circle_members WHERE circle_id = $1 AND user_id = $2`, circleID, targetID)
	return err
}

func dropMemberData(ctx context.Context, tx pgx.Tx, circleID, userID int64) error {
	if _, err := tx.Exec(ctx, `
		DELETE FROM claims WHERE user_id = $2
		AND gift_id IN (SELECT id FROM gifts WHERE circle_id = $1)`, circleID, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM persons WHERE circle_id = $1 AND user_id = $2`, circleID, userID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		DELETE FROM person_guardians WHERE user_id = $2
		AND person_id IN (SELECT id FROM persons WHERE circle_id = $1)`, circleID, userID)
	return err
}

func newToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// CreateInvite replaces the invite link of a circle with a new one.
func (s *Store) CreateInvite(ctx context.Context, userID, circleID int64) (Invite, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Invite{}, err
	}
	defer tx.Rollback(ctx)
	r, err := actorRole(ctx, tx, circleID, userID)
	if err != nil {
		return Invite{}, err
	}
	if r != "admin" {
		return Invite{}, ErrForbidden
	}
	tok, err := newToken()
	if err != nil {
		return Invite{}, err
	}
	now := s.now()
	if _, err := tx.Exec(ctx, `UPDATE circle_invites SET revoked_at = $2 WHERE circle_id = $1 AND revoked_at IS NULL`, circleID, now); err != nil {
		return Invite{}, err
	}
	inv := Invite{Token: tok, ExpiresAt: now.Add(InviteLifetime)}
	if _, err := tx.Exec(ctx, `INSERT INTO circle_invites (circle_id, token, created_by, expires_at) VALUES ($1, $2, $3, $4)`,
		circleID, inv.Token, userID, inv.ExpiresAt); err != nil {
		return Invite{}, err
	}
	return inv, tx.Commit(ctx)
}

// RevokeInvite switches the invite link of a circle off.
func (s *Store) RevokeInvite(ctx context.Context, userID, circleID int64) error {
	r, err := actorRole(ctx, s.pool, circleID, userID)
	if err != nil {
		return err
	}
	if r != "admin" {
		return ErrForbidden
	}
	_, err = s.pool.Exec(ctx, `UPDATE circle_invites SET revoked_at = $2 WHERE circle_id = $1 AND revoked_at IS NULL`, circleID, s.now())
	return err
}

// InviteInfo returns the circle an invite link leads to.
func (s *Store) InviteInfo(ctx context.Context, token string) (id int64, name string, err error) {
	err = s.pool.QueryRow(ctx, `
		SELECT c.id, c.name FROM circle_invites i JOIN circles c ON c.id = i.circle_id
		WHERE i.token = $1 AND i.revoked_at IS NULL AND i.expires_at > $2`, token, s.now()).Scan(&id, &name)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", ErrInvite
	}
	return id, name, err
}

// Join adds the user to the circle behind the invite link. It is idempotent.
func (s *Store) Join(ctx context.Context, userID int64, token string) (int64, error) {
	circleID, _, err := s.InviteInfo(ctx, token)
	if err != nil {
		return 0, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	if _, err := role(ctx, tx, circleID, userID); err == nil {
		return circleID, nil
	} else if !errors.Is(err, ErrNotFound) {
		return 0, err
	}
	if err := addMember(ctx, tx, circleID, userID, "member"); err != nil {
		return 0, err
	}
	return circleID, tx.Commit(ctx)
}

// personCircle returns the circle and linked user of a person.
func personInfo(ctx context.Context, q querier, personID int64) (circleID int64, userID *int64, err error) {
	err = q.QueryRow(ctx, `SELECT circle_id, user_id FROM persons WHERE id = $1`, personID).Scan(&circleID, &userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, ErrNotFound
	}
	return circleID, userID, err
}

// canManage reports whether the user is a guardian of the person or an
// administrator of its circle.
func canManage(ctx context.Context, q querier, personID, circleID, userID int64) (bool, error) {
	r, err := actorRole(ctx, q, circleID, userID)
	if err != nil {
		return false, err
	}
	if r == "admin" {
		return true, nil
	}
	var g bool
	err = q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM person_guardians WHERE person_id = $1 AND user_id = $2)`, personID, userID).Scan(&g)
	return g, err
}

// CreatePerson adds a person without an account; the creator is the first guardian.
func (s *Store) CreatePerson(ctx context.Context, userID, circleID int64, name, birthday string) (Person, error) {
	name, err := cleanName(name)
	if err != nil {
		return Person{}, err
	}
	bd, err := users.ParseBirthday(birthday)
	if err != nil {
		return Person{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Person{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := actorRole(ctx, tx, circleID, userID); err != nil {
		return Person{}, err
	}
	operator, err := isOperator(ctx, tx, userID)
	if err != nil {
		return Person{}, err
	}
	p := Person{Name: name, Guardians: []int64{userID}, Birthday: strings.TrimSpace(birthday)}
	if operator {
		p.Guardians = []int64{} // the operator account is no guardian; guardians are added afterwards
	}
	if err := tx.QueryRow(ctx, `INSERT INTO persons (circle_id, name, birthday, created_by) VALUES ($1, $2, $3, $4) RETURNING id`, circleID, name, bd, userID).Scan(&p.ID); err != nil {
		return Person{}, err
	}
	if !operator {
		if _, err := tx.Exec(ctx, `INSERT INTO person_guardians (person_id, user_id) VALUES ($1, $2)`, p.ID, userID); err != nil {
			return Person{}, err
		}
	}
	return p, tx.Commit(ctx)
}

// UpdatePerson changes name and birthday of a person without an account; an
// empty birthday clears it.
func (s *Store) UpdatePerson(ctx context.Context, userID, personID int64, name, birthday string) error {
	name, err := cleanName(name)
	if err != nil {
		return err
	}
	bd, err := users.ParseBirthday(birthday)
	if err != nil {
		return err
	}
	circleID, linked, err := personInfo(ctx, s.pool, personID)
	if err != nil {
		return err
	}
	ok, err := canManage(ctx, s.pool, personID, circleID, userID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	if linked != nil {
		return ErrLinked
	}
	_, err = s.pool.Exec(ctx, `UPDATE persons SET name = $2, birthday = $3 WHERE id = $1`, personID, name, bd)
	return err
}

// DeletePerson removes a person without an account together with their gifts.
func (s *Store) DeletePerson(ctx context.Context, userID, personID int64) error {
	circleID, linked, err := personInfo(ctx, s.pool, personID)
	if err != nil {
		return err
	}
	ok, err := canManage(ctx, s.pool, personID, circleID, userID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	if linked != nil {
		return ErrLinked
	}
	_, err = s.pool.Exec(ctx, `DELETE FROM persons WHERE id = $1`, personID)
	return err
}

// AddGuardian makes a circle member a guardian of a person without an account.
func (s *Store) AddGuardian(ctx context.Context, actorID, personID, newGuardian int64) error {
	circleID, linked, err := personInfo(ctx, s.pool, personID)
	if err != nil {
		return err
	}
	ok, err := canManage(ctx, s.pool, personID, circleID, actorID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	if linked != nil {
		return ErrLinked
	}
	if _, err := role(ctx, s.pool, circleID, newGuardian); err != nil {
		return ErrNotMember
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO person_guardians (person_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, personID, newGuardian)
	return err
}

// RemoveGuardian removes a guardian. A guardian may remove themselves.
func (s *Store) RemoveGuardian(ctx context.Context, actorID, personID, guardian int64) error {
	circleID, _, err := personInfo(ctx, s.pool, personID)
	if err != nil {
		return err
	}
	ok, err := canManage(ctx, s.pool, personID, circleID, actorID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	_, err = s.pool.Exec(ctx, `DELETE FROM person_guardians WHERE person_id = $1 AND user_id = $2`, personID, guardian)
	return err
}

// PersonDetail is a person with the circle it belongs to.
type PersonDetail struct {
	Person
	CircleID   int64    `json:"circle_id"`
	CircleName string   `json:"circle_name"`
	IsSelf     bool     `json:"is_self"`
	Members    []Member `json:"members"`
}

// GetPerson returns a person for a member of its circle.
func (s *Store) GetPerson(ctx context.Context, userID, personID int64) (PersonDetail, error) {
	var d PersonDetail
	err := s.pool.QueryRow(ctx, `
		SELECT p.id, CASE WHEN p.user_id IS NULL THEN p.name ELSE `+users.NameExpr+` END, p.user_id,
		       COALESCE((SELECT array_agg(g.user_id ORDER BY g.user_id) FROM person_guardians g WHERE g.person_id = p.id), '{}'),
		       `+personBirthday+`, `+personColor+`, p.household_id,
		       c.id, c.name
		FROM persons p
		JOIN circles c ON c.id = p.circle_id
		LEFT JOIN circle_members m ON m.circle_id = c.id AND m.user_id = $2
		LEFT JOIN users ON users.id = p.user_id
		WHERE p.id = $1 AND (m.user_id IS NOT NULL OR EXISTS (SELECT 1 FROM users o WHERE o.id = $2 AND o.is_admin))`, personID, userID).
		Scan(&d.ID, &d.Name, &d.UserID, &d.Guardians, &d.Birthday, &d.FavoriteColor, &d.HouseholdID, &d.CircleID, &d.CircleName)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, ErrNotFound
	}
	d.IsSelf = d.UserID != nil && *d.UserID == userID
	return d, err
}
