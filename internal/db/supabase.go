package db

import (
	"encoding/json"
	"fmt"
	"greenvue/lib"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
)

// Global client instance and mutex for thread safety
var (
	globalClient     *SupabaseClient
	globalClientOnce sync.Once
	globalClientMu   sync.RWMutex
)

type SupabaseClient struct {
	URL    string
	APIKey string
	Client *resty.Client
}

// InitGlobalClient initializes the global Supabase client if it doesn't exist yet
func InitGlobalClient(useServiceKey ...bool) (*SupabaseClient, error) {
	globalClientOnce.Do(func() {
		globalClient = NewSupabaseClient(useServiceKey...)
	})

	if globalClient == nil {
		return nil, fmt.Errorf("failed to initialize global Supabase client")
	}

	return globalClient, nil
}

// GetGlobalClient returns the global Supabase client instance
// If it hasn't been initialized yet, it creates a new instance
func GetGlobalClient() *SupabaseClient {
	globalClientMu.RLock()
	if globalClient != nil {
		defer globalClientMu.RUnlock()
		return globalClient
	}
	globalClientMu.RUnlock()

	// If client doesn't exist, initialize it
	globalClientMu.Lock()
	defer globalClientMu.Unlock()

	if globalClient == nil {
		globalClient = NewSupabaseClient()
	}

	return globalClient
}

// NewSupabaseClient creates a new Supabase client using environment variables
func NewSupabaseClient(useServiceKey ...bool) *SupabaseClient {
	url := os.Getenv("SUPABASE_URL")

	var apiKey string
	// Check if we should use the service key or the anon key
	if len(useServiceKey) > 0 && useServiceKey[0] {
		apiKey = os.Getenv("SUPABASE_SERVICE_KEY")
	} else {
		// Default to using the anon key
		apiKey = os.Getenv("SUPABASE_ANON")
	}

	// Validate that the required environment variables are set
	if url == "" || apiKey == "" {
		fmt.Println("ERROR: Supabase environment variables not set. SUPABASE_URL and SUPABASE_ANON or SUPABASE_SERVICE_KEY are required.")
		return nil
	}

	client := resty.New().
		SetBaseURL(url).
		SetTimeout(10*time.Second).
		SetHeader("Content-Type", "application/json").
		SetHeader("Accept", "application/json").
		SetHeader("apikey", apiKey).
		SetHeader("Authorization", "Bearer "+apiKey).
		SetHeader("Prefer", "return=representation")

	return &SupabaseClient{
		URL:    url,
		APIKey: apiKey,
		Client: client,
	}
}

// GET performs a GET request to fetch data with optional query parameters
func (s *SupabaseClient) GET(table, query string) ([]byte, error) {
	url := fmt.Sprintf("%s/rest/v1/%s?%s", s.URL, table, query)

	resp, err := s.Client.R().Get(url)
	if err != nil {
		return nil, err
	}

	body := resp.Body()

	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		return nil, fmt.Errorf("supabase error: status %d - %s", resp.StatusCode(), string(body))
	}

	return body, nil
}

// POST creates a new record
func (s *SupabaseClient) POST(table string, data any) ([]byte, error) {
	url := fmt.Sprintf("%s/rest/v1/%s?select=*", s.URL, table)

	resp, err := s.Client.R().
		SetBody(data).
		Post(url)

	if err != nil {
		fmt.Println("Error sending request:", err)
		return nil, err
	}

	body := resp.Body()

	// Empty response is valid in some cases
	if len(body) == 0 {
		return []byte("{}"), nil
	}

	if resp.StatusCode() != http.StatusCreated {
		return nil, fmt.Errorf("supabase error: %s", string(body))
	}

	return body, nil
}

// PATCH updates an existing record by ID
func (s *SupabaseClient) PATCH(table string, id uuid.UUID, data any) ([]byte, error) {
	url := fmt.Sprintf("%s/rest/v1/%s?id=eq.%s", s.URL, table, id)

	resp, err := s.Client.R().
		SetBody(data).
		Patch(url)

	if err != nil {
		return nil, err
	}

	respBody := resp.Body()

	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		return nil, fmt.Errorf("supabase PATCH error (%d): %s", resp.StatusCode(), string(respBody))
	}

	return respBody, nil
}

// DELETE removes a record based on condition
func (s *SupabaseClient) DELETE(table, conditions string) ([]byte, error) {
	url := fmt.Sprintf("%s/rest/v1/%s?%s", s.URL, table, conditions)

	resp, err := s.Client.R().Delete(url)
	if err != nil {
		return nil, fmt.Errorf("failed to execute DELETE request: %w", err)
	}

	respBody := resp.Body()

	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		return nil, fmt.Errorf("DELETE operation failed (status %d): %s", resp.StatusCode(), string(respBody))
	}

	return respBody, nil
}

// UploadImage uploads an image to Supabase storage
func (s *SupabaseClient) UploadImage(filename, bucket string, image []byte) ([]byte, error) {
	url := fmt.Sprintf("%s/storage/v1/object/%s/%s", s.URL, bucket, filename)
	fmt.Printf("Uploading to URL: %s\n", url)
	contentType := "image/jpeg"
	if strings.HasSuffix(filename, ".png") {
		contentType = "image/png"
	} else if strings.HasSuffix(filename, ".webp") {
		contentType = "image/webp"
	}

	fmt.Printf("Using content type: %s\n", contentType)

	resp, err := s.Client.R().
		SetBody(image).
		Post(url)

	if err != nil {
		fmt.Printf("Error sending request: %v\n", err)
		return nil, err
	}

	body := resp.Body()

	fmt.Printf("Status code: %d\n", resp.StatusCode())
	if resp.StatusCode() >= 400 {
		fmt.Printf("Error response: %s\n", string(body))
		return nil, fmt.Errorf("supabase storage error (%d): %s", resp.StatusCode(), string(body))
	}

	return body, nil
}

// SignUp registers a new user
func (s *SupabaseClient) SignUp(email, password string) (*lib.User, error) {
	url := fmt.Sprintf("%s/auth/v1/signup", s.URL)

	// Create request payload
	payload := map[string]string{
		"email":    email,
		"password": password,
	}

	resp, err := s.Client.R().
		SetBody(payload).
		Post(url)

	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	body := resp.Body()

	// Check for HTTP errors
	if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusCreated {
		return nil, fmt.Errorf("failed to sign up user: %s", string(body))
	}

	// Parse JSON response based on actual Supabase structure
	var userResp struct {
		ID    uuid.UUID `json:"id"`
		Email string    `json:"email"`
	}

	if err := json.Unmarshal(body, &userResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Check if user ID exists
	if userResp.ID == (uuid.UUID{}) {
		return nil, fmt.Errorf("user ID missing in response")
	}

	// Create the user object
	user := &lib.User{
		ID:    userResp.ID,
		Email: userResp.Email,
	}

	return user, nil
}

// Login authenticates a user
func (s *SupabaseClient) Login(email, password string) (*lib.AuthResponse, error) {
	url := fmt.Sprintf("%s/auth/v1/token?grant_type=password", s.URL)

	// Create request payload
	payload := map[string]string{
		"email":    email,
		"password": password,
	}

	resp, err := s.Client.R().
		SetBody(payload).
		Post(url)

	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	body := resp.Body()

	if strings.Contains(string(body), "error_code") {
		// Parse the error response
		var errorResp struct {
			ErrorCode string `json:"error_code"`
			Code      int    `json:"code"`
			Message   string `json:"msg"`
		}
		if err := json.Unmarshal(body, &errorResp); err != nil {
			return nil, fmt.Errorf("failed to parse error response: %w", err)
		}

		switch errorResp.ErrorCode {
		case "invalid_credentials":
			return nil, fmt.Errorf("invalid_credentials")
		case "email_not_confirmed":
			return nil, fmt.Errorf("email_not_confirmed")
		case "user_not_found":
			return nil, fmt.Errorf("user_not_found")
		default:
			return nil, fmt.Errorf("login_failed: %s", errorResp.Message)
		}
	}

	// Check for HTTP errors
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("login_failed: %s", string(body))
	}

	// Parse JSON response
	var authResp lib.AuthResponse
	if err := json.Unmarshal(body, &authResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &authResp, nil
}

func (s *SupabaseClient) UpdateUser(id uuid.UUID, data map[string]any) (*lib.User, error) {
	url := fmt.Sprintf("%s/auth/v1/admin/users/%s", s.URL, id)

	resp, err := s.Client.R().
		SetBody(data).
		Put(url)

	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	body := resp.Body()

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("update user failed: %s", string(body))
	}

	var user lib.User
	if err := json.Unmarshal(body, &user); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &user, nil
}
