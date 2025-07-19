package api

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

const (
	BaseSearchURL  = "https://api.hh.ru/vacancies"
	BaseVacancyURL = "https://api.hh.ru/vacancies/"
)

type HHClient struct {
	BearerToken string
	HTTPClient  *http.Client
}

func NewHHClient(bearerToken string) *HHClient {
	return &HHClient{
		BearerToken: bearerToken,
		HTTPClient:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *HHClient) GetVacancyIDs(ctx context.Context, startDate, endDate, area, role string, page, perPage int) ([]string, int, error) {
	searchURL := fmt.Sprintf("%s?area=%s&professional_role=%s&date_from=%s&date_to=%s&per_page=%d&page=%d",
		BaseSearchURL, area, role, startDate, endDate, perPage, page)

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.BearerToken))

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read response body: %w", err)
	}

	var searchResp struct {
		Pages int `json:"pages"`
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return nil, 0, fmt.Errorf("failed to parse JSON: %w", err)
	}

	ids := make([]string, 0, len(searchResp.Items))
	for _, item := range searchResp.Items {
		ids = append(ids, item.ID)
	}

	return ids, searchResp.Pages, nil
}

func (c *HHClient) GetVacancyDetails(ctx context.Context, vacancyID string) (map[string]interface{}, error) {
	vacancyURL := BaseVacancyURL + vacancyID

	req, err := http.NewRequestWithContext(ctx, "GET", vacancyURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.BearerToken))

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response body: %w", err)
		}

		var data map[string]interface{}
		if err := json.Unmarshal(body, &data); err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}
		return data, nil

	case http.StatusNotFound:
		return nil, fmt.Errorf("vacancy not found: %w", ErrVacancyNotFound)
	case http.StatusForbidden, http.StatusTooManyRequests:
		return nil, fmt.Errorf("rate limited: %w", ErrRateLimited)
	default:
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
}

func MD5Hash(text string) string {
	hash := md5.Sum([]byte(text))
	return hex.EncodeToString(hash[:])
}

var (
	ErrVacancyNotFound = errors.New("vacancy not found")
	ErrRateLimited     = errors.New("rate limited")
)
