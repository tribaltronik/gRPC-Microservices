package vault

import (
	"context"
	"fmt"
	"time"

	vapi "github.com/hashicorp/vault/api"
)

// DBCreds holds dynamic database credentials from Vault.
type DBCreds struct {
	Username       string
	Password       string
	LeaseID        string
	LeaseDuration  time.Duration
}

// Client wraps the HashiCorp Vault API client.
type Client struct {
	client *vapi.Client
}

// New creates a new Vault client with token authentication.
func New(address, token string) (*Client, error) {
	config := vapi.DefaultConfig()
	config.Address = address

	client, err := vapi.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("create vault client: %w", err)
	}
	client.SetToken(token)

	return &Client{client: client}, nil
}

// GetDBCreds fetches dynamic database credentials from the database secret engine.
func (c *Client) GetDBCreds(ctx context.Context, role string) (*DBCreds, error) {
	secret, err := c.client.Logical().ReadWithContext(ctx, "database/creds/"+role)
	if err != nil {
		return nil, fmt.Errorf("read db creds for role %s: %w", role, err)
	}
	if secret == nil || secret.Data == nil {
		return nil, fmt.Errorf("empty response for role %s", role)
	}

	data, ok := secret.Data["data"].(map[string]interface{})
	if !ok {
		// KV v1 returns data at top level
		data = secret.Data
	}

	username, _ := data["username"].(string)
	password, _ := data["password"].(string)

	if username == "" || password == "" {
		return nil, fmt.Errorf("missing username or password in vault response")
	}

	leaseDuration := time.Duration(secret.LeaseDuration) * time.Second

	return &DBCreds{
		Username:      username,
		Password:      password,
		LeaseID:       secret.LeaseID,
		LeaseDuration: leaseDuration,
	}, nil
}

// GetSecret reads a static secret from the KV v1 secret engine.
func (c *Client) GetSecret(ctx context.Context, path string) (map[string]interface{}, error) {
	secret, err := c.client.Logical().ReadWithContext(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("read secret %s: %w", path, err)
	}
	if secret == nil || secret.Data == nil {
		return nil, fmt.Errorf("secret %s not found", path)
	}

	return secret.Data, nil
}

// RenewLease renews a lease for a dynamic secret.
func (c *Client) RenewLease(ctx context.Context, leaseID string, increment int) error {
	_, err := c.client.Sys().RenewWithContext(ctx, leaseID, increment)
	if err != nil {
		return fmt.Errorf("renew lease %s: %w", leaseID, err)
	}
	return nil
}
