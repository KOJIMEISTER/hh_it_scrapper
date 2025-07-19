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
}

func LoadConfig() *AppConfig {
	from := flag.String("from", "", "Start date in YYYY-MM-DD format (required)")
	to := flag.String("to", "", "End date in YYYY-MM-DD format (required)")
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
	}
}
