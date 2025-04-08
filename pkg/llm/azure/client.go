package azure

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type Client struct {
	apiKey          string
	azureEndpoint   string
	azureDeployment string
	apiVersion      string
	client          *http.Client
}

func NewClient(apiKey string, azureEndpoint string, azureDeployment string, apiVersion string) *Client {
	return &Client{
		apiKey:          apiKey,
		azureEndpoint:   azureEndpoint,
		azureDeployment: azureDeployment,
		apiVersion:      apiVersion,
		client:          &http.Client{},
	}
}

func (c *Client) CreateChatCompletion(ctx context.Context, req CreateRequest) (*APIResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx,
		"POST",
		fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=%s", c.azureEndpoint, c.azureDeployment, c.apiVersion),
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("api-key", c.apiKey)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error struct {
				Message string `json:"message"`
				Type    string `json:"type"`
				Code    string `json:"code"`
			} `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
			return nil, fmt.Errorf("error response with status %d", resp.StatusCode)
		}

		return nil, fmt.Errorf("%s: %s", errResp.Error.Type, errResp.Error.Message)
	}

	var response APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	return &response, nil
}
