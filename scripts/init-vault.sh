#!/bin/bash
set -euo pipefail

VAULT_ADDR="${VAULT_ADDR:-http://localhost:8200}"
VAULT_TOKEN="${VAULT_TOKEN:-root}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="${PROJECT_DIR:-$(cd "$SCRIPT_DIR/.." && pwd)}"

echo "=== Initializing Vault ==="
echo "Vault Address: $VAULT_ADDR"

# Wait for Vault to be ready
echo "[1/8] Waiting for Vault..."
for i in $(seq 1 30); do
  if curl -sf "$VAULT_ADDR/v1/sys/health" > /dev/null 2>&1; then
    echo "  Vault is ready"
    break
  fi
  if [ "$i" -eq 30 ]; then
    echo "  ERROR: Vault not ready after 30s"
    exit 1
  fi
  sleep 1
done

alias vault-cmd="vault"

# Login
echo "[2/8] Logging in..."
vault login -address="$VAULT_ADDR" "$VAULT_TOKEN" > /dev/null

# Enable database secret engine
echo "[3/8] Enabling database secret engine..."
vault secrets enable -address="$VAULT_ADDR" -path=database database 2>/dev/null || echo "  database engine already enabled"

# Configure PostgreSQL connections
echo "[4/8] Configuring database connections..."

# User DB connection
vault write -address="$VAULT_ADDR" database/config/user-db \
  plugin_name=postgresql-database-plugin \
  allowed_roles="user-service" \
  connection_url="postgresql://{{username}}:{{password}}@user-db:5432/users?sslmode=disable" \
  username="postgres" \
  password="postgres" 2>/dev/null || echo "  user-db connection already configured"

# Order DB connection
vault write -address="$VAULT_ADDR" database/config/order-db \
  plugin_name=postgresql-database-plugin \
  allowed_roles="order-service" \
  connection_url="postgresql://{{username}}:{{password}}@order-db:5432/orders?sslmode=disable" \
  username="postgres" \
  password="postgres" 2>/dev/null || echo "  order-db connection already configured"

# Create database roles for dynamic credentials
echo "[5/8] Creating database roles..."

vault write -address="$VAULT_ADDR" database/roles/user-service \
  db_name=user-db \
  creation_statements="CREATE USER \"{{name}}\" WITH PASSWORD '{{password}}' VALID UNTIL '{{expiration}}'; GRANT USAGE ON SCHEMA public TO \"{{name}}\"; GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO \"{{name}}\";" \
  revocation_statements="DROP USER IF EXISTS \"{{name}}\";" \
  default_ttl=1h \
  max_ttl=24h 2>/dev/null || echo "  user-service role already exists"

vault write -address="$VAULT_ADDR" database/roles/order-service \
  db_name=order-db \
  creation_statements="CREATE USER \"{{name}}\" WITH PASSWORD '{{password}}' VALID UNTIL '{{expiration}}'; GRANT USAGE ON SCHEMA public TO \"{{name}}\"; GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO \"{{name}}\";" \
  revocation_statements="DROP USER IF EXISTS \"{{name}}\";" \
  default_ttl=1h \
  max_ttl=24h 2>/dev/null || echo "  order-service role already exists"

# Store static secrets
echo "[6/8] Storing static secrets..."

vault kv put -address="$VAULT_ADDR" secret/user-service/db \
  url="postgres://postgres:postgres@user-db:5432/users?sslmode=disable" 2>/dev/null || true

vault kv put -address="$VAULT_ADDR" secret/order-service/db \
  url="postgres://postgres:postgres@order-db:5432/orders?sslmode=disable" 2>/dev/null || true

vault kv put -address="$VAULT_ADDR" secret/api-gateway/api-key \
  key="grpc-poc-api-key-2026" 2>/dev/null || true

# Create policies
echo "[7/8] Creating policies..."

vault policy write -address="$VAULT_ADDR" user-service "$PROJECT_DIR/vault/policies/user-service.hcl" 2>/dev/null || echo "  user-service policy already exists"
vault policy write -address="$VAULT_ADDR" order-service "$PROJECT_DIR/vault/policies/order-service.hcl" 2>/dev/null || echo "  order-service policy already exists"

# Create service tokens
echo "[8/8] Creating service tokens..."

TOKENS_DIR="$PROJECT_DIR/vault/tokens"
mkdir -p "$TOKENS_DIR"

vault token create -address="$VAULT_ADDR" -policy=user-service -period=168h -format=json 2>/dev/null \
  | jq -r '.auth.client_token' > "$TOKENS_DIR/user-service.token" || echo "  using existing token"

vault token create -address="$VAULT_ADDR" -policy=order-service -period=168h -format=json 2>/dev/null \
  | jq -r '.auth.client_token' > "$TOKENS_DIR/order-service.token" || echo "  using existing token"

chmod 600 "$TOKENS_DIR"/*.token 2>/dev/null || true

echo ""
echo "=== Vault initialization complete ==="
echo ""
echo "Service tokens saved to:"
echo "  vault/tokens/user-service.token"
echo "  vault/tokens/order-service.token"
echo ""
echo "Dynamic DB credentials (example):"
echo "  vault read database/creds/user-service"
echo "  vault read database/creds/order-service"
echo ""
echo "Static secrets:"
echo "  vault kv get secret/user-service/db"
echo "  vault kv get secret/order-service/db"
echo "  vault kv get secret/api-gateway/api-key"
