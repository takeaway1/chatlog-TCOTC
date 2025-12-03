package repository

import (
	"context"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/sjzar/chatlog/internal/model"
	"github.com/sjzar/chatlog/internal/wechatdb/msgstore"
)

// IndexMessages incrementally indexes the provided messages into the FTS cache.
func (r *Repository) IndexMessages(ctx context.Context, messages []*model.Message) error {
	if len(messages) == 0 || r == nil {
		return nil
	}

	log.Debug().Int("count", len(messages)).Msg("incremental index: received messages")

	if r.index == nil {
		log.Debug().Msg("incremental index: index not initialized, skipping")
		return nil
	}

	r.indexMu.Lock()
	status := r.indexStatus
	r.indexMu.Unlock()

	if status.InProgress || !status.Ready {
		log.Debug().Bool("in_progress", status.InProgress).Bool("ready", status.Ready).Msg("incremental index: index not ready, skipping")
		return nil
	}

	batches := make(map[string][]*model.Message)
	stores := make(map[string]*msgstore.Store)

	for _, msg := range messages {
		if msg == nil {
			continue
		}

		store, err := r.ds.LocateMessageStore(msg)
		if err != nil {
			log.Warn().Err(err).Str("talker", msg.Talker).Msg("locate message store for incremental index failed")
			continue
		}
		if store == nil {
			log.Warn().Str("talker", msg.Talker).Msg("skip incremental index: message store not found")
			continue
		}

		batches[store.ID] = append(batches[store.ID], msg)
		if _, ok := stores[store.ID]; !ok {
			stores[store.ID] = store
		}
	}

	if len(batches) == 0 {
		log.Debug().Msg("incremental index: no valid batches to index")
		return nil
	}

	log.Debug().Int("batches", len(batches)).Msg("incremental index: processing batches")

	for id, batch := range batches {
		store := stores[id]
		if len(batch) == 0 || store == nil {
			continue
		}
		if err := r.index.IndexStoreMessages(store, batch); err != nil {
			return err
		}
	}

	fp, err := r.ds.GetDatasetFingerprint(ctx)
	if err != nil {
		log.Debug().Err(err).Msg("get dataset fingerprint for incremental index failed")
		return nil
	}

	if strings.TrimSpace(fp) == "" {
		return nil
	}

	if err := r.index.UpdateFingerprint(fp); err != nil {
		return err
	}

	r.indexMu.Lock()
	r.indexFingerprint = fp
	r.indexStatus.LastCompletedAt = time.Now()
	r.indexMu.Unlock()

	log.Debug().Str("fingerprint", fp).Msg("incremental index: completed successfully")

	return nil
}
