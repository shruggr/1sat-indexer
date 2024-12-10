package pgstore

import (
	"context"
	"log"
	"sync"

	"github.com/shruggr/1sat-indexer/v5/idx"
	"github.com/shruggr/1sat-indexer/v5/jb"
)

func (p *PGStore) AcctsByOwners(ctx context.Context, owners []string) ([]string, error) {
	if len(owners) == 0 {
		return nil, nil
	}
	rows, err := p.DB.Query(ctx, `SELECT account 
		FROM owner_accounts 
		WHERE owner = ANY($1)`,
		owners,
	)
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

func (p *PGStore) AcctOwners(ctx context.Context, account string) ([]string, error) {
	if rows, err := p.DB.Query(ctx, `SELECT owner 
		FROM owner_accounts 
		WHERE account = $1`,
		account,
	); err != nil {
		return nil, err
	} else {
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
}

func (p *PGStore) UpdateAccount(ctx context.Context, account string, owners []string) error {
	for _, owner := range owners {
		if owner == "" {
			continue
		}
		if result, err := p.DB.Exec(ctx, `INSERT INTO owner_accounts(owner, account)
			VALUES ($1, $2)
			ON CONFLICT(owner) DO UPDATE 
				SET account=$2
				WHERE owner_accounts.account!=$2`,
			owner,
			account,
		); err != nil {
			log.Panic(err)
			return err
		} else if result.RowsAffected() == 0 {
			continue
		} else if _, err := p.DB.Exec(ctx, `INSERT INTO logs(search_key, member, score)
			SELECT $1, member, score
			FROM logs
			WHERE search_key=$2
			ON CONFLICT (search_key, member) DO NOTHING`,
			idx.AccountKey(account),
			idx.OwnerKey(owner),
		); err != nil {
			log.Panic(err)
			return err
		}
	}
	return nil
}

func (p *PGStore) SyncAcct(ctx context.Context, tag string, acct string, ing *idx.IngestCtx) error {
	if owners, err := p.AcctOwners(ctx, acct); err != nil {
		return err
	} else {
		for _, owner := range owners {
			if err := p.SyncOwner(ctx, tag, owner, ing); err != nil {
				log.Panic(err)
				return err
			}
		}
	}
	return nil
}

func (p *PGStore) SyncOwner(ctx context.Context, tag string, own string, ing *idx.IngestCtx) error {
	log.Println("Syncing:", own)
	var lastHeight float64
	if err := p.DB.QueryRow(ctx, `SELECT sync_height 
		FROM owner_accounts 
		WHERE owner=$1`,
		own,
	).Scan(&lastHeight); err != nil {
		log.Panic(err)
		return err
	} else if addTxns, err := jb.FetchOwnerTxns(own, int(lastHeight)); err != nil {
		log.Panic(err)
		return err
	} else {
		limiter := make(chan struct{}, ing.Concurrency)
		var wg sync.WaitGroup
		for _, addTxn := range addTxns {
			if score, err := p.LogScore(ctx, tag, addTxn.Txid); err != nil {
				log.Panic(err)
				return err
			} else if score > 0 {
				continue
			}
			wg.Add(1)
			limiter <- struct{}{}
			go func(addTxn *jb.AddressTxn) {
				defer func() {
					<-limiter
					wg.Done()
				}()
				if _, err := ing.IngestTxid(ctx, addTxn.Txid, idx.AncestorConfig{
					Load:  true,
					Parse: true,
					Save:  true,
				}); err != nil {
					log.Panic(err)
				}
			}(addTxn)

			if addTxn.Height > uint32(lastHeight) {
				lastHeight = float64(addTxn.Height)
			}
		}
		wg.Wait()
		if _, err := p.DB.Exec(ctx, `UPDATE owner_accounts
			SET sync_height=$2
			WHERE owner=$1`,
			own,
			lastHeight,
		); err != nil {
			log.Panic(err)
			return err
		}
	}

	return nil
}
