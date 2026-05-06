package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"hh_it_scrapper/api"
	"hh_it_scrapper/config"
	"hh_it_scrapper/logger"
	"hh_it_scrapper/processor"
	"hh_it_scrapper/storage"
)

func main() {
	cfg := config.LoadConfig()
	if cfg.BearerToken == "" {
		log.Fatal("BEARER_TOKEN must be provided")
	}
	if cfg.MongoURI == "" {
		log.Fatal("MONGO_URI must be provided")
	}

	logger := logger.NewAppLogger(cfg.ErrorOnly)

	mongoStore, err := storage.NewMongoStore(cfg.MongoURI, "vacancy_db", "vacancies")
	if err != nil {
		logger.Error.Fatalf("MongoDB connection error: %v", err)
	}
	defer mongoStore.Collection.Database().Client().Disconnect(context.Background())

	if err := mongoStore.LoadExistingData(); err != nil {
		logger.Error.Fatalf("Failed to load existing data: %v", err)
	}

	hhClient := api.NewHHClient(cfg.BearerToken)

	vacProcCfg := processor.NewProcessorConfig(cfg)
	vacProc := processor.NewProcessor(vacProcCfg, mongoStore, hhClient, logger)

	startTime := time.Now()
	logger.Info.Println("Job started...")
	savedCount, err := vacProc.FetchAndStoreVacancies(context.Background())
	if err != nil {
		logger.Error.Printf("Job failed: %v", err)
	} else {
		logger.Info.Println("Job completed successfully.")
	}
	duration := time.Since(startTime)
	logger.Info.Printf("Duration: %v", duration)

	fmt.Printf("Number of successfully saved vacancies: %d\n", savedCount)
}
