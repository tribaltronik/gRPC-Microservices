# Allow reading dynamic database credentials
path "database/creds/user-service" {
  capabilities = ["read"]
}

# Allow reading static secrets
path "secret/user-service/*" {
  capabilities = ["read", "list"]
}
