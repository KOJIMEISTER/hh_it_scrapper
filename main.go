package main

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
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
	// Raw JSON for flexibility
	RawJSON json.RawMessage `json:"-"`
}

// SafeMap is a thread-safe map for string keys and empty struct values
type SafeMap struct {
	sync.RWMutex
	internal map[string]struct{}
}

// NewSafeMap initializes a new SafeMap
func NewSafeMap() *SafeMap {
	return &SafeMap{
		internal: make(map[string]struct{}),
	}
}

// Exists checks if a key exists in the SafeMap
func (sm *SafeMap) Exists(key string) bool {
	sm.RLock()
	defer sm.RUnlock()
	_, exists := sm.internal[key]
	return exists
}

// Add inserts a key into the SafeMap
func (sm *SafeMap) Add(key string) {
	sm.Lock()
	defer sm.Unlock()
	sm.internal[key] = struct{}{}
}

// Variables for loggers
var (
	successLogger *log.Logger
	errorLogger   *log.Logger
)

func init() {
	// Initialize loggers
	err := os.MkdirAll("logs", os.ModePerm)
	if err != nil {
		log.Fatalf("Failed to create logs directory: %v", err)
	}

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
	from := flag.String("from", "", "Start date in YYYY-MM-DD format (required)")
	to := flag.String("to", "", "End date in YYYY-MM-DD format (required)")
	flag.Parse()

	// Validate date arguments
	if *from == "" || *to == "" {
		errorLogger.Println("Both --from and --to date arguments must be provided.")
		fmt.Println("Error: Both --from and --to date arguments must be provided in YYYY-MM-DD format.")
		os.Exit(1)
	}

	startDate := *from
	endDate := *to

	mongoURI := os.Getenv("MONGO_URI")
	bearerToken := os.Getenv("BEARER_TOKEN")

	if bearerToken == "" {
		errorLogger.Println("BEARER_TOKEN must be provided")
		log.Fatal("BEARER_TOKEN must be provided")
	}

	if mongoURI == "" {
		errorLogger.Println("MONGO_URI must be provided")
		log.Fatal("MONGO_URI must be provided")
	}

	// Create a MongoDB client
	clientOptions := options.Client().ApplyURI(mongoURI)
	client, err := mongo.Connect(context.TODO(), clientOptions)
	if err != nil {
		errorLogger.Fatalf("MongoDB connection error: %v", err)
	}
	defer func() {
		if err = client.Disconnect(context.TODO()); err != nil {
			errorLogger.Fatalf("MongoDB disconnection error: %v", err)
		}
	}()

	collection := client.Database("vacancy_db").Collection("vacancies")

	// Load existing vacancy IDs and description_hashes
	existingVacancyIDs, existingDescriptionHashes, err := loadExistingData(collection)
	if err != nil {
		errorLogger.Fatalf("Failed to load existing data: %v", err)
	}

	// Initialize counters
	var savedCount int64

	// Run the job
	startTime := time.Now()
	log.Println("Job started...")
	err = fetchAndStoreVacancies(startDate, endDate, collection, bearerToken, existingVacancyIDs, existingDescriptionHashes, &savedCount)
	if err != nil {
		errorLogger.Printf("Job failed: %v", err)
	} else {
		log.Println("Job completed successfully.")
	}
	duration := time.Since(startTime)
	log.Printf("Duration: %v", duration)

	// Display the number of successfully saved vacancies
	fmt.Printf("Number of successfully saved vacancies: %d\n", savedCount)
}

func loadExistingData(collection *mongo.Collection) (map[string]struct{}, *SafeMap, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Initialize maps
	vacancyIDs := make(map[string]struct{})
	descriptionHashes := NewSafeMap()

	// Create a cursor to iterate over all documents
	cursor, err := collection.Find(ctx, bson.D{}, options.Find().SetProjection(bson.D{
		{"id", 1},
		{"description_hash", 1},
	}))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch existing vacancies: %v", err)
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var doc struct {
			ID              string `bson:"id"`
			DescriptionHash string `bson:"description_hash"`
		}
		if err := cursor.Decode(&doc); err != nil {
			return nil, nil, fmt.Errorf("failed to decode document: %v", err)
		}
		vacancyIDs[doc.ID] = struct{}{}
		if doc.DescriptionHash != "" {
			descriptionHashes.Add(doc.DescriptionHash)
		}
	}

	if err := cursor.Err(); err != nil {
		return nil, nil, fmt.Errorf("cursor error: %v", err)
	}

	log.Printf("Loaded %d existing vacancy IDs and %d description hashes from MongoDB.", len(vacancyIDs), len(descriptionHashes.internal))
	return vacancyIDs, descriptionHashes, nil
}

func fetchAndStoreVacancies(startDate, endDate string, collection *mongo.Collection, bearerToken string, existingVacancyIDs map[string]struct{}, existingDescriptionHashes *SafeMap, savedCount *int64) error {
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
			// Proceed to next page
			page++
			continue
		}

		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", bearerToken))

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			errorLogger.Printf("Failed to fetch search page %d: %v", page, err)
			// Proceed to next page
			page++
			continue
		}

		if resp != nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				body, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					errorLogger.Printf("Failed to read response body for page %d: %v", page, err)
					// Proceed to next page
					page++
					continue
				}

				var searchResp VacancySearchResponse
				err = json.Unmarshal(body, &searchResp)
				if err != nil {
					errorLogger.Printf("Failed to parse JSON for page %d: %v", page, err)
					// Proceed to next page
					page++
					continue
				}

				// On the first page, set totalPages
				if page == 0 {
					totalPages = searchResp.Pages
					log.Printf("Total pages to fetch: %d", totalPages)
				}

				// Extract vacancy IDs, excluding those already in DB
				var vacancyIDs []string
				for _, item := range searchResp.Items {
					if _, exists := existingVacancyIDs[item.ID]; !exists {
						vacancyIDs = append(vacancyIDs, item.ID)
					}
				}

				log.Printf("Processing page %d: %d new vacancies found.", page, len(vacancyIDs))

				// Fetch vacancy details asynchronously
				err = fetchVacancyDetails(vacancyIDs, collection, bearerToken, existingDescriptionHashes, savedCount)
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
					// Proceed to next page
					if page >= totalPages-1 {
						break
					}
					page++
				} else {
					errorLogger.Printf("Unexpected HTTP status %d on page %d", resp.StatusCode, page)
					// Proceed to next page
					if page >= totalPages-1 {
						break
					}
					page++
				}
			}
		} else {
			// If response is nil due to an error during GET
			errorLogger.Printf("Received nil response for page %d", page)
			// Proceed to next page
			page++
		}
	}

	return nil
}

func fetchVacancyDetails(vacancyIDs []string, collection *mongo.Collection, bearerToken string, existingDescriptionHashes *SafeMap, savedCount *int64) error {
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
				err := processVacancy(vacancyID, collection, bearerToken, existingDescriptionHashes)
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
				} else {
					// Increment the saved count atomically
					atomic.AddInt64(savedCount, 1)
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

func processVacancy(vacancyID string, collection *mongo.Collection, bearerToken string, existingDescriptionHashes *SafeMap) error {
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

		// Parse JSON into a map
		var vacancyData map[string]interface{}
		err = json.Unmarshal(body, &vacancyData)
		if err != nil {
			errorLogger.Printf("Failed to parse JSON for vacancy %s: %v", vacancyID, err)
			return ErrNonRetryable
		}

		// Compute MD5 hash of the description field
		description, ok := vacancyData["description"].(string)
		if !ok {
			errorLogger.Printf("Vacancy %s does not contain a valid 'description' field.", vacancyID)
			return ErrNonRetryable
		}
		descriptionHash := md5Hash(description)

		// Check if description_hash already exists
		if existingDescriptionHashes.Exists(descriptionHash) {
			// Duplicate description, skip saving
			successLogger.Printf("Vacancy %s skipped due to duplicate description_hash.", vacancyID)
			return nil
		}

		// Add description_hash to the vacancy data
		vacancyData["description_hash"] = descriptionHash

		// Insert into MongoDB
		filter := bson.M{"id": vacancyData["id"]}
		update := bson.M{"$set": vacancyData}
		opts := options.Update().SetUpsert(true)

		_, err = collection.UpdateOne(context.TODO(), filter, update, opts)
		if err != nil {
			errorLogger.Printf("MongoDB insertion error for vacancy %s: %v", vacancyID, err)
			return ErrRetryable
		}

		// Update the in-memory hash map to include the new hash
		existingDescriptionHashes.Add(descriptionHash)

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

func md5Hash(text string) string {
	hash := md5.Sum([]byte(text))
	return hex.EncodeToString(hash[:])
}
