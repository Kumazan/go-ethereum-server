package db

import (
	"fmt"
	"log"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const (
	migrationPath = "file://db/migrations"
)

var (
	dbName     = os.Getenv("POSTGRES_DB")
	dbUser     = os.Getenv("POSTGRES_USER")
	dbPassword = os.Getenv("POSTGRES_PASSWORD")

	dbHost = os.Getenv("POSTGRES_HOST")
	dbPort = os.Getenv("POSTGRES_PORT")
)

func init() {
	dbURL := fmt.Sprintf("postgres://%s:%s@%s:%v/%s?sslmode=disable", dbUser, dbPassword, dbHost, dbPort, dbName)

	m, err := migrate.New(migrationPath, dbURL)
	if err != nil {
		log.Fatalf("migrate.New failed: %+v", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatalf("Migrate failed: %+v", err)
	}
}

func New() *gorm.DB {
	dsn := fmt.Sprintf("host=%s dbname=%s user=%s password=%s port=%v sslmode=disable", dbHost, dbName, dbUser, dbPassword, dbPort)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		CreateBatchSize: 1000,
	})
	if err != nil {
		log.Fatalf("failed to connect database: %+v", err)
	}

	return db
}
