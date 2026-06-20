package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	token      string
	baseURL    string
	httpClient *http.Client
}

func NewClient(token string) *Client {
	return &Client{
		token:      token,
		baseURL:    "https://api.telegram.org/bot" + token,
		httpClient: &http.Client{Timeout: 35 * time.Second},
	}
}

func (c *Client) GetUpdates(ctx context.Context, offset int, timeout int) ([]Update, error) {
	values := url.Values{}
	if offset > 0 {
		values.Set("offset", fmt.Sprint(offset))
	}
	values.Set("timeout", fmt.Sprint(timeout))
	values.Set("allowed_updates", `["message"]`)

	endpoint := c.baseURL + "/getUpdates?" + values.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	var response struct {
		OK          bool     `json:"ok"`
		Result      []Update `json:"result"`
		Description string   `json:"description,omitempty"`
	}
	if err := c.do(req, &response); err != nil {
		return nil, err
	}
	if !response.OK {
		return nil, fmt.Errorf("telegram getUpdates failed: %s", response.Description)
	}
	return response.Result, nil
}

func (c *Client) SendMessage(ctx context.Context, chatID int64, text string, replyToMessageID int) (*Message, error) {
	payload := map[string]any{
		"chat_id":              chatID,
		"text":                 strings.TrimSpace(text),
		"disable_notification": false,
	}
	if replyToMessageID > 0 {
		payload["reply_to_message_id"] = replyToMessageID
		payload["allow_sending_without_reply"] = true
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	var response struct {
		OK          bool     `json:"ok"`
		Result      *Message `json:"result,omitempty"`
		Description string   `json:"description,omitempty"`
	}
	if err := c.do(req, &response); err != nil {
		return nil, err
	}
	if !response.OK {
		return nil, fmt.Errorf("telegram sendMessage failed: %s", response.Description)
	}
	return response.Result, nil
}

func (c *Client) GetMe(ctx context.Context) (*User, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/getMe", nil)
	if err != nil {
		return nil, err
	}

	var response struct {
		OK          bool  `json:"ok"`
		Result      *User `json:"result,omitempty"`
		Description string `json:"description,omitempty"`
	}
	if err := c.do(req, &response); err != nil {
		return nil, err
	}
	if !response.OK {
		return nil, fmt.Errorf("telegram getMe failed: %s", response.Description)
	}
	return response.Result, nil
}

func (c *Client) IsChatAdmin(ctx context.Context, chatID int64, userID int64) (bool, error) {
	endpoint := fmt.Sprintf("%s/getChatMember?chat_id=%d&user_id=%d", c.baseURL, chatID, userID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, err
	}

	var response ChatMemberResponse
	if err := c.do(req, &response); err != nil {
		return false, err
	}
	if !response.OK {
		return false, fmt.Errorf("telegram getChatMember failed: %s", response.Error)
	}
	return response.Result.Status == "creator" || response.Result.Status == "administrator", nil
}

func (c *Client) do(req *http.Request, target any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram http %d: %s", resp.StatusCode, string(data))
	}
	return json.Unmarshal(data, target)
}
