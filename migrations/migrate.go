package main

import (
	"log"
	"os"

	"github.com/golang-migrate/migrate"
	_ "github.com/golang-migrate/migrate/database/postgres"
	_ "github.com/golang-migrate/migrate/source/file"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load("../.env")
	m, err := migrate.New(
		"file://",
		os.Getenv("POSTGRES"),
	)
	if err != nil {
		log.Print(err)
	}
	if err = m.Up(); err != nil {
		log.Print(err)
	}
}
