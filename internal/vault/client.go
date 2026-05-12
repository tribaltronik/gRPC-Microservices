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

// DBRefreshFunc is a callback invoked when credentials are refreshed.
type DBRefreshFunc func(*DBCreds)

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

// StartCredentialRefresher runs a background goroutine that re-fetches
// dynamic database credentials at half the lease duration, calling
// refreshFn with each refreshed credential set. Stops when ctx is cancelled.
func (c *Client) StartCredentialRefresher(ctx context.Context, role string, refreshFn DBRefreshFunc, logFn func(string, ...interface{})) {
	go func() {
		for {
			creds, err := c.GetDBCreds(ctx, role)
			if err != nil {
				if logFn != nil {
					logFn("vault credential refresh failed", "error", err)
				}
				select {
				case <-ctx.Done():
					return
				case <-time.After(30 * time.Second):
				}
				continue
			}

			if refreshFn != nil {
				refreshFn(creds)
			}

			interval := creds.LeaseDuration / 2
			if interval < 30*time.Second {
				interval = 30 * time.Second
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(interval):
			}
		}
	}()
}
