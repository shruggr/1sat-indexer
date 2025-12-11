package evt

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

func TagKey(tag string) string {
	return "tag:" + tag
}

func EventKey(tag string, event *Event) string {
	return fmt.Sprintf("evt:%s:%s:%s", tag, event.Id, event.Value)
}

type Event struct {
	Id    string `json:"id"`
	Value string `json:"value"`
}

var db *redis.Client

func init() {
	wd, _ := os.Getwd()
	log.Println("CWD:", wd)
	godotenv.Load(".env")

	log.Println("REDISEVT", os.Getenv("REDISEVT"))
	if opts, err := redis.ParseURL(os.Getenv("REDISEVT")); err != nil {
		panic(err)
	} else {
		db = redis.NewClient(opts)
	}
}

func Publish(ctx context.Context, event string, data string) error {
	return db.Publish(ctx, event, data).Err()
}
