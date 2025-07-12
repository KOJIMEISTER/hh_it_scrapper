package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Constants for API
const (
	BaseSearchURL    = "https://api.hh.ru/vacancies"
	BaseVacancyURL   = "https://api.hh.ru/vacancies/"
	PerPage          = 100
	Area             = "113" // Moscow
	ProfessionalRole = "96"  // Programmer Developer
	MaxConcurrency   = 10    // Adjust based on needs
	RetryDelay       = 10 * time.Second
	MaxRetries       = 3
)

// Structures to parse JSON responses
type VacancySearchResponse struct {
	Found     int          `json:"found"`
	Pages     int          `json:"pages"`
	PerPage   int          `json:"per_page"`
	Page      int          `json:"page"`
	Items     []SearchItem `json:"items"`
	Alternate string       `json:"alternate_url"`
}

type SearchItem struct {
	ID string `json:"id"`
}

type Vacancy struct {
	// Define fields as needed; using raw JSON if not needed explicitly
	RawJSON json.RawMessage `json:"-"`
}

// Global logger
var (
	successLogger *log.Logger
	errorLogger   *log.Logger
)

func init() {
	// Initialize loggers
	successFile, err := os.OpenFile("logs/success.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatalf("Failed to open success log file: %v", err)
	}
	errorFile, err := os.OpenFile("logs/error.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatalf("Failed to open error log file: %v", err)
	}
	successLogger = log.New(successFile, "SUCCESS: ", log.Ldate|log.Ltime)
	errorLogger = log.New(errorFile, "ERROR: ", log.Ldate|log.Ltime|log.Lshortfile)
}

func main() {
	// Define command-line flags
	from := flag.String("from", "", "Start date in YYYY-MM-DD format")
	to := flag.String("to", "", "End date in YYYY-MM-DD format")
	flag.Parse()

	var startDate, endDate string

	if *from != "" && *to != "" {
		startDate = *from
		endDate = *to
	} else {
		// If not provided, default to yesterday and today
		today := time.Now().Format("2006-01-02")
		yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
		startDate = yesterday
		endDate = today
	}

	mongoURI := os.Getenv("MONGO_URI")
	bearerToken := os.Getenv("BEARER_TOKEN")

	if bearerToken == "" {
		log.Fatal("BEARER_TOKEN must be provided")
	}

	if mongoURI == "" {
		log.Fatal("MONGO_URI must be provided")
	}

	// Create a MongoDB client
	clientOptions := options.Client().ApplyURI(mongoURI)
	client, err := mongo.Connect(context.TODO(), clientOptions)
	if err != nil {
		log.Fatalf("MongoDB connection error: %v", err)
	}
	defer func() {
		if err = client.Disconnect(context.TODO()); err != nil {
			log.Fatalf("MongoDB disconnection error: %v", err)
		}
	}()

	collection := client.Database("vacancy_db").Collection("vacancies")

	// Schedule the job to run once a day
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	// Run the job immediately and then every day
	runJob(startDate, endDate, collection, bearerToken)

	for range ticker.C {
		// Update dates for each run to increment the day
		yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
		today := time.Now().Format("2006-01-02")
		runJob(yesterday, today, collection, bearerToken)
	}
}

func runJob(startDate, endDate string, collection *mongo.Collection, bearerToken string) {
	log.Println("Job started...")
	err := fetchAndStoreVacancies(startDate, endDate, collection, bearerToken)
	if err != nil {
		errorLogger.Printf("Job failed: %v", err)
	} else {
		log.Println("Job completed successfully.")
	}
}

func fetchAndStoreVacancies(startDate, endDate string, collection *mongo.Collection, bearerToken string) error {
	page := 0
	var totalPages int

	for {
		// Construct the search URL with Bearer Token
		searchURL := fmt.Sprintf("%s?area=%s&professional_role=%s&date_from=%s&date_to=%s&per_page=%d&page=%d",
			BaseSearchURL, Area, ProfessionalRole, startDate, endDate, PerPage, page)

		// Create HTTP request with Bearer token
		req, err := http.NewRequest("GET", searchURL, nil)
		if err != nil {
			errorLogger.Printf("Failed to create request for page %d: %v", page, err)
			// Proceed to next step or handle accordingly
		}

		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", bearerToken))

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			errorLogger.Printf("Failed to fetch search page %d: %v", page, err)
			// Proceed to next step or handle accordingly
		}

		if resp != nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				body, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					errorLogger.Printf("Failed to read response body for page %d: %v", page, err)
					// Proceed to next step
				}

				var searchResp VacancySearchResponse
				err = json.Unmarshal(body, &searchResp)
				if err != nil {
					errorLogger.Printf("Failed to parse JSON for page %d: %v", page, err)
					// Proceed to next step
				}

				// On the first page, set totalPages
				if page == 0 {
					totalPages = searchResp.Pages
					log.Printf("Total pages to fetch: %d", totalPages)
				}

				// Extract vacancy IDs
				var vacancyIDs []string
				for _, item := range searchResp.Items {
					vacancyIDs = append(vacancyIDs, item.ID)
				}

				// Fetch vacancy details asynchronously
				err = fetchVacancyDetails(vacancyIDs, collection, bearerToken)
				if err != nil {
					errorLogger.Printf("Failed to fetch vacancy details on page %d: %v", page, err)
				}

				// Check if we've fetched all pages
				if page >= totalPages-1 {
					break
				}
				page++
			} else {
				// Log errors 400, 403, 404
				if resp.StatusCode == http.StatusBadRequest ||
					resp.StatusCode == http.StatusForbidden ||
					resp.StatusCode == http.StatusNotFound {
					errorLogger.Printf("Error fetching page %d: HTTP %d", page, resp.StatusCode)
					// Proceed to next step
					if page >= totalPages-1 {
						break
					}
					page++
				} else {
					errorLogger.Printf("Unexpected HTTP status %d on page %d", resp.StatusCode, page)
					// You might decide to retry or skip
					if page >= totalPages-1 {
						break
					}
					page++
				}
			}
		} else {
			// If response is nil due to an error during GET
			errorLogger.Printf("Received nil response for page %d", page)
			if page >= totalPages-1 {
				break
			}
			page++
		}
	}

	return nil
}

func fetchVacancyDetails(vacancyIDs []string, collection *mongo.Collection, bearerToken string) error {
	var wg sync.WaitGroup
	sem := make(chan struct{}, MaxConcurrency)

	for _, id := range vacancyIDs {
		wg.Add(1)
		go func(vacancyID string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			retries := 0

			for {
				err := processVacancy(vacancyID, collection, bearerToken)
				if err != nil {
					if errors.Is(err, ErrRetryable) {
						if retries < MaxRetries {
							retries++
							errorLogger.Printf("Retryable error for vacancy %s. Attempt %d. Error: %v", vacancyID, retries, err)
							time.Sleep(RetryDelay)
							continue
						} else {
							errorLogger.Printf("Max retries reached for vacancy %s. Skipping.", vacancyID)
						}
					} else if errors.Is(err, ErrNonRetryable) {
						// Non-retryable error, already logged
					} else {
						// Unknown error
						errorLogger.Printf("Unknown error for vacancy %s: %v", vacancyID, err)
					}
				}
				break
			}
		}(id)
	}

	wg.Wait()
	return nil
}

var (
	ErrRetryable    = errors.New("retryable error")
	ErrNonRetryable = errors.New("non-retryable error")
)

func processVacancy(vacancyID string, collection *mongo.Collection, bearerToken string) error {
	vacancyURL := BaseVacancyURL + vacancyID

	// Create HTTP request with Bearer token
	req, err := http.NewRequest("GET", vacancyURL, nil)
	if err != nil {
		errorLogger.Printf("Failed to create request for vacancy %s: %v", vacancyID, err)
		return ErrRetryable
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", bearerToken))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		errorLogger.Printf("HTTP GET error for vacancy %s: %v", vacancyID, err)
		return ErrRetryable
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			errorLogger.Printf("Failed to read vacancy %s response body: %v", vacancyID, err)
			return ErrRetryable
		}

		// Optional: Validate JSON structure if needed

		// Insert into MongoDB
		var vacancyData bson.M
		err = json.Unmarshal(body, &vacancyData)
		if err != nil {
			errorLogger.Printf("Failed to parse JSON for vacancy %s: %v", vacancyID, err)
			return ErrNonRetryable
		}

		// Insert with upsert to avoid duplicates
		filter := bson.M{"id": vacancyData["id"]}
		update := bson.M{"$set": vacancyData}
		opts := options.Update().SetUpsert(true)

		_, err = collection.UpdateOne(context.TODO(), filter, update, opts)
		if err != nil {
			errorLogger.Printf("MongoDB insertion error for vacancy %s: %v", vacancyID, err)
			return ErrRetryable
		}

		successLogger.Printf("Vacancy %s stored successfully.", vacancyID)
		return nil
	case http.StatusNotFound:
		errorLogger.Printf("Vacancy %s not found (404). Skipping.", vacancyID)
		return ErrNonRetryable
	case http.StatusForbidden, http.StatusTooManyRequests:
		errorLogger.Printf("Received HTTP %d for vacancy %s. Will retry.", resp.StatusCode, vacancyID)
		return ErrRetryable
	default:
		errorLogger.Printf("Unexpected HTTP status %d for vacancy %s", resp.StatusCode, vacancyID)
		return ErrRetryable
	}
}
