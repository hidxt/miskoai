package storage

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
)

type Profile struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Style       string `json:"style"`
	Address     string `json:"address"`
	Length      string `json:"length"`
	Sticker     string `json:"sticker"`
	Humor       int    `json:"humor"`
}

func builtinProfile(id string) bool { return id == "warm" || id == "concise" || id == "professional" }
func validProfileID(id string) bool {
	if len(id) < 1 || len(id) > 64 {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}
func (p Profile) validate() error {
	if !validProfileID(p.ID) || builtinProfile(p.ID) || !derivedText(p.Name, 128, false) || !derivedText(p.Description, 1024, false) || !derivedText(p.Style, 512, false) || !derivedText(p.Address, 128, false) || p.Humor < 0 || p.Humor > 3 {
		return ErrInvalid
	}
	if p.Length != "short" && p.Length != "normal" && p.Length != "detailed" {
		return ErrInvalid
	}
	if p.Sticker != "off" && p.Sticker != "low" && p.Sticker != "normal" {
		return ErrInvalid
	}
	return nil
}
func profileJSON(p Profile) (string, error) {
	if e := p.validate(); e != nil {
		return "", e
	}
	b, e := json.Marshal(p)
	if e != nil || len(b) > 8192 {
		return "", ErrInvalid
	}
	return string(b), nil
}
func decodeProfile(body string) (Profile, error) {
	var p Profile
	if !derivedText(body, 8192, true) {
		return p, ErrInvalid
	}
	dec := json.NewDecoder(bytes.NewBufferString(body))
	dec.DisallowUnknownFields()
	if e := dec.Decode(&p); e != nil {
		return p, ErrInvalid
	}
	var extra any
	if e := dec.Decode(&extra); e != io.EOF {
		return p, ErrInvalid
	}
	canonical, e := profileJSON(p)
	if e != nil || canonical != body {
		return p, ErrInvalid
	}
	return p, nil
}
func activeProfile(ctx context.Context, q rowQuery, sc Scope) (Profile, error) {
	var id string
	e := q.QueryRowContext(ctx, "SELECT id FROM profile_selections WHERE account=? AND user=?", sc.Account, sc.User).Scan(&id)
	if e == sql.ErrNoRows {
		return Profile{ID: "warm"}, nil
	}
	if e != nil {
		return Profile{}, storageError(ctx, e)
	}
	p, e := profileByID(ctx, q, sc, id)
	if e == ErrNotFound {
		return Profile{}, ErrInvalid
	}
	return p, e
}
func profileByID(ctx context.Context, q rowQuery, sc Scope, id string) (Profile, error) {
	if !validProfileID(id) {
		return Profile{}, ErrInvalid
	}
	if builtinProfile(id) {
		return Profile{ID: id}, nil
	}
	var body string
	e := q.QueryRowContext(ctx, "SELECT body FROM profiles WHERE account=? AND user=? AND id=?", sc.Account, sc.User, id).Scan(&body)
	if e == sql.ErrNoRows {
		return Profile{}, ErrNotFound
	}
	if e != nil {
		return Profile{}, storageError(ctx, e)
	}
	return decodeProfile(body)
}
func (s *Store) ActiveProfile(ctx context.Context, sc Scope) (Profile, error) {
	if e := sc.validate(); e != nil {
		return Profile{}, e
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return Profile{}, storageError(ctx, e)
	}
	defer tx.Rollback()
	if e = validateProfileCapacity(ctx, tx); e != nil {
		return Profile{}, e
	}
	p, e := activeProfile(ctx, tx, sc)
	if e != nil {
		return Profile{}, e
	}
	return p, storageError(ctx, tx.Commit())
}
func (s *Store) Profiles(ctx context.Context, sc Scope) ([]Profile, error) {
	if e := sc.validate(); e != nil {
		return nil, e
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return nil, storageError(ctx, e)
	}
	defer tx.Rollback()
	if e = validateProfileCapacity(ctx, tx); e != nil {
		return nil, e
	}
	rows, e := tx.QueryContext(ctx, "SELECT body FROM profiles WHERE account=? AND user=? ORDER BY id", sc.Account, sc.User)
	if e != nil {
		return nil, storageError(ctx, e)
	}
	ps := []Profile{}
	for rows.Next() {
		var body string
		if e = rows.Scan(&body); e != nil {
			rows.Close()
			return nil, storageError(ctx, e)
		}
		p, e := decodeProfile(body)
		if e != nil {
			rows.Close()
			return nil, e
		}
		ps = append(ps, p)
	}
	if e = closeValidatedRows(ctx, rows); e != nil {
		return nil, e
	}
	return ps, storageError(ctx, tx.Commit())
}
func (s *Store) PutProfile(ctx context.Context, sc Scope, p Profile) error {
	if e := sc.validate(); e != nil {
		return e
	}
	body, e := profileJSON(p)
	if e != nil {
		return e
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return storageError(ctx, e)
	}
	defer tx.Rollback()
	if e = validateProfileCapacity(ctx, tx); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO profiles(account,user,id,body) VALUES(?,?,?,?) ON CONFLICT(account,user,id) DO UPDATE SET body=excluded.body`, sc.Account, sc.User, p.ID, body); e != nil {
		return storageError(ctx, e)
	}
	if e = validateProfileCapacity(ctx, tx); e != nil {
		return e
	}
	return storageError(ctx, tx.Commit())
}
func (s *Store) DeleteProfile(ctx context.Context, sc Scope, id string) error {
	if e := sc.validate(); e != nil {
		return e
	}
	if !validProfileID(id) || builtinProfile(id) {
		return ErrInvalid
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return storageError(ctx, e)
	}
	defer tx.Rollback()
	r, e := tx.ExecContext(ctx, "DELETE FROM profiles WHERE account=? AND user=? AND id=?", sc.Account, sc.User, id)
	if e = changed(ctx, r, e); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "UPDATE profile_selections SET id='warm' WHERE account=? AND user=? AND id=?", sc.Account, sc.User, id); e != nil {
		return storageError(ctx, e)
	}
	return storageError(ctx, tx.Commit())
}
func (s *Store) SelectProfile(ctx context.Context, sc Scope, id string) error {
	if e := sc.validate(); e != nil {
		return e
	}
	if !validProfileID(id) {
		return ErrInvalid
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return storageError(ctx, e)
	}
	defer tx.Rollback()
	if e = validateProfileCapacity(ctx, tx); e != nil {
		return e
	}
	if !builtinProfile(id) {
		var exists int
		if e = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM profiles WHERE account=? AND user=? AND id=?)", sc.Account, sc.User, id).Scan(&exists); e != nil {
			return storageError(ctx, e)
		}
		if exists == 0 {
			return ErrNotFound
		}
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO profile_selections(account,user,id) VALUES(?,?,?) ON CONFLICT(account,user) DO UPDATE SET id=excluded.id`, sc.Account, sc.User, id); e != nil {
		return storageError(ctx, e)
	}
	if e = validateProfileCapacity(ctx, tx); e != nil {
		return e
	}
	return storageError(ctx, tx.Commit())
}
