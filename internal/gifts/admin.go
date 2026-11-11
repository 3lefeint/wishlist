package gifts

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// AdminGift is a gift as an administrator sees it. It carries no claim
// information: who claimed what stays hidden.
type AdminGift struct {
	ID         int64     `json:"id"`
	PersonID   int64     `json:"person_id"`
	PersonName string    `json:"person_name"`
	Title      string    `json:"title"`
	Comment    string    `json:"comment"`
	URL        string    `json:"url"`
	HasImage   bool      `json:"has_image"`
	Active     bool      `json:"active"`
	CreatedAt  time.Time `json:"created_at"`
}

// AdminList returns all gifts of a circle.
func (s *Store) AdminList(ctx context.Context, circleID int64) ([]AdminGift, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT g.id, g.person_id,
		       CASE WHEN p.user_id IS NULL THEN p.name
		            ELSE COALESCE(NULLIF(btrim(u.first_name || ' ' || u.last_name), ''), split_part(u.email, '@', 1)) END,
		       g.title, g.comment, g.url, g.has_image, g.active, g.created_at
		FROM gifts g
		JOIN persons p ON p.id = g.person_id
		LEFT JOIN users u ON u.id = p.user_id
		WHERE g.circle_id = $1
		ORDER BY g.person_id, g.created_at DESC, g.id DESC`, circleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AdminGift{}
	for rows.Next() {
		var g AdminGift
		if err := rows.Scan(&g.ID, &g.PersonID, &g.PersonName, &g.Title, &g.Comment, &g.URL, &g.HasImage, &g.Active, &g.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// AdminSetActive deactivates or reactivates a gift. A claim stays.
func (s *Store) AdminSetActive(ctx context.Context, giftID int64, active bool) error {
	tag, err := s.pool.Exec(ctx, `UPDATE gifts SET active = $2, updated_at = now() WHERE id = $1`, giftID, active)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// AdminDelete removes a gift and reports whether it had an image.
func (s *Store) AdminDelete(ctx context.Context, giftID int64) (hadImage bool, err error) {
	err = s.pool.QueryRow(ctx, `DELETE FROM gifts WHERE id = $1 RETURNING has_image`, giftID).Scan(&hadImage)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	return hadImage, err
}
