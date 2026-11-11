package circles

import (
	"context"
	"errors"
	"time"
)

// Errors of the administration methods.
var (
	ErrInvalidRole = errors.New("invalid role")
	ErrMember      = errors.New("user is already a member of the circle")
)

// AdminCircle is a circle in the administrator's overview.
type AdminCircle struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	MemberCount int       `json:"member_count"`
	GiftCount   int       `json:"gift_count"`
	CreatedAt   time.Time `json:"created_at"`
}

// AdminList returns every circle.
func (s *Store) AdminList(ctx context.Context) ([]AdminCircle, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT c.id, c.name, (SELECT count(*) FROM circle_members WHERE circle_id = c.id),
		       (SELECT count(*) FROM gifts WHERE circle_id = c.id), c.created_at
		FROM circles c ORDER BY c.name, c.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AdminCircle{}
	for rows.Next() {
		var c AdminCircle
		if err := rows.Scan(&c.ID, &c.Name, &c.MemberCount, &c.GiftCount, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// AdminGet returns a circle with members and persons, without an invite link.
func (s *Store) AdminGet(ctx context.Context, circleID int64) (Detail, error) {
	return s.load(ctx, circleID, "")
}

// AdminDelete removes a circle with everything in it.
func (s *Store) AdminDelete(ctx context.Context, circleID int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM circles WHERE id = $1`, circleID)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func validRole(r string) bool { return r == "admin" || r == "member" }

// AdminAddMember adds an approved user to a circle.
func (s *Store) AdminAddMember(ctx context.Context, circleID, userID int64, r string) error {
	if !validRole(r) {
		return ErrInvalidRole
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var ok bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM circles WHERE id = $1)
		   AND EXISTS (SELECT 1 FROM users WHERE id = $2 AND status = 'approved')`, circleID, userID).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	if _, err := role(ctx, tx, circleID, userID); err == nil {
		return ErrMember
	} else if !errors.Is(err, ErrNotFound) {
		return err
	}
	if err := addMember(ctx, tx, circleID, userID, r); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// AdminSetRole changes the role of a member. The last administrator cannot be demoted.
func (s *Store) AdminSetRole(ctx context.Context, circleID, userID int64, r string) error {
	if !validRole(r) {
		return ErrInvalidRole
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	cur, err := role(ctx, tx, circleID, userID)
	if err != nil {
		return err
	}
	if cur == "admin" && r != "admin" {
		var others int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM circle_members WHERE circle_id = $1 AND user_id <> $2 AND role = 'admin'`,
			circleID, userID).Scan(&others); err != nil {
			return err
		}
		if others == 0 {
			return ErrLastAdmin
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE circle_members SET role = $3 WHERE circle_id = $1 AND user_id = $2`, circleID, userID, r); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// AdminRemoveMember removes a member by the same rules as a member leaving.
func (s *Store) AdminRemoveMember(ctx context.Context, circleID, userID int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := removeMember(ctx, tx, circleID, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
