# Allow reading dynamic database credentials
path "database/creds/order-service" {
  capabilities = ["read"]
}

# Allow reading static secrets
path "secret/order-service/*" {
  capabilities = ["read", "list"]
}
