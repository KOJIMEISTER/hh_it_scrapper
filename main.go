package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"hh_it_scrapper/api"
	"hh_it_scrapper/config"
	"hh_it_scrapper/logger"
	"hh_it_scrapper/storage"
)

func main() {
	cfg := config.LoadConfig()
	if cfg.StartDate == "" || cfg.EndDate == "" {
		log.Fatal("Both --from and --to date arguments must be provided")
	}
	if cfg.BearerToken == "" {
		log.Fatal("BEARER_TOKEN must be provided")
	}
	if cfg.MongoURI == "" {
		log.Fatal("MONGO_URI must be provided")
	}

	logger := logger.NewAppLogger()
	mongoStore, err := storage.NewMongoStore(cfg.MongoURI, "vacancy_db", "vacancies")
	if err != nil {
		logger.Error.Fatalf("MongoDB connection error: %v", err)
	}
	defer mongoStore.Collection.Database().Client().Disconnect(context.Background())

	if err := mongoStore.LoadExistingData(); err != nil {
		logger.Error.Fatalf("Failed to load existing data: %v", err)
	}

	hhClient := api.NewHHClient(cfg.BearerToken)

	startTime := time.Now()
	logger.Info.Println("Job started...")
	savedCount, err := fetchAndStoreVacancies(context.Background(), cfg, mongoStore, hhClient, logger)
	if err != nil {
		logger.Error.Printf("Job failed: %v", err)
	} else {
		logger.Info.Println("Job completed successfully.")
	}
	duration := time.Since(startTime)
	logger.Info.Printf("Duration: %v", duration)

	fmt.Printf("Number of successfully saved vacancies: %d\n", savedCount)
}

func fetchAndStoreVacancies(ctx context.Context, cfg *config.AppConfig, store *storage.MongoStore, client *api.HHClient, logger *logger.AppLogger) (int64, error) {
	var savedCount int64
	page := 0
	var totalPages int

	for {
		select {
		case <-ctx.Done():
			return atomic.LoadInt64(&savedCount), ctx.Err()
		default:
			vacancyIDs, pages, err := client.GetVacancyIDs(ctx, cfg.StartDate, cfg.EndDate, cfg.Area, cfg.ProfessionalRole, page, cfg.PerPage)
			if err != nil {
				logger.Error.Printf("Failed to fetch search page %d: %v", page, err)
				page++
				continue
			}

			if page == 0 {
				totalPages = pages
				logger.Info.Printf("Total pages to fetch: %d", totalPages)
			}

			var newIDs []string
			for _, id := range vacancyIDs {
				if !store.VacancyExists(id) {
					newIDs = append(newIDs, id)
				}
			}

			logger.Info.Printf("Processing page %d: %d new vacancies found", page, len(newIDs))
			if len(newIDs) > 0 {
				if err := fetchAndProcessVacancies(ctx, client, store, newIDs, cfg.MaxRetries, cfg.RetryDelay, &savedCount, logger); err != nil {
					logger.Error.Printf("Failed to process vacancies: %v", err)
				}
			}

			if page >= totalPages-1 {
				return atomic.LoadInt64(&savedCount), nil
			}
			page++
		}
	}
}

func fetchAndProcessVacancies(ctx context.Context, client *api.HHClient, store *storage.MongoStore, ids []string, maxRetries int, retryDelay time.Duration, savedCount *int64, logger *logger.AppLogger) error {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10) // Concurrency control

	for _, id := range ids {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case sem <- struct{}{}:
			wg.Add(1)
			go func(vacancyID string) {
				defer wg.Done()
				defer func() { <-sem }()

				for retries := 0; retries <= maxRetries; retries++ {
					if err := processVacancy(ctx, client, store, vacancyID, savedCount, logger); err == nil {
						return
					} else if retries < maxRetries {
						logger.Error.Printf("Retrying vacancy %s (%d/%d): %v", vacancyID, retries+1, maxRetries, err)
						time.Sleep(retryDelay)
					} else {
						logger.Error.Printf("Failed to process vacancy %s after %d retries: %v", vacancyID, maxRetries, err)
					}
				}
			}(id)
		}
	}

	wg.Wait()
	return nil
}

func processVacancy(ctx context.Context, client *api.HHClient, store *storage.MongoStore, vacancyID string, savedCount *int64, logger *logger.AppLogger) error {
	data, err := client.GetVacancyDetails(ctx, vacancyID)
	if err != nil {
		if errors.Is(err, api.ErrVacancyNotFound) {
			logger.Info.Printf("Vacancy %s not found, skipping", vacancyID)
			return nil
		}
		return fmt.Errorf("failed to get vacancy details: %w", err)
	}

	description, ok := data["description"].(string)
	if !ok || description == "" {
		return fmt.Errorf("vacancy %s has invalid description", vacancyID)
	}

	descriptionHash := api.MD5Hash(description)
	if store.DescriptionHashExists(descriptionHash) {
		logger.Info.Printf("Vacancy %s skipped due to duplicate description", vacancyID)
		return nil
	}

	data["description_hash"] = descriptionHash
	if err := store.UpsertVacancy(data); err != nil {
		return fmt.Errorf("MongoDB insertion error: %w", err)
	}

	store.AddDescriptionHash(descriptionHash)
	atomic.AddInt64(savedCount, 1)
	logger.Info.Printf("Vacancy %s stored successfully", vacancyID)
	return nil
}
