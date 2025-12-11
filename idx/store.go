package idx

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/b-open-io/overlay/pubsub"
	"github.com/b-open-io/overlay/queue"
	"github.com/shruggr/1sat-indexer/v5/lib"
)

const TxoKey = "txo"
const SatsKey = "sats"
const SpendsKey = "spends"
const TdataKey = "tdata"
const EventsKey = "events"

type QueueStore struct {
	Queue  queue.QueueStorage
	PubSub pubsub.PubSub
}

func NewQueueStore(q queue.QueueStorage, ps pubsub.PubSub) *QueueStore {
	return &QueueStore{Queue: q, PubSub: ps}
}

type storedTxo struct {
	Height   uint32   `json:"height"`
	Idx      uint64   `json:"idx"`
	Satoshis uint64   `json:"satoshis"`
	Owners   []string `json:"owners,omitempty"`
}

func (s *QueueStore) LoadTxo(ctx context.Context, outpoint string, tags []string, spend bool) (*Txo, error) {
	value, err := s.Queue.HGet(ctx, TxoKey, outpoint)
	if err != nil && err.Error() != "redis: nil" {
		return nil, err
	}
	if value == "" {
		return nil, nil
	}

	txo, err := s.parseTxo(outpoint, value)
	if err != nil {
		return nil, err
	}

	if len(tags) > 0 {
		if txo.Data, err = s.loadTagData(ctx, outpoint, tags); err != nil {
			return nil, err
		}
	}

	if spend {
		txo.Spend, _ = s.GetSpend(ctx, outpoint)
	}

	return txo, nil
}

func (s *QueueStore) LoadTxos(ctx context.Context, outpoints []string, tags []string, spend bool) ([]*Txo, error) {
	if len(outpoints) == 0 {
		return nil, nil
	}

	values, err := s.Queue.HMGet(ctx, TxoKey, outpoints...)
	if err != nil {
		return nil, err
	}

	txos := make([]*Txo, len(outpoints))
	for i, outpoint := range outpoints {
		if values[i] == "" {
			continue
		}
		txo, err := s.parseTxo(outpoint, values[i])
		if err != nil {
			return nil, err
		}
		txos[i] = txo
	}

	if len(tags) > 0 {
		for i, outpoint := range outpoints {
			if txos[i] != nil {
				if txos[i].Data, err = s.loadTagData(ctx, outpoint, tags); err != nil {
					return nil, err
				}
			}
		}
	}

	if spend {
		spends, err := s.GetSpends(ctx, outpoints)
		if err != nil {
			return nil, err
		}
		for i, txo := range txos {
			if txo != nil {
				txo.Spend = spends[i]
			}
		}
	}

	return txos, nil
}

func (s *QueueStore) LoadTxosByTxid(ctx context.Context, txid string, tags []string, spend bool) ([]*Txo, error) {
	logs, err := s.Search(ctx, &SearchCfg{
		Keys: []string{"txid:" + txid},
	})
	if err != nil {
		return nil, err
	}

	outpoints := make([]string, len(logs))
	for i, log := range logs {
		outpoints[i] = log.Member
	}

	return s.LoadTxos(ctx, outpoints, tags, spend)
}

func (s *QueueStore) LoadData(ctx context.Context, outpoint string, tags []string) (IndexDataMap, error) {
	return s.loadTagData(ctx, outpoint, tags)
}

func (s *QueueStore) GetSpend(ctx context.Context, outpoint string) (string, error) {
	spend, err := s.Queue.HGet(ctx, SpendsKey, outpoint)
	if err != nil && err.Error() == "redis: nil" {
		return "", nil
	}
	return spend, err
}

func (s *QueueStore) GetSpends(ctx context.Context, outpoints []string) ([]string, error) {
	if len(outpoints) == 0 {
		return nil, nil
	}
	return s.Queue.HMGet(ctx, SpendsKey, outpoints...)
}

func (s *QueueStore) GetSats(ctx context.Context, outpoints []string) ([]uint64, error) {
	if len(outpoints) == 0 {
		return nil, nil
	}
	values, err := s.Queue.HMGet(ctx, SatsKey, outpoints...)
	if err != nil {
		return nil, err
	}
	sats := make([]uint64, len(outpoints))
	for i, v := range values {
		if v != "" {
			sats[i], _ = strconv.ParseUint(v, 10, 64)
		}
	}
	return sats, nil
}

func (s *QueueStore) SetNewSpend(ctx context.Context, outpoint string, txid string) (bool, error) {
	existing, err := s.GetSpend(ctx, outpoint)
	if err != nil {
		return false, err
	}
	if existing != "" {
		return false, nil
	}

	if err := s.Queue.HSet(ctx, SpendsKey, outpoint, txid); err != nil {
		return false, err
	}
	return true, nil
}

func (s *QueueStore) UnsetSpends(ctx context.Context, outpoints []string) error {
	if len(outpoints) == 0 {
		return nil
	}
	return s.Queue.HDel(ctx, SpendsKey, outpoints...)
}

func (s *QueueStore) SaveTxos(idxCtx *IndexContext) error {
	ctx := idxCtx.Ctx
	txid := idxCtx.TxidHex
	score := idxCtx.Score

	for _, txo := range idxCtx.Txos {
		outpoint := txo.Outpoint.String()

		stored := storedTxo{
			Height:   txo.Height,
			Idx:      txo.Idx,
			Satoshis: *txo.Satoshis,
			Owners:   txo.Owners,
		}
		txoJSON, err := json.Marshal(stored)
		if err != nil {
			return err
		}

		if err := s.Queue.HSet(ctx, TxoKey, outpoint, string(txoJSON)); err != nil {
			return err
		}

		if err := s.Queue.HSet(ctx, SatsKey, outpoint, strconv.FormatUint(*txo.Satoshis, 10)); err != nil {
			return err
		}

		events := make([]string, 0, 100)
		events = append(events, "txid:"+txid)

		for tag, data := range txo.Data {
			events = append(events, tag)
			for _, event := range data.Events {
				events = append(events, EventKey(tag, event))
			}
			if data.Data != nil {
				if dataJSON, err := data.MarshalJSON(); err == nil {
					if err := s.Queue.HSet(ctx, TdataKey, outpoint+":"+tag, string(dataJSON)); err != nil {
						return err
					}
				}
			}
		}

		for _, owner := range txo.Owners {
			if owner != "" {
				events = append(events, OwnerKey(owner))
			}
		}

		if len(events) > 0 {
			eventsJSON, err := json.Marshal(events)
			if err != nil {
				return err
			}
			if err := s.Queue.HSet(ctx, EventsKey, outpoint, string(eventsJSON)); err != nil {
				return err
			}
		}

		for _, event := range events {
			if err := s.Queue.ZAdd(ctx, event, queue.ScoredMember{
				Member: outpoint,
				Score:  score,
			}); err != nil {
				return err
			}
		}

		for _, event := range events {
			s.PubSub.Publish(ctx, event, outpoint)
		}
	}

	return nil
}

func (s *QueueStore) SaveSpends(idxCtx *IndexContext) error {
	ctx := idxCtx.Ctx
	score := idxCtx.Score

	for vin, spend := range idxCtx.Spends {
		if spend == nil {
			continue
		}

		outpoint := spend.Outpoint.String()

		if err := s.Queue.HSet(ctx, SpendsKey, outpoint, idxCtx.TxidHex); err != nil {
			return err
		}

		for _, owner := range spend.Owners {
			if owner == "" {
				continue
			}
			ospKey := OwnerSpentKey(owner)

			if err := s.Queue.ZAdd(ctx, ospKey, queue.ScoredMember{
				Member: outpoint,
				Score:  score,
			}); err != nil {
				return err
			}

			eventsStr, _ := s.Queue.HGet(ctx, EventsKey, outpoint)
			var events []string
			if eventsStr != "" {
				json.Unmarshal([]byte(eventsStr), &events)
			}
			events = append(events, ospKey)
			eventsJSON, _ := json.Marshal(events)
			s.Queue.HSet(ctx, EventsKey, outpoint, string(eventsJSON))

			s.PubSub.Publish(ctx, ospKey, fmt.Sprintf("%s:%d", idxCtx.TxidHex, vin))
		}
	}

	return nil
}

func (s *QueueStore) Rollback(ctx context.Context, txid string) error {
	logs, err := s.Search(ctx, &SearchCfg{
		Keys: []string{"txid:" + txid},
	})
	if err != nil {
		return err
	}

	for _, log := range logs {
		outpoint := log.Member

		eventsStr, err := s.Queue.HGet(ctx, EventsKey, outpoint)
		if err != nil && err.Error() != "redis: nil" {
			return err
		}

		if eventsStr != "" {
			var events []string
			if err := json.Unmarshal([]byte(eventsStr), &events); err == nil {
				for _, event := range events {
					s.Queue.ZRem(ctx, event, outpoint)
				}
			}
		}

		s.Queue.HDel(ctx, TxoKey, outpoint)
		s.Queue.HDel(ctx, SatsKey, outpoint)
		s.Queue.HDel(ctx, EventsKey, outpoint)
	}

	return nil
}

func (s *QueueStore) Log(ctx context.Context, key string, id string, score float64) error {
	return s.Queue.ZAdd(ctx, key, queue.ScoredMember{
		Member: id,
		Score:  score,
	})
}

func (s *QueueStore) LogMany(ctx context.Context, key string, logs []queue.ScoredMember) error {
	return s.Queue.ZAdd(ctx, key, logs...)
}

func (s *QueueStore) LogOnce(ctx context.Context, key string, id string, score float64) (bool, error) {
	existing, err := s.Queue.ZScore(ctx, key, id)
	if err != nil && err.Error() != "redis: nil" {
		return false, err
	}
	if existing > 0 {
		return false, nil
	}

	if err := s.Queue.ZAdd(ctx, key, queue.ScoredMember{
		Member: id,
		Score:  score,
	}); err != nil {
		return false, err
	}
	return true, nil
}

func (s *QueueStore) Delog(ctx context.Context, key string, ids ...string) error {
	return s.Queue.ZRem(ctx, key, ids...)
}

func (s *QueueStore) LogScore(ctx context.Context, key string, id string) (float64, error) {
	score, err := s.Queue.ZScore(ctx, key, id)
	if err != nil && err.Error() == "redis: nil" {
		return 0, nil
	}
	return score, err
}

func (s *QueueStore) parseTxo(outpoint string, value string) (*Txo, error) {
	op, err := lib.NewOutpointFromString(outpoint)
	if err != nil {
		return nil, err
	}

	var stored storedTxo
	if err := json.Unmarshal([]byte(value), &stored); err != nil {
		return nil, err
	}

	return &Txo{
		Outpoint: op,
		Height:   stored.Height,
		Idx:      stored.Idx,
		Satoshis: &stored.Satoshis,
		Owners:   stored.Owners,
		Data:     make(IndexDataMap),
		Score:    HeightScore(stored.Height, stored.Idx),
	}, nil
}

func (s *QueueStore) loadTagData(ctx context.Context, outpoint string, tags []string) (IndexDataMap, error) {
	if len(tags) == 0 {
		return nil, nil
	}

	fieldKeys := make([]string, len(tags))
	for i, tag := range tags {
		fieldKeys[i] = outpoint + ":" + tag
	}

	values, err := s.Queue.HMGet(ctx, TdataKey, fieldKeys...)
	if err != nil {
		return nil, err
	}

	data := make(IndexDataMap, len(tags))
	for i, tag := range tags {
		if values[i] != "" {
			data[tag] = &IndexData{
				Data: json.RawMessage(values[i]),
			}
		}
	}
	return data, nil
}
