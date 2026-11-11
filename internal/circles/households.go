package circles

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// Household groups persons of one circle, for example a family.
type Household struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// CreateHousehold adds a household to a circle. Only administrators may.
func (s *Store) CreateHousehold(ctx context.Context, userID, circleID int64, name string) (Household, error) {
	name, err := cleanName(name)
	if err != nil {
		return Household{}, err
	}
	r, err := actorRole(ctx, s.pool, circleID, userID)
	if err != nil {
		return Household{}, err
	}
	if r != "admin" {
		return Household{}, ErrForbidden
	}
	h := Household{Name: name}
	err = s.pool.QueryRow(ctx, `INSERT INTO households (circle_id, name) VALUES ($1, $2) RETURNING id`, circleID, name).Scan(&h.ID)
	return h, err
}

// ownHousehold returns the household of the user's own person in a circle, or nil.
func ownHousehold(ctx context.Context, q querier, circleID, userID int64) (*int64, error) {
	var id *int64
	err := q.QueryRow(ctx, `SELECT household_id FROM persons WHERE circle_id = $1 AND user_id = $2`, circleID, userID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return id, err
}

// mayManage reports whether a viewer may change a household: administrators may change any,
// other members only the one they belong to. A nil household is open to everyone.
func mayManage(r string, own, household *int64) bool {
	return household == nil || r == "admin" || (own != nil && *own == *household)
}

// householdCircle returns the circle of a household.
func householdCircle(ctx context.Context, q querier, householdID int64) (int64, error) {
	var circleID int64
	err := q.QueryRow(ctx, `SELECT circle_id FROM households WHERE id = $1`, householdID).Scan(&circleID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	return circleID, err
}

// RenameHousehold renames a household. An administrator or a member of the household may.
func (s *Store) RenameHousehold(ctx context.Context, userID, householdID int64, name string) error {
	name, err := cleanName(name)
	if err != nil {
		return err
	}
	circleID, err := householdCircle(ctx, s.pool, householdID)
	if err != nil {
		return err
	}
	r, err := actorRole(ctx, s.pool, circleID, userID)
	if err != nil {
		return err
	}
	own, err := ownHousehold(ctx, s.pool, circleID, userID)
	if err != nil {
		return err
	}
	if !mayManage(r, own, &householdID) {
		return ErrForbidden
	}
	_, err = s.pool.Exec(ctx, `UPDATE households SET name = $2 WHERE id = $1`, householdID, name)
	return err
}

// DeleteHousehold removes a household; its persons stay in the circle. Only administrators may.
func (s *Store) DeleteHousehold(ctx context.Context, userID, householdID int64) error {
	circleID, err := householdCircle(ctx, s.pool, householdID)
	if err != nil {
		return err
	}
	r, err := actorRole(ctx, s.pool, circleID, userID)
	if err != nil {
		return err
	}
	if r != "admin" {
		return ErrForbidden
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE persons SET household_id = NULL WHERE household_id = $1`, householdID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM households WHERE id = $1`, householdID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SetPersonHousehold puts a person into a household, or out of any with a nil householdID.
// Administrators may move anyone; another member only between no household and their own.
func (s *Store) SetPersonHousehold(ctx context.Context, userID, personID int64, householdID *int64) error {
	circleID, _, err := personInfo(ctx, s.pool, personID)
	if err != nil {
		return err
	}
	r, err := actorRole(ctx, s.pool, circleID, userID)
	if err != nil {
		return err
	}
	own, err := ownHousehold(ctx, s.pool, circleID, userID)
	if err != nil {
		return err
	}
	var current *int64
	if err := s.pool.QueryRow(ctx, `SELECT household_id FROM persons WHERE id = $1`, personID).Scan(&current); err != nil {
		return err
	}
	if householdID != nil {
		hc, err := householdCircle(ctx, s.pool, *householdID)
		if err != nil || hc != circleID {
			return ErrNotFound
		}
	}
	if !mayManage(r, own, current) || !mayManage(r, own, householdID) {
		return ErrForbidden
	}
	_, err = s.pool.Exec(ctx, `UPDATE persons SET household_id = $2 WHERE id = $1`, personID, householdID)
	return err
}
