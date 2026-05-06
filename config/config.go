package config

import (
	"flag"
	"os"
	"time"
)

type AppConfig struct {
	StartDate        string
	EndDate          string
	BearerToken      string
	MongoURI         string
	MaxRetries       int
	RetryDelay       time.Duration
	Concurrency      int
	PerPage          int
	Area             string
	ProfessionalRole string
	ErrorOnly        bool
	SearchText       string
}

func LoadConfig() *AppConfig {
	fromDate := time.Now().AddDate(0, 0, -2).Format("2006-01-02")
	from := flag.String("from", fromDate, "Start date in YYYY-MM-DD format")
	toDate := time.Now().Format("2006-01-02")
	to := flag.String("to", toDate, "End date in YYYY-MM-DD format")
	errorOnly := flag.Bool("erronly", false, "Log only errors")
	area := flag.String("area", "1", "Vacancies area")
	text := flag.String("text", "C%2B%2B+or+golang", "Search by text")
	flag.Parse()

	return &AppConfig{
		StartDate:        *from,
		EndDate:          *to,
		BearerToken:      os.Getenv("BEARER_TOKEN"),
		MongoURI:         os.Getenv("MONGO_URI"),
		MaxRetries:       3,
		RetryDelay:       10 * time.Second,
		Concurrency:      10,
		PerPage:          100,
		Area:             *area,
		ProfessionalRole: "96",
		ErrorOnly:        *errorOnly,
		SearchText:       *text,
	}
}
