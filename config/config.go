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
}

func LoadConfig() *AppConfig {
	fromDate := time.Now().AddDate(0, 0, -2).Format("2006-01-02")
	from := flag.String("from", fromDate, "Start date in YYYY-MM-DD format")
	to := flag.String("to", "", "End date in YYYY-MM-DD format")
	errorOnly := flag.Bool("--erronly", true, "Log only errors")
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
		Area:             "113",
		ProfessionalRole: "96",
		ErrorOnly:        *errorOnly,
	}
}
