package secrets

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/hashicorp/vault/api"
)

type Client struct {
	vaultClient *api.Client
	keyName     string
}

const connectivityCheckPlaintext = "zerotrust-transit-connectivity-check"

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
	if addr != "" && token == "" {
		return nil, fmt.Errorf("BAO_TOKEN or VAULT_TOKEN is required when a secrets server address is configured")
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
	if resp == nil {
		return "", fmt.Errorf("unexpected empty response from OpenBao")
	}

	ciphertext, ok := resp.Data["ciphertext"].(string)
	if !ok {
		return "", fmt.Errorf("unexpected response from OpenBao: ciphertext not found")
	}

	return ciphertext, nil
}

// Check verifies that the configured transit key is reachable and that the
// token can both encrypt and decrypt.
func (c *Client) Check(ctx context.Context) error {
	ciphertext, err := c.EncryptData(ctx, connectivityCheckPlaintext)
	if err != nil {
		return fmt.Errorf("encrypt connectivity check: %w", err)
	}
	plaintext, err := c.DecryptData(ctx, ciphertext)
	if err != nil {
		return fmt.Errorf("decrypt connectivity check: %w", err)
	}
	if plaintext != connectivityCheckPlaintext {
		return fmt.Errorf("transit connectivity check returned unexpected plaintext")
	}
	return nil
}

func (c *Client) DecryptData(ctx context.Context, ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	// Existing installations may contain plaintext values from before transit
	// encryption was enabled. Only complete transit envelopes are sent to
	// OpenBao, so plaintext such as "vault:victor" remains readable.
	if !isTransitCiphertext(ciphertext) {
		return ciphertext, nil
	}

	path := fmt.Sprintf("transit/decrypt/%s", c.keyName)
	data := map[string]any{
		"ciphertext": ciphertext,
	}

	resp, err := c.vaultClient.Logical().WriteWithContext(ctx, path, data)
	if err != nil {
		return "", err
	}
	if resp == nil {
		return "", fmt.Errorf("unexpected empty response from OpenBao")
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

func isTransitCiphertext(value string) bool {
	const prefix = "vault:v"
	if !strings.HasPrefix(value, prefix) {
		return false
	}
	versionEnd := strings.IndexByte(value[len(prefix):], ':')
	if versionEnd <= 0 {
		return false
	}
	for _, r := range value[len(prefix) : len(prefix)+versionEnd] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
