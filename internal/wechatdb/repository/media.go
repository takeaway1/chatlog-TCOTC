package repository

import (
	"context"

	"github.com/takeaway1/chatlog-TCOTC/internal/model"
)

func (r *Repository) GetMedia(ctx context.Context, _type string, key string) (*model.Media, error) {
	return r.ds.GetMedia(ctx, _type, key)
}
