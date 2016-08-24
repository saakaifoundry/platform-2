// Copyright (c) 2016 Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package store

import (
	"github.com/mattermost/platform/model"

	"github.com/go-gorp/gorp"
)

type SqlReactionStore struct {
	*SqlStore
}

func NewSqlReactionStore(sqlStore *SqlStore) ReactionStore {
	s := &SqlReactionStore{sqlStore}

	for _, db := range sqlStore.GetAllConns() {
		table := db.AddTableWithName(model.Reaction{}, "Reactions").SetKeys(false, "UserId", "PostId", "EmojiName")
		table.ColMap("UserId").SetMaxSize(26)
		table.ColMap("PostId").SetMaxSize(26)
		table.ColMap("EmojiName").SetMaxSize(64)
	}

	return s
}

func (s SqlReactionStore) UpgradeSchemaIfNeeded() {
}

func (s SqlReactionStore) CreateIndexesIfNotExists() {
	s.CreateIndexIfNotExists("idx_reactions_post_id", "Reactions", "PostId")
}

func (s SqlReactionStore) Save(reaction *model.Reaction) StoreChannel {
	storeChannel := make(StoreChannel)

	go func() {
		result := StoreResult{}

		if result.Err = reaction.IsValid(); result.Err != nil {
			storeChannel <- result
			close(storeChannel)
			return
		}

		if transaction, err := s.GetMaster().Begin(); err != nil {
			result.Err = model.NewLocAppError("SqlReactionStore.Save", "store.sql_reaction.save.begin.app_error", nil, "")
		} else {
			err := saveReactionAndUpdatePost(transaction, reaction)

			if err != nil {
				transaction.Rollback()

				result.Err = model.NewLocAppError("SqlPreferenceStore.Save", "store.sql_reaction.save.save.app_error", nil, err.Error())
			} else if err := transaction.Commit(); err != nil {
				// don't need to rollback here since the transaction is already closed
				result.Err = model.NewLocAppError("SqlPreferenceStore.Save", "store.sql_preference.save.commit.app_error", nil, err.Error())
			} else {
				result.Data = reaction
			}
		}

		storeChannel <- result
		close(storeChannel)
	}()

	return storeChannel
}

func (s SqlReactionStore) Delete(reaction *model.Reaction) StoreChannel {
	storeChannel := make(StoreChannel)

	go func() {
		result := StoreResult{}

		if result.Err = reaction.IsValid(); result.Err != nil {
			storeChannel <- result
			close(storeChannel)
			return
		}

		if transaction, err := s.GetMaster().Begin(); err != nil {
			result.Err = model.NewLocAppError("SqlReactionStore.Save", "store.sql_reaction.delete.begin.app_error", nil, "")
		} else {
			err := deleteReactionAndUpdatePost(transaction, reaction)

			if err != nil {
				transaction.Rollback()

				result.Err = model.NewLocAppError("SqlPreferenceStore.Save", "store.sql_reaction.delete.app_error", nil, err.Error())
			} else if err := transaction.Commit(); err != nil {
				// don't need to rollback here since the transaction is already closed
				result.Err = model.NewLocAppError("SqlPreferenceStore.Save", "store.sql_preference.delete.commit.app_error", nil, err.Error())
			} else {
				result.Data = reaction
			}
		}

		storeChannel <- result
		close(storeChannel)
	}()

	return storeChannel
}

func saveReactionAndUpdatePost(transaction *gorp.Transaction, reaction *model.Reaction) error {
	if count, err := transaction.SelectInt(
		`SELECT
			COUNT(0)
		FROM
			Reactions
		WHERE
			PostId = :PostId AND
			UserId = :UserId AND
			EmojiName = :EmojiName`,
		map[string]interface{}{"PostId": reaction.PostId, "UserId": reaction.UserId, "EmojiName": reaction.EmojiName}); err != nil {
		return err
	} else if count != 0 {
		// reaction already exists, just return
		return nil
	}

	if err := transaction.Insert(reaction); err != nil {
		return err
	}

	return updatePostForReactions(transaction, reaction.PostId)
}

func deleteReactionAndUpdatePost(transaction *gorp.Transaction, reaction *model.Reaction) error {
	if _, err := transaction.Delete(reaction); err != nil {
		return err
	}

	return updatePostForReactions(transaction, reaction.PostId)
}

func updatePostForReactions(transaction *gorp.Transaction, postId string) error {
	// set HasReactions = true iff the post has reactions, update UpdateAt only if HasReactions changes
	_, err := transaction.Exec(
		`UPDATE
			Posts
		SET
			UpdateAt = (CASE
				WHEN HasReactions != (SELECT count(0) > 0 FROM Reactions WHERE PostId = :PostId) THEN :UpdateAt
				ELSE UpdateAt
			END),
			HasReactions = (SELECT count(0) > 0 FROM Reactions WHERE PostId = :PostId)
		WHERE
			Id = :PostId`,
		map[string]interface{}{"PostId": postId, "UpdateAt": model.GetMillis()},
	)

	return err
}

func (s SqlReactionStore) List(postId string) StoreChannel {
	storeChannel := make(StoreChannel)

	go func() {
		result := StoreResult{}

		var reactions []*model.Reaction

		if _, err := s.GetReplica().Select(&reactions,
			`SELECT
				*
			FROM
				Reactions
			WHERE
				PostId = :PostId`, map[string]interface{}{"PostId": postId}); err != nil {
			result.Err = model.NewLocAppError("SqlReactionStore.List", "store.sql_reaction.list.app_error", nil, "")
		} else {
			result.Data = reactions
		}

		storeChannel <- result
		close(storeChannel)
	}()

	return storeChannel
}
