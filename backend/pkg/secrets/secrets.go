package secrets

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"

	"github.com/hashicorp/vault/api"
)

type Client struct {
	vaultClient *api.Client
	keyName     string
}

func NewClient(keyName string) (*Client, error) {
	config := api.DefaultConfig()

	// Read BAO_ADDR/BAO_TOKEN or fall back to VAULT_ADDR/VAULT_TOKEN
	addr := os.Getenv("BAO_ADDR")
	if addr == "" {
		addr = os.Getenv("VAULT_ADDR")
	}
	if addr != "" {
		config.Address = addr
	}

	token := os.Getenv("BAO_TOKEN")
	if token == "" {
		token = os.Getenv("VAULT_TOKEN")
	}

	client, err := api.NewClient(config)
	if err != nil {
		return nil, err
	}

	if token != "" {
		client.SetToken(token)
	}

	return &Client{
		vaultClient: client,
		keyName:     keyName,
	}, nil
}

func (c *Client) EncryptData(ctx context.Context, plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	b64Plaintext := base64.StdEncoding.EncodeToString([]byte(plaintext))

	path := fmt.Sprintf("transit/encrypt/%s", c.keyName)
	data := map[string]any{
		"plaintext": b64Plaintext,
	}

	resp, err := c.vaultClient.Logical().WriteWithContext(ctx, path, data)
	if err != nil {
		return "", err
	}

	ciphertext, ok := resp.Data["ciphertext"].(string)
	if !ok {
		return "", fmt.Errorf("unexpected response from OpenBao: ciphertext not found")
	}

	return ciphertext, nil
}

func (c *Client) DecryptData(ctx context.Context, ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}

	path := fmt.Sprintf("transit/decrypt/%s", c.keyName)
	data := map[string]any{
		"ciphertext": ciphertext,
	}

	resp, err := c.vaultClient.Logical().WriteWithContext(ctx, path, data)
	if err != nil {
		return "", err
	}

	b64Plaintext, ok := resp.Data["plaintext"].(string)
	if !ok {
		return "", fmt.Errorf("unexpected response from OpenBao: plaintext not found")
	}

	decoded, err := base64.StdEncoding.DecodeString(b64Plaintext)
	if err != nil {
		return "", err
	}

	return string(decoded), nil
}
