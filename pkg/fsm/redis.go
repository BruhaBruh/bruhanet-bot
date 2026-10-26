package fsm

import (
	"context"
	"errors"
	"strconv"

	"github.com/redis/go-redis/v9"
)

const (
	keyPrefix  = "fsm:"
	fieldState = "state"
	fieldData  = "data"
)

var casScript = redis.NewScript(`
local cur = redis.call('HGET', KEYS[1], 'state')
if cur == false then cur = '' end
if cur ~= ARGV[1] then return 0 end
redis.call('HSET', KEYS[1], 'state', ARGV[2], 'data', ARGV[3])
return 1
`)

type Redis struct {
	rdb *redis.Client
}

func NewRedis(rdb *redis.Client) *Redis {
	return &Redis{rdb: rdb}
}

func (s *Redis) Get(ctx context.Context, userID int64) (Record, error) {
	vals, err := s.rdb.HGetAll(ctx, key(userID)).Result()
	if err != nil {
		return Record{}, errors.Join(ErrStorage, err)
	}
	r := Record{State: State(vals[fieldState])}
	if d := vals[fieldData]; d != "" {
		r.Data = []byte(d)
	}
	return r, nil
}

func (s *Redis) Set(ctx context.Context, userID int64, r Record) error {
	if err := s.rdb.HSet(ctx, key(userID), fieldState, string(r.State), fieldData, r.Data).Err(); err != nil {
		return errors.Join(ErrStorage, err)
	}
	return nil
}

func (s *Redis) Delete(ctx context.Context, userID int64) error {
	if err := s.rdb.Del(ctx, key(userID)).Err(); err != nil {
		return errors.Join(ErrStorage, err)
	}
	return nil
}

func (s *Redis) CompareAndSet(ctx context.Context, userID int64, from State, r Record) error {
	ok, err := casScript.Run(ctx, s.rdb, []string{key(userID)}, string(from), string(r.State), r.Data).Int()
	if err != nil {
		return errors.Join(ErrStorage, err)
	}
	if ok == 0 {
		return ErrStateMismatch
	}
	return nil
}

func key(userID int64) string {
	return keyPrefix + strconv.FormatInt(userID, 10)
}
