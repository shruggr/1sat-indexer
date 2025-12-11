package sqlitestore

import (
	"context"
	"log"
	"strings"

	"github.com/shruggr/1sat-indexer/v5/idx"
)

func (s *SQLiteStore) AcctsByOwners(ctx context.Context, owners []string) ([]string, error) {
	if len(owners) == 0 {
		return nil, nil
	}

	query := `SELECT account FROM owner_accounts WHERE owner IN (` + placeholders(len(owners)) + `)`
	rows, err := s.READDB.QueryContext(ctx, query, toInterfaceSlice(owners)...)
	if err != nil {
		log.Panic(err)
		return nil, err
	}
	defer rows.Close()

	accts := make([]string, 0, 4)
	for rows.Next() {
		var acct string
		if err = rows.Scan(&acct); err != nil {
			log.Panic(err)
			return nil, err
		}
		accts = append(accts, acct)
	}
	return accts, nil
}

func (s *SQLiteStore) AcctOwners(ctx context.Context, account string) ([]string, error) {
	rows, err := s.READDB.QueryContext(ctx, `SELECT owner FROM owner_accounts WHERE account = ?`, account)
	if err != nil {
		log.Panic(err)
		return nil, err
	}
	defer rows.Close()

	owners := make([]string, 0, 4)
	for rows.Next() {
		var owner string
		if err = rows.Scan(&owner); err != nil {
			log.Panic(err)
			return nil, err
		}
		owners = append(owners, owner)
	}
	return owners, nil
}

func (s *SQLiteStore) UpdateAccount(ctx context.Context, account string, owners []string) error {
	log.Printf("UpdateAccount: starting update for account %s with %d owners", account, len(owners))

	for i, owner := range owners {
		if owner == "" {
			continue
		}

		log.Printf("UpdateAccount: processing owner %d/%d: %s", i+1, len(owners), owner)
		// Insert or update owner account mapping
		if _, err := s.WRITEDB.ExecContext(ctx, `INSERT INTO owner_accounts(owner, account)
			VALUES (?, ?)
			ON CONFLICT(owner) DO UPDATE SET account = ?`,
			owner,
			account,
			account,
		); err != nil {
			log.Printf("UpdateAccount: error inserting owner %s: %v", owner, err)
			log.Panic(err)
			return err
		}
	}

	// Batch insert logs for all owners in one query
	if len(owners) > 0 {
		log.Printf("UpdateAccount: batch inserting logs for %d owners", len(owners))
		query := "INSERT INTO logs(search_key, member, score) VALUES "
		args := make([]interface{}, 0, len(owners)*3)
		placeholders := make([]string, 0, len(owners))

		for _, owner := range owners {
			if owner == "" {
				continue
			}
			placeholders = append(placeholders, "(?, ?, ?)")
			args = append(args, idx.OwnerSyncKey, owner, 0)
		}

		query += strings.Join(placeholders, ", ") + " ON CONFLICT DO NOTHING"

		if _, err := s.WRITEDB.ExecContext(ctx, query, args...); err != nil {
			log.Printf("UpdateAccount: error batch inserting logs: %v", err)
			log.Panic(err)
			return err
		}
	}

	log.Printf("UpdateAccount: completed update for account %s", account)
	return nil
}
