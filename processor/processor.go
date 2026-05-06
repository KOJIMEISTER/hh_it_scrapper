package processor

import (
	"context"
	"errors"
	"fmt"
	"hh_it_scrapper/api"
	"hh_it_scrapper/config"
	"hh_it_scrapper/logger"
	"hh_it_scrapper/storage"
	"sync"
	"sync/atomic"
	"time"
)

type ProcessorConfig struct {
	StartDate        string
	EndDate          string
	Area             string
	ProfessionalRole string
	PerPage          int
	MaxRetries       int
	RetryDelay       time.Duration
}

func NewProcessorConfig(cfg *config.AppConfig) *ProcessorConfig {
	return &ProcessorConfig{
		StartDate:        cfg.StartDate,
		EndDate:          cfg.EndDate,
		Area:             cfg.Area,
		ProfessionalRole: cfg.ProfessionalRole,
		PerPage:          cfg.PerPage,
		MaxRetries:       cfg.MaxRetries,
		RetryDelay:       cfg.RetryDelay,
	}
}

type Processor struct {
	cfg    *ProcessorConfig
	store  *storage.MongoStore
	client *api.HHClient
	logger *logger.AppLogger
}

func NewProcessor(cfg *ProcessorConfig, store *storage.MongoStore, client *api.HHClient, logger *logger.AppLogger) *Processor {
	return &Processor{cfg: cfg, store: store, client: client, logger: logger}
}

func (this *Processor) FetchAndStoreVacancies(ctx context.Context) (int64, error) {
	var savedCount int64
	page := 0
	var totalPages int

	for {
		select {
		case <-ctx.Done():
			return atomic.LoadInt64(&savedCount), ctx.Err()
		default:
			vacancyIDs, pages, err := this.client.GetVacancyIDs(ctx, this.cfg.StartDate, this.cfg.EndDate, this.cfg.Area, this.cfg.ProfessionalRole, page, this.cfg.PerPage)
			if err != nil {
				this.logger.Error.Printf("Failed to fetch search page %d: %v", page, err)
				page++
				continue
			}

			if page == 0 {
				totalPages = pages
				this.logger.Info.Printf("Total pages to fetch: %d", totalPages)
			}

			var newIDs []string
			for _, id := range vacancyIDs {
				if !this.store.VacancyExists(id) {
					newIDs = append(newIDs, id)
				}
			}

			this.logger.Info.Printf("Processing page %d: %d new vacancies found", page, len(newIDs))
			if len(newIDs) > 0 {
				if err := this.fetchAndProcessVacancies(ctx, newIDs, &savedCount); err != nil {
					this.logger.Error.Printf("Failed to process vacancies: %v", err)
				}
			}

			if page >= totalPages-1 {
				return atomic.LoadInt64(&savedCount), nil
			}
			page++
		}
	}
}

func (this *Processor) fetchAndProcessVacancies(ctx context.Context, ids []string, savedCount *int64) error {
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

				for retries := 0; retries <= this.cfg.MaxRetries; retries++ {
					if err := this.processVacancy(ctx, vacancyID, savedCount); err == nil {
						return
					} else if retries < this.cfg.MaxRetries {
						this.logger.Error.Printf("Retrying vacancy %s (%d/%d): %v", vacancyID, retries+1, this.cfg.MaxRetries, err)
						time.Sleep(this.cfg.RetryDelay)
					} else {
						this.logger.Error.Printf("Failed to process vacancy %s after %d retries: %v", vacancyID, this.cfg.MaxRetries, err)
					}
				}
			}(id)
		}
	}

	wg.Wait()
	return nil
}

func (this *Processor) processVacancy(ctx context.Context, vacancyID string, savedCount *int64) error {
	data, err := this.client.GetVacancyDetails(ctx, vacancyID)
	if err != nil {
		if errors.Is(err, api.ErrVacancyNotFound) {
			this.logger.Info.Printf("Vacancy %s not found, skipping", vacancyID)
			return nil
		}
		return fmt.Errorf("failed to get vacancy details: %w", err)
	}

	description, ok := data["description"].(string)
	if !ok || description == "" {
		return fmt.Errorf("vacancy %s has invalid description", vacancyID)
	}

	descriptionHash := api.MD5Hash(description)
	if this.store.DescriptionHashExists(descriptionHash) {
		this.logger.Info.Printf("Vacancy %s skipped due to duplicate description", vacancyID)
		return nil
	}

	data["description_hash"] = descriptionHash
	if err := this.store.UpsertVacancy(data); err != nil {
		return fmt.Errorf("MongoDB insertion error: %w", err)
	}

	this.store.AddDescriptionHash(descriptionHash)
	atomic.AddInt64(savedCount, 1)
	this.logger.Info.Printf("Vacancy %s stored successfully", vacancyID)
	return nil
}
