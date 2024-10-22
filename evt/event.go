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
	godotenv.Load(fmt.Sprintf(`%s/../../.env`, wd))

	log.Println("REDISEVT", os.Getenv("REDISEVT"))
	db = redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDISEVT"),
		Password: "", // no password set
		DB:       0,  // use default DB
	})
}

func Publish(ctx context.Context, event string, data string) error {
	return db.Publish(ctx, event, data).Err()
}
