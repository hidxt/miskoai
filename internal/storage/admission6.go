package storage

import "context"

func imageReplyPreflight(ctx context.Context, q rowQuery) error {
	if e := validateLegacyShape(ctx, q); e != nil {
		return e
	}
	cols := []boundedColumn{{"account", 256, "text"}, {"user", 256, "text"}, {"id", 512, "text"}, {"sha256", 64, "text"}, {"text_client_id", 72, "text"}, {"image_client_id", 72, "text"}, {"preparation", 8, "text"}, {"text_phase", 8, "text"}, {"image_phase", 8, "text"}, {"stop_phase", 11, "text"}, {"stop_code", 11, "text"}}
	if e := validateRowShape(ctx, q, "image_replies", cols, `typeof(asset_id)<>'integer' OR asset_id<=0 OR typeof(created_at)<>'integer' OR created_at<1 OR created_at>253402300799999 OR typeof(updated_at)<>'integer' OR updated_at<created_at OR updated_at>253402300799999 OR typeof(text_acked_at)<>'integer' OR text_acked_at<0 OR text_acked_at>updated_at OR typeof(image_acked_at)<>'integer' OR image_acked_at<0 OR image_acked_at>updated_at`); e != nil {
		return e
	}
	var n, parents int64
	if e := q.QueryRowContext(ctx, "SELECT count(*) FROM image_replies").Scan(&n); e != nil {
		return storageError(ctx, e)
	}
	if e := q.QueryRowContext(ctx, "SELECT count(*) FROM messages").Scan(&parents); e != nil {
		return storageError(ctx, e)
	}
	if n > 10000 || n > parents {
		return ErrCapacity
	}
	return nil
}
func validateSchema6IfPresent(ctx context.Context, q rowQuery) error {
	var v int
	if e := q.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v); e != nil {
		return storageError(ctx, e)
	}
	if v != 6 {
		return nil
	}
	if e := imageReplyPreflight(ctx, q); e != nil {
		return e
	}
	if _, _, e := messageBudget(ctx, q); e != nil {
		return e
	}
	if e := noUnsafe(ctx, q, `SELECT EXISTS(SELECT 1 FROM image_replies r WHERE NOT EXISTS(SELECT 1 FROM messages m WHERE m.account=r.account AND m.user=r.user AND m.id=r.id)) OR EXISTS(SELECT 1 FROM image_replies a JOIN image_replies b ON a.text_client_id=b.image_client_id)`); e != nil {
		return e
	}
	rows, e := q.QueryContext(ctx, `SELECT r.account,r.user,r.id,r.asset_id,r.sha256,r.text_client_id,r.image_client_id,r.preparation,r.text_phase,r.image_phase,r.stop_phase,r.stop_code,r.created_at,r.updated_at,r.text_acked_at,r.image_acked_at,m.state,m.reply FROM image_replies r JOIN messages m ON m.account=r.account AND m.user=r.user AND m.id=r.id`)
	if e != nil {
		return storageError(ctx, e)
	}
	for rows.Next() {
		var r ImageReply
		var state, reply string
		if e = rows.Scan(&r.Scope.Account, &r.Scope.User, &r.ID, &r.AssetID, &r.SHA256, &r.TextClientID, &r.ImageClientID, &r.Preparation, &r.TextPhase, &r.ImagePhase, &r.StopPhase, &r.StopCode, &r.CreatedAtMillis, &r.UpdatedAtMillis, &r.TextAcknowledgedAtMillis, &r.ImageAcknowledgedAtMillis, &state, &reply); e != nil {
			rows.Close()
			return storageError(ctx, e)
		}
		if !validImageReply(r, state, reply) {
			rows.Close()
			return ErrInvalid
		}
	}
	return closeValidatedRows(ctx, rows)
}
