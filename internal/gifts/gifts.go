// Package gifts stores gifts, their tags and claims.
package gifts

import (
	"context"
	"errors"
	"math/rand/v2"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Errors returned by the store. Anything a user may not see answers ErrNotFound.
var (
	ErrNotFound  = errors.New("not found")
	ErrForbidden = errors.New("forbidden")
	ErrInvalid   = errors.New("invalid gift")
	ErrClaimed   = errors.New("gift is already claimed")
	ErrInactive  = errors.New("gift is deactivated")
	ErrOwnList   = errors.New("gifts on your own list cannot be claimed")
)

const maxTags = 10

// Store reads and writes gifts.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore returns a store.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Tag is a label with a stored colour.
type Tag struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	TextColor string `json:"text_color"`
}

// Claimer is the member who claimed a gift.
type Claimer struct {
	UserID int64  `json:"user_id"`
	Name   string `json:"name"`
}

// Gift is a gift as seen by one viewer. Claim is nil for the person the list
// belongs to, who must not learn whether a gift is claimed.
type Gift struct {
	ID        int64     `json:"id"`
	PersonID  int64     `json:"person_id"`
	Title     string    `json:"title"`
	Comment   string    `json:"comment"`
	URL       string    `json:"url"`
	HasImage  bool      `json:"has_image"`
	Active    bool      `json:"active"`
	Tags      []Tag     `json:"tags"`
	Claim     *Claimer  `json:"claim"`
	CanEdit   bool      `json:"can_edit"`
	CanDelete bool      `json:"can_delete"`
	CreatedAt time.Time `json:"created_at"`
}

// Input is the editable part of a gift.
type Input struct {
	Title   string   `json:"title"`
	Comment string   `json:"comment"`
	URL     string   `json:"url"`
	Tags    []string `json:"tags"`
}

func (in *Input) clean() error {
	in.Title = strings.TrimSpace(in.Title)
	in.Comment = strings.TrimSpace(in.Comment)
	in.URL = strings.TrimSpace(in.URL)
	if in.Title == "" || utf8.RuneCountInString(in.Title) > 120 || utf8.RuneCountInString(in.Comment) > 2000 {
		return ErrInvalid
	}
	if in.URL != "" {
		u, err := url.Parse(in.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || len(in.URL) > 2000 {
			return ErrInvalid
		}
	}
	seen := map[string]bool{}
	var tags []string
	for _, t := range in.Tags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if utf8.RuneCountInString(t) > 40 {
			return ErrInvalid
		}
		if k := strings.ToLower(t); !seen[k] {
			seen[k] = true
			tags = append(tags, t)
		}
	}
	if len(tags) > maxTags {
		return ErrInvalid
	}
	in.Tags = tags
	return nil
}

// access describes what a viewer may do with a gift.
type access struct {
	circleID   int64
	personID   int64
	personUser *int64
	creator    *int64
	claimer    *int64
	active     bool
	hasImage   bool
	admin      bool
	operator   bool // the operator account acts on every gift without being a circle member
	viewer     int64
}

func (a access) isCreator() bool { return a.creator != nil && *a.creator == a.viewer }

// mayEdit reports whether the viewer may change the gift: its creator or the operator account.
func (a access) mayEdit() bool   { return a.isCreator() || a.operator }
func (a access) isClaimer() bool { return a.claimer != nil && *a.claimer == a.viewer }
func (a access) isOwnList() bool { return a.personUser != nil && *a.personUser == a.viewer }

func (s *Store) access(ctx context.Context, q pgx.Tx, giftID, viewer int64) (access, error) {
	a := access{viewer: viewer}
	var role *string
	err := q.QueryRow(ctx, `
		SELECT g.circle_id, g.person_id, p.user_id, g.created_by, c.user_id, g.active, g.has_image, m.role, op.id IS NOT NULL
		FROM gifts g
		JOIN persons p ON p.id = g.person_id
		LEFT JOIN circle_members m ON m.circle_id = g.circle_id AND m.user_id = $2
		LEFT JOIN users op ON op.id = $2 AND op.is_admin
		LEFT JOIN claims c ON c.gift_id = g.id
		WHERE g.id = $1 AND (m.user_id IS NOT NULL OR op.id IS NOT NULL) FOR UPDATE OF g`, giftID, viewer).
		Scan(&a.circleID, &a.personID, &a.personUser, &a.creator, &a.claimer, &a.active, &a.hasImage, &role, &a.operator)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, ErrNotFound
	}
	a.admin = a.operator || (role != nil && *role == "admin")
	if err == nil && !a.active && !a.mayEdit() && !a.isClaimer() {
		return a, ErrNotFound
	}
	return a, err
}

func (s *Store) inTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// List returns the gifts on a person's list as the viewer may see them.
func (s *Store) List(ctx context.Context, viewer, personID int64) ([]Gift, error) {
	var circleID int64
	var personUser *int64
	var admin, operator bool
	err := s.pool.QueryRow(ctx, `
		SELECT p.circle_id, p.user_id, COALESCE(m.role = 'admin', false) OR op.id IS NOT NULL, op.id IS NOT NULL
		FROM persons p
		LEFT JOIN circle_members m ON m.circle_id = p.circle_id AND m.user_id = $2
		LEFT JOIN users op ON op.id = $2 AND op.is_admin
		WHERE p.id = $1 AND (m.user_id IS NOT NULL OR op.id IS NOT NULL)`, personID, viewer).Scan(&circleID, &personUser, &admin, &operator)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	// Claims stay hidden from the person the list is for and from the operator account.
	hideClaims := operator || (personUser != nil && *personUser == viewer)

	rows, err := s.pool.Query(ctx, `
		SELECT g.id, g.title, g.comment, g.url, g.has_image, g.active, g.created_by, g.created_at,
		       c.user_id, COALESCE(NULLIF(btrim(u.first_name || ' ' || u.last_name), ''), split_part(u.email, '@', 1))
		FROM gifts g
		LEFT JOIN claims c ON c.gift_id = g.id
		LEFT JOIN users u ON u.id = c.user_id
		WHERE g.person_id = $1 AND (g.active OR g.created_by = $2 OR c.user_id = $2 OR $3)
		ORDER BY g.active DESC, g.created_at DESC, g.id DESC`, personID, viewer, operator)
	if err != nil {
		return nil, err
	}
	out := []Gift{}
	ids := []int64{}
	idx := map[int64]int{}
	for rows.Next() {
		g := Gift{PersonID: personID, Tags: []Tag{}}
		var creator, claimUser *int64
		var claimName *string
		if err := rows.Scan(&g.ID, &g.Title, &g.Comment, &g.URL, &g.HasImage, &g.Active, &creator, &g.CreatedAt, &claimUser, &claimName); err != nil {
			rows.Close()
			return nil, err
		}
		mine := creator != nil && *creator == viewer
		g.CanEdit = mine || operator
		g.CanDelete = mine || admin
		if claimUser != nil && !hideClaims {
			g.Claim = &Claimer{UserID: *claimUser, Name: *claimName}
		}
		idx[g.ID] = len(out)
		ids = append(ids, g.ID)
		out = append(out, g)
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(ids) == 0 {
		return out, err
	}
	trows, err := s.pool.Query(ctx, `
		SELECT gt.gift_id, t.id, t.name, t.color, t.text_color
		FROM gift_tags gt JOIN tags t ON t.id = gt.tag_id
		WHERE gt.gift_id = ANY($1) ORDER BY lower(t.name)`, ids)
	if err != nil {
		return nil, err
	}
	defer trows.Close()
	for trows.Next() {
		var gid int64
		var t Tag
		if err := trows.Scan(&gid, &t.ID, &t.Name, &t.Color, &t.TextColor); err != nil {
			return nil, err
		}
		out[idx[gid]].Tags = append(out[idx[gid]].Tags, t)
	}
	return out, trows.Err()
}

// Create adds a gift to a person's list. Any circle member and the operator account may.
func (s *Store) Create(ctx context.Context, viewer, personID int64, in Input) (int64, error) {
	if err := in.clean(); err != nil {
		return 0, err
	}
	var id int64
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var circleID int64
		err := tx.QueryRow(ctx, `
			SELECT p.circle_id FROM persons p
			LEFT JOIN circle_members m ON m.circle_id = p.circle_id AND m.user_id = $2
			LEFT JOIN users op ON op.id = $2 AND op.is_admin
			WHERE p.id = $1 AND (m.user_id IS NOT NULL OR op.id IS NOT NULL)`, personID, viewer).Scan(&circleID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO gifts (circle_id, person_id, title, comment, url, created_by)
			VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
			circleID, personID, in.Title, in.Comment, in.URL, viewer).Scan(&id); err != nil {
			return err
		}
		return setTags(ctx, tx, circleID, id, in.Tags)
	})
	return id, err
}

// Update changes title, comment, link and tags. The creator and the operator account may.
func (s *Store) Update(ctx context.Context, viewer, giftID int64, in Input) error {
	if err := in.clean(); err != nil {
		return err
	}
	return s.inTx(ctx, func(tx pgx.Tx) error {
		a, err := s.access(ctx, tx, giftID, viewer)
		if err != nil {
			return err
		}
		if !a.mayEdit() {
			return ErrForbidden
		}
		if _, err := tx.Exec(ctx, `
			UPDATE gifts SET title = $2, comment = $3, url = $4, updated_at = now() WHERE id = $1`,
			giftID, in.Title, in.Comment, in.URL); err != nil {
			return err
		}
		return setTags(ctx, tx, a.circleID, giftID, in.Tags)
	})
}

// SetActive deactivates or reactivates a gift. The creator and the operator account may. A claim
// stays; the claimer still sees the gift.
func (s *Store) SetActive(ctx context.Context, viewer, giftID int64, active bool) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		a, err := s.access(ctx, tx, giftID, viewer)
		if err != nil {
			return err
		}
		if !a.mayEdit() {
			return ErrForbidden
		}
		_, err = tx.Exec(ctx, `UPDATE gifts SET active = $2, updated_at = now() WHERE id = $1`, giftID, active)
		return err
	})
}

// Delete removes a gift. The creator or a circle administrator may. It reports
// whether the gift had an image, so the caller can remove the files.
func (s *Store) Delete(ctx context.Context, viewer, giftID int64) (hadImage bool, err error) {
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		a, err := s.access(ctx, tx, giftID, viewer)
		if err != nil {
			return err
		}
		if !a.isCreator() && !a.admin {
			return ErrForbidden
		}
		hadImage = a.hasImage
		_, err = tx.Exec(ctx, `DELETE FROM gifts WHERE id = $1`, giftID)
		return err
	})
	return hadImage, err
}

// Claim marks a gift as taken by the viewer. Claiming twice is fine; a gift
// claimed by someone else answers ErrClaimed.
func (s *Store) Claim(ctx context.Context, viewer, giftID int64) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		a, err := s.access(ctx, tx, giftID, viewer)
		if err != nil {
			return err
		}
		if a.operator {
			return ErrForbidden // the operator account is no circle member and claims nothing
		}
		if a.isOwnList() {
			return ErrOwnList
		}
		if a.isClaimer() {
			return nil
		}
		if a.claimer != nil {
			return ErrClaimed
		}
		if !a.active {
			return ErrInactive
		}
		_, err = tx.Exec(ctx, `INSERT INTO claims (gift_id, user_id) VALUES ($1, $2)`, giftID, viewer)
		return err
	})
}

// Unclaim withdraws the viewer's claim. The operator account may withdraw any claim.
func (s *Store) Unclaim(ctx context.Context, viewer, giftID int64) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		a, err := s.access(ctx, tx, giftID, viewer)
		if err != nil {
			return err
		}
		if !a.isClaimer() && !a.operator {
			return ErrForbidden
		}
		_, err = tx.Exec(ctx, `DELETE FROM claims WHERE gift_id = $1`, giftID)
		return err
	})
}

// ImageAccess tells whether the viewer may see the gift and whether it has an image.
func (s *Store) ImageAccess(ctx context.Context, viewer, giftID int64) (hasImage bool, err error) {
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		a, err := s.access(ctx, tx, giftID, viewer)
		hasImage = a.hasImage
		return err
	})
	return hasImage, err
}

// SetImage records that a gift has (or no longer has) an image. The creator and the operator account may.
func (s *Store) SetImage(ctx context.Context, viewer, giftID int64, has bool) error {
	return s.inTx(ctx, func(tx pgx.Tx) error {
		a, err := s.access(ctx, tx, giftID, viewer)
		if err != nil {
			return err
		}
		if !a.mayEdit() {
			return ErrForbidden
		}
		_, err = tx.Exec(ctx, `UPDATE gifts SET has_image = $2, updated_at = now() WHERE id = $1`, giftID, has)
		return err
	})
}

// ImageIDs returns the ids of all gifts that have an image.
func (s *Store) ImageIDs(ctx context.Context) (map[int64]bool, error) {
	rows, err := s.pool.Query(ctx, `SELECT id FROM gifts WHERE has_image`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids[id] = true
	}
	return ids, rows.Err()
}

// Tags lists the tags of a circle for a member or the operator account.
func (s *Store) Tags(ctx context.Context, viewer, circleID int64) ([]Tag, error) {
	var ok bool
	if err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM circle_members WHERE circle_id = $1 AND user_id = $2)
		    OR EXISTS (SELECT 1 FROM circles WHERE id = $1) AND EXISTS (SELECT 1 FROM users WHERE id = $2 AND is_admin)`, circleID, viewer).Scan(&ok); err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrNotFound
	}
	rows, err := s.pool.Query(ctx, `SELECT id, name, color, text_color FROM tags WHERE circle_id = $1 ORDER BY lower(name)`, circleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Tag{}
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.Name, &t.Color, &t.TextColor); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// setTags replaces the tags of a gift, creating unknown tags with a random colour.
func setTags(ctx context.Context, tx pgx.Tx, circleID, giftID int64, names []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM gift_tags WHERE gift_id = $1`, giftID); err != nil {
		return err
	}
	for _, name := range names {
		color, text := randomColor()
		var tagID int64
		// The no-op update makes RETURNING work for an existing tag.
		if err := tx.QueryRow(ctx, `
			INSERT INTO tags (circle_id, name, color, text_color) VALUES ($1, $2, $3, $4)
			ON CONFLICT (circle_id, lower(name)) DO UPDATE SET name = tags.name
			RETURNING id`, circleID, name, color, text).Scan(&tagID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO gift_tags (gift_id, tag_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, giftID, tagID); err != nil {
			return err
		}
	}
	return nil
}

// randomColor returns a random background colour and a readable text colour.
func randomColor() (bg, text string) {
	h := rand.Float64() * 360
	sat := 0.55 + rand.Float64()*0.3
	l := 0.3 + rand.Float64()*0.45
	r, g, b := hslToRGB(h, sat, l)
	return hex(r, g, b), textFor(r, g, b)
}

func hex(r, g, b uint8) string {
	const d = "0123456789abcdef"
	return string([]byte{'#', d[r>>4], d[r&15], d[g>>4], d[g&15], d[b>>4], d[b&15]})
}

func textFor(r, g, b uint8) string {
	lin := func(c uint8) float64 {
		v := float64(c) / 255
		if v <= 0.03928 {
			return v / 12.92
		}
		return pow((v+0.055)/1.055, 2.4)
	}
	lum := 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
	if lum > 0.179 {
		return "#000000"
	}
	return "#ffffff"
}

func hslToRGB(h, s, l float64) (uint8, uint8, uint8) {
	c := (1 - abs(2*l-1)) * s
	x := c * (1 - abs(mod(h/60, 2)-1))
	m := l - c/2
	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return uint8((r + m) * 255), uint8((g + m) * 255), uint8((b + m) * 255)
}
