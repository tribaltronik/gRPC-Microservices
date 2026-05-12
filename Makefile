.PHONY: up down verify verify-online test-resilience logs clean certs ps stop

# Deploy full stack
up: certs
	@echo "=== Generating certificates ==="
	@certs/generate-certs.sh
	@echo "=== Starting stack ==="
	@docker compose up -d --build
	@echo "=== Waiting for services ==="
	@scripts/wait-for-health.sh
	@echo ""
	@echo "Stack is ready!"
	@echo "  Envoy HTTP gateway: http://localhost:8080"
	@echo "  Envoy admin:        http://localhost:9901"
	@echo "  Vault UI:           http://localhost:8200"
	@echo ""
	@echo "Quick test:"
	@echo '  curl -H "x-api-key: grpc-poc-api-key-2026" http://localhost:8080/api/v1/users/00000000-0000-0000-0000-000000000000'
	@echo ""
	@echo "Run 'make verify' for full verification"

# Run static verification (no Docker needed)
verify:
	@scripts/verify.sh

# Run full verification including runtime checks
verify-online:
	@scripts/verify.sh --online

# Run resilience test suite (requires running stack)
test-resilience:
	@scripts/test-resilience.sh

# Generate certificates only
certs:
	@certs/generate-certs.sh

# View logs
logs:
	@docker compose logs -f

# Follow logs for a specific service
logs-%:
	@docker compose logs -f $*

# Stop stack and clean up
down:
	@docker compose down -v

# Stop stack without deleting volumes
stop:
	@docker compose stop

# Show service status
ps:
	@docker compose ps

# Clean generated files (certs and vault tokens)
clean:
	@rm -f certs/**/*.pem certs/**/*.csr certs/ca/*.pem certs/ca/*.csr
	@rm -f vault/tokens/*.token 2>/dev/null || true
	@echo "Cleaned generated certificates and vault tokens"

# Open a shell in a running service
shell-%:
	@docker compose exec $* sh
