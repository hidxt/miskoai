package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"math"
	"strings"
	"time"
)

// Fixed phase values describe receipts only. None grants restart execution.
const (
	ImagePreparationPlanned  = "planned"
	ImagePreparationExternal = "external"
	ImagePreparationReady    = "ready"
	ImagePreparationStopped  = "stopped"
	ImagePreparationUnknown  = "unknown"
	ImagePartPlanned         = "planned"
	ImagePartSending         = "sending"
	ImagePartAcknowledged    = "acked"
	ImagePartStopped         = "stopped"
	ImagePartUnknown         = "unknown"
	ImageStopPreparation     = "preparation"
	ImageStopText            = "text"
	ImageStopImage           = "image"
	ImageStopRefused         = "refused"
	ImageStopCanceled        = "canceled"
	ImageStopCleanup         = "cleanup"
	ImageStopUnavailable     = "unavailable"
	ImageStopUncertain       = "uncertain"
)
const schema6 = `
CREATE TABLE image_replies (
 account TEXT NOT NULL, user TEXT NOT NULL, id TEXT NOT NULL,
 asset_id INTEGER NOT NULL CHECK(asset_id>0), sha256 TEXT NOT NULL,
 text_client_id TEXT NOT NULL UNIQUE, image_client_id TEXT NOT NULL UNIQUE CHECK(text_client_id<>image_client_id),
 preparation TEXT NOT NULL CHECK(preparation IN ('planned','external','ready','stopped','unknown')),
 text_phase TEXT NOT NULL CHECK(text_phase IN ('planned','sending','acked','stopped','unknown')),
 image_phase TEXT NOT NULL CHECK(image_phase IN ('planned','sending','acked','stopped','unknown')),
 stop_phase TEXT NOT NULL CHECK(stop_phase IN ('','preparation','text','image')),
 stop_code TEXT NOT NULL CHECK(stop_code IN ('','refused','canceled','cleanup','unavailable','uncertain')),
 created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
 text_acked_at INTEGER NOT NULL, image_acked_at INTEGER NOT NULL,
 PRIMARY KEY(account,user,id),
 FOREIGN KEY(account,user,id) REFERENCES messages(account,user,id) ON DELETE CASCADE
);
PRAGMA user_version=6;
`

// ImageReply is bounded descriptive metadata; it contains no reply copy or
// upload capability and cannot be enumerated as pending executable work.
type ImageReply struct {
	Scope                                                                                 Scope
	ID                                                                                    string
	AssetID                                                                               int64
	SHA256, TextClientID, ImageClientID                                                   string
	Preparation, TextPhase, ImagePhase, StopPhase, StopCode                               string
	CreatedAtMillis, UpdatedAtMillis, TextAcknowledgedAtMillis, ImageAcknowledgedAtMillis int64
}

const imageReplyColumns = `account,user,id,asset_id,sha256,text_client_id,image_client_id,preparation,text_phase,image_phase,stop_phase,stop_code,created_at,updated_at,text_acked_at,image_acked_at`

func imageClientIDs(sc Scope, id string) (string, string) {
	encode := func(parts []string) string {
		b, _ := json.Marshal(parts)
		h := sha256.Sum256(b)
		return "miskoai-" + hex.EncodeToString(h[:])
	}
	return encode([]string{sc.Account, sc.User, id}), encode([]string{sc.Account, sc.User, id, "image-v1"})
}
func imageDigest(v string) bool {
	if len(v) != 64 {
		return false
	}
	for _, c := range v {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func imageStamp(n int64) bool { return n >= 1 && n <= 253402300799999 }
func imagePart(v string) bool {
	return v == ImagePartPlanned || v == ImagePartSending || v == ImagePartAcknowledged || v == ImagePartStopped || v == ImagePartUnknown
}
func imageStopCode(v string) bool {
	return v == ImageStopRefused || v == ImageStopCanceled || v == ImageStopCleanup || v == ImageStopUnavailable || v == ImageStopUncertain
}
func validImageReply(r ImageReply, parent, reply string) bool {
	text, image := imageClientIDs(r.Scope, r.ID)
	if r.Scope.validate() != nil || !validMessageID(r.ID) || r.AssetID <= 0 || !imageDigest(r.SHA256) || r.TextClientID != text || r.ImageClientID != image || !imageStamp(r.CreatedAtMillis) || !imageStamp(r.UpdatedAtMillis) || r.UpdatedAtMillis < r.CreatedAtMillis || !imagePart(r.TextPhase) || !imagePart(r.ImagePhase) {
		return false
	}
	if r.Preparation != ImagePreparationPlanned && r.Preparation != ImagePreparationExternal && r.Preparation != ImagePreparationReady && r.Preparation != ImagePreparationStopped && r.Preparation != ImagePreparationUnknown {
		return false
	}
	if (r.StopPhase == "") != (r.StopCode == "") {
		return false
	}
	if r.StopCode != "" && !imageStopCode(r.StopCode) {
		return false
	}
	if r.TextAcknowledgedAtMillis < 0 || r.ImageAcknowledgedAtMillis < 0 || r.TextAcknowledgedAtMillis > r.UpdatedAtMillis || r.ImageAcknowledgedAtMillis > r.UpdatedAtMillis {
		return false
	}
	if (r.TextPhase == ImagePartAcknowledged) != (r.TextAcknowledgedAtMillis > 0) || (r.ImagePhase == ImagePartAcknowledged) != (r.ImageAcknowledgedAtMillis > 0) {
		return false
	}
	if r.TextAcknowledgedAtMillis > 0 && (!imageStamp(r.TextAcknowledgedAtMillis) || r.TextAcknowledgedAtMillis < r.CreatedAtMillis) {
		return false
	}
	if r.ImageAcknowledgedAtMillis > 0 && (!imageStamp(r.ImageAcknowledgedAtMillis) || r.ImageAcknowledgedAtMillis < r.TextAcknowledgedAtMillis) {
		return false
	}
	switch r.TextPhase {
	case ImagePartPlanned:
		if parent != "processing" || reply != "" {
			return false
		}
	case ImagePartSending:
		if parent != "sending" || reply != "" {
			return false
		}
	case ImagePartAcknowledged:
		if parent != "sent" || strings.TrimSpace(reply) == "" || !validField(reply, 16384) {
			return false
		}
	case ImagePartStopped:
		if parent != "failed" || reply != "" {
			return false
		}
	case ImagePartUnknown:
		if parent != "ambiguous" || reply != "" {
			return false
		}
	}
	if r.TextPhase != ImagePartPlanned && r.TextPhase != ImagePartStopped && r.Preparation != ImagePreparationReady {
		return false
	}
	if r.ImagePhase == ImagePartSending || r.ImagePhase == ImagePartAcknowledged || r.ImagePhase == ImagePartUnknown {
		if r.Preparation != ImagePreparationReady || r.TextPhase != ImagePartAcknowledged {
			return false
		}
	}
	if r.StopPhase == "" {
		if r.Preparation == ImagePreparationStopped || r.Preparation == ImagePreparationUnknown || r.TextPhase == ImagePartStopped || r.TextPhase == ImagePartUnknown || r.ImagePhase == ImagePartStopped || r.ImagePhase == ImagePartUnknown {
			return false
		}
		return true
	}
	switch r.StopPhase {
	case ImageStopPreparation:
		return (r.Preparation == ImagePreparationStopped || r.Preparation == ImagePreparationUnknown) && r.TextPhase == ImagePartStopped && r.ImagePhase == ImagePartStopped
	case ImageStopText:
		return r.Preparation == ImagePreparationReady && (r.TextPhase == ImagePartStopped || r.TextPhase == ImagePartUnknown) && r.ImagePhase == ImagePartStopped
	case ImageStopImage:
		return r.Preparation == ImagePreparationReady && r.TextPhase == ImagePartAcknowledged && (r.ImagePhase == ImagePartStopped || r.ImagePhase == ImagePartUnknown)
	}
	return false
}
func scanImageReply(ctx context.Context, row libraryScanner) (ImageReply, error) {
	var r ImageReply
	e := row.Scan(&r.Scope.Account, &r.Scope.User, &r.ID, &r.AssetID, &r.SHA256, &r.TextClientID, &r.ImageClientID, &r.Preparation, &r.TextPhase, &r.ImagePhase, &r.StopPhase, &r.StopCode, &r.CreatedAtMillis, &r.UpdatedAtMillis, &r.TextAcknowledgedAtMillis, &r.ImageAcknowledgedAtMillis)
	if e == sql.ErrNoRows {
		return r, ErrNotFound
	}
	return r, storageError(ctx, e)
}
func getImageReply(ctx context.Context, q rowQuery, sc Scope, id string) (ImageReply, string, string, error) {
	r, e := scanImageReply(ctx, q.QueryRowContext(ctx, "SELECT "+imageReplyColumns+" FROM image_replies WHERE account=? AND user=? AND id=?", sc.Account, sc.User, id))
	if e != nil {
		return r, "", "", e
	}
	var state, reply string
	if e = q.QueryRowContext(ctx, "SELECT state,reply FROM messages WHERE account=? AND user=? AND id=?", sc.Account, sc.User, id).Scan(&state, &reply); e != nil {
		return r, "", "", storageError(ctx, e)
	}
	if !validImageReply(r, state, reply) {
		return r, "", "", ErrInvalid
	}
	return r, state, reply, nil
}
func imageAsset(ctx context.Context, q rowQuery, r ImageReply) error {
	if e := libraryPreflight(ctx, q); e != nil {
		return e
	}
	a, e := libraryGet(ctx, q, r.Scope, r.AssetID)
	if e != nil {
		return e
	}
	if a.SHA256 != r.SHA256 || a.State != "ready" || !a.Enabled || a.Frames != 1 || (a.Format != "png" && a.Format != "jpeg") {
		return ErrInvalid
	}
	return nil
}
func (s *Store) PrepareImageReply(ctx context.Context, sc Scope, id string, assetID int64, digest, textClientID, imageClientID string) error {
	if sc.validate() != nil || !validMessageID(id) {
		return ErrInvalid
	}
	text, image := imageClientIDs(sc, id)
	if assetID <= 0 || !imageDigest(digest) || textClientID != text || imageClientID != image {
		return ErrInvalid
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return storageError(ctx, e)
	}
	defer tx.Rollback()
	_, bytes, e := messageBudget(ctx, tx)
	if e != nil {
		return e
	}
	if bytes > (64<<20)-imageReplyReservation {
		return ErrCapacity
	}
	var parent, reply string
	if e = tx.QueryRowContext(ctx, "SELECT state,reply FROM messages WHERE account=? AND user=? AND id=?", sc.Account, sc.User, id).Scan(&parent, &reply); e == sql.ErrNoRows {
		return ErrNotFound
	} else if e != nil {
		return storageError(ctx, e)
	}
	if parent != "processing" || reply != "" {
		return ErrInvalid
	}
	var exists int
	if e = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM image_replies WHERE account=? AND user=? AND id=?)", sc.Account, sc.User, id).Scan(&exists); e != nil {
		return storageError(ctx, e)
	}
	if exists != 0 {
		return ErrStale
	}
	r := ImageReply{Scope: sc, ID: id, AssetID: assetID, SHA256: digest}
	if e = imageAsset(ctx, tx, r); e != nil {
		return e
	}
	now := time.Now().UTC().UnixMilli()
	_, e = tx.ExecContext(ctx, `INSERT INTO image_replies VALUES(?,?,?,?,?,?,?,'planned','planned','planned','','',?,?,0,0)`, sc.Account, sc.User, id, assetID, digest, textClientID, imageClientID, now, now)
	if e != nil {
		return storageError(ctx, e)
	}
	return storageError(ctx, tx.Commit())
}
func (s *Store) imageReplyMutation(ctx context.Context, sc Scope, id string, fn func(*sql.Tx, *ImageReply, string, string) error) error {
	if sc.validate() != nil || !validMessageID(id) {
		return ErrInvalid
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return storageError(ctx, e)
	}
	defer tx.Rollback()
	if e = imageReplyPreflight(ctx, tx); e != nil {
		return e
	}
	r, parent, reply, e := getImageReply(ctx, tx, sc, id)
	if e != nil {
		return e
	}
	if e = fn(tx, &r, parent, reply); e != nil {
		return e
	}
	if _, _, _, e = getImageReply(ctx, tx, sc, id); e != nil {
		return e
	}
	return storageError(ctx, tx.Commit())
}
func saveImageReply(ctx context.Context, tx *sql.Tx, r *ImageReply) error {
	now := time.Now().UTC().UnixMilli()
	if now < r.UpdatedAtMillis {
		now = r.UpdatedAtMillis
	}
	r.UpdatedAtMillis = now
	result, e := tx.ExecContext(ctx, `UPDATE image_replies SET preparation=?,text_phase=?,image_phase=?,stop_phase=?,stop_code=?,updated_at=?,text_acked_at=?,image_acked_at=? WHERE account=? AND user=? AND id=?`, r.Preparation, r.TextPhase, r.ImagePhase, r.StopPhase, r.StopCode, r.UpdatedAtMillis, r.TextAcknowledgedAtMillis, r.ImageAcknowledgedAtMillis, r.Scope.Account, r.Scope.User, r.ID)
	return changed(ctx, result, e)
}
func (s *Store) EnterImagePreparation(ctx context.Context, sc Scope, id string) error {
	return s.imageReplyMutation(ctx, sc, id, func(tx *sql.Tx, r *ImageReply, _, _ string) error {
		if r.StopPhase != "" || r.Preparation != ImagePreparationPlanned {
			return ErrStale
		}
		r.Preparation = ImagePreparationExternal
		return saveImageReply(ctx, tx, r)
	})
}
func (s *Store) FinishImagePreparation(ctx context.Context, sc Scope, id string) error {
	return s.imageReplyMutation(ctx, sc, id, func(tx *sql.Tx, r *ImageReply, _, _ string) error {
		if r.StopPhase != "" || r.Preparation != ImagePreparationExternal {
			return ErrStale
		}
		r.Preparation = ImagePreparationReady
		return saveImageReply(ctx, tx, r)
	})
}
func (s *Store) BeginImageReplyText(ctx context.Context, sc Scope, id string) error {
	return s.imageReplyMutation(ctx, sc, id, func(tx *sql.Tx, r *ImageReply, parent, _ string) error {
		if r.StopPhase != "" || r.Preparation != ImagePreparationReady || r.TextPhase != ImagePartPlanned || parent != "processing" {
			return ErrStale
		}
		r.TextPhase = ImagePartSending
		if _, e := tx.ExecContext(ctx, "UPDATE messages SET state='sending',updated_at=? WHERE account=? AND user=? AND id=?", time.Now().UTC().UnixMilli(), sc.Account, sc.User, id); e != nil {
			return storageError(ctx, e)
		}
		return saveImageReply(ctx, tx, r)
	})
}
func (s *Store) ConfirmImageReplyText(ctx context.Context, sc Scope, id, reply string) error {
	if strings.TrimSpace(reply) == "" || !validField(reply, 16384) {
		return ErrInvalid
	}
	return s.imageReplyMutation(ctx, sc, id, func(tx *sql.Tx, r *ImageReply, parent, old string) error {
		if r.TextPhase == ImagePartAcknowledged {
			if old == reply {
				return nil
			}
			return ErrStale
		}
		if r.StopPhase != "" || r.Preparation != ImagePreparationReady || r.TextPhase != ImagePartSending || parent != "sending" {
			return ErrStale
		}
		r.TextPhase = ImagePartAcknowledged
		r.TextAcknowledgedAtMillis = max(time.Now().UTC().UnixMilli(), r.UpdatedAtMillis)
		if _, e := tx.ExecContext(ctx, "UPDATE messages SET reply=?,state='sent',updated_at=? WHERE account=? AND user=? AND id=?", reply, r.TextAcknowledgedAtMillis, sc.Account, sc.User, id); e != nil {
			return storageError(ctx, e)
		}
		return saveImageReply(ctx, tx, r)
	})
}
func (s *Store) BeginImageReplyImage(ctx context.Context, sc Scope, id string) error {
	return s.imageReplyMutation(ctx, sc, id, func(tx *sql.Tx, r *ImageReply, parent, _ string) error {
		if r.StopPhase != "" || r.Preparation != ImagePreparationReady || r.TextPhase != ImagePartAcknowledged || r.ImagePhase != ImagePartPlanned || parent != "sent" {
			return ErrStale
		}
		if e := imageAsset(ctx, tx, *r); e != nil {
			return e
		}
		r.ImagePhase = ImagePartSending
		return saveImageReply(ctx, tx, r)
	})
}
func (s *Store) ConfirmImageReplyImage(ctx context.Context, sc Scope, id string, atMillis int64) error {
	if !imageStamp(atMillis) {
		return ErrInvalid
	}
	return s.imageReplyMutation(ctx, sc, id, func(tx *sql.Tx, r *ImageReply, parent, _ string) error {
		if r.ImagePhase == ImagePartAcknowledged {
			if atMillis == r.ImageAcknowledgedAtMillis {
				return nil
			}
			return ErrStale
		}
		if r.StopPhase != "" || r.TextPhase != ImagePartAcknowledged || r.ImagePhase != ImagePartSending || parent != "sent" || atMillis < r.TextAcknowledgedAtMillis {
			return ErrStale
		}
		if e := imageAsset(ctx, tx, *r); e != nil {
			return e
		}
		a, e := libraryGet(ctx, tx, sc, r.AssetID)
		if e != nil {
			return e
		}
		if a.Uses == math.MaxInt64 {
			return ErrCapacity
		}
		result, e := tx.ExecContext(ctx, `UPDATE library_assets SET uses=uses+1,last_used=max(last_used,?) WHERE account=? AND user=? AND id=? AND sha256=? AND state='ready' AND enabled=1 AND uses=?`, atMillis, sc.Account, sc.User, r.AssetID, r.SHA256, a.Uses)
		if e = changed(ctx, result, e); e != nil {
			return e
		}
		r.ImagePhase = ImagePartAcknowledged
		r.ImageAcknowledgedAtMillis = atMillis
		r.UpdatedAtMillis = max(r.UpdatedAtMillis, atMillis)
		return saveImageReply(ctx, tx, r)
	})
}
func (s *Store) StopImageReply(ctx context.Context, sc Scope, id, phase, code string) error {
	if !imageStopCode(code) || (phase != ImageStopPreparation && phase != ImageStopText && phase != ImageStopImage) {
		return ErrInvalid
	}
	return s.imageReplyMutation(ctx, sc, id, func(tx *sql.Tx, r *ImageReply, parent, _ string) error {
		if r.StopPhase != "" {
			if r.StopPhase == phase && r.StopCode == code {
				return nil
			}
			return ErrStale
		}
		state := parent
		switch phase {
		case ImageStopPreparation:
			if r.Preparation != ImagePreparationPlanned && r.Preparation != ImagePreparationExternal {
				return ErrStale
			}
			if r.TextPhase != ImagePartPlanned {
				return ErrStale
			}
			if r.Preparation == ImagePreparationExternal {
				r.Preparation = ImagePreparationUnknown
			} else {
				r.Preparation = ImagePreparationStopped
			}
			r.TextPhase = ImagePartStopped
			r.ImagePhase = ImagePartStopped
			state = "failed"
		case ImageStopText:
			if r.Preparation != ImagePreparationReady {
				return ErrStale
			}
			switch r.TextPhase {
			case ImagePartPlanned:
				r.TextPhase = ImagePartStopped
				state = "failed"
			case ImagePartSending:
				r.TextPhase = ImagePartUnknown
				state = "ambiguous"
			default:
				return ErrStale
			}
			r.ImagePhase = ImagePartStopped
		case ImageStopImage:
			if r.TextPhase != ImagePartAcknowledged {
				return ErrStale
			}
			switch r.ImagePhase {
			case ImagePartPlanned:
				r.ImagePhase = ImagePartStopped
			case ImagePartSending:
				r.ImagePhase = ImagePartUnknown
			default:
				return ErrStale
			}
		}
		r.StopPhase, r.StopCode = phase, code
		if state != parent {
			if _, e := tx.ExecContext(ctx, "UPDATE messages SET state=?,updated_at=? WHERE account=? AND user=? AND id=?", state, time.Now().UTC().UnixMilli(), sc.Account, sc.User, id); e != nil {
				return storageError(ctx, e)
			}
		}
		return saveImageReply(ctx, tx, r)
	})
}
func (s *Store) ImageReplyReceipt(ctx context.Context, sc Scope, id string) (ImageReply, error) {
	if sc.validate() != nil || !validMessageID(id) {
		return ImageReply{}, ErrInvalid
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return ImageReply{}, storageError(ctx, e)
	}
	defer tx.Rollback()
	if e = imageReplyPreflight(ctx, tx); e != nil {
		return ImageReply{}, e
	}
	r, _, _, e := getImageReply(ctx, tx, sc, id)
	return r, e
}
