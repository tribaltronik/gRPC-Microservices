.PHONY: up down verify verify-online test-resilience logs clean certs ps stop test-integration test-integration-auth test-integration-mtls test-integration-resilience load-test load-test-heavy load-test-chaos kind-up kind-down k8s-platform k8s-logs k8s-port-forward

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

# Run integration tests (requires running stack)
.PHONY: test-integration test-integration-auth test-integration-mtls test-integration-resilience

test-integration:
	@echo "=== Installing test dependencies ==="
	@pip install -q -r tests/requirements.txt
	@echo "=== Running integration tests ==="
	@python -m pytest tests/ -v --tb=short
	@echo "=== All tests passed ==="

test-integration-auth:
	@pip install -q -r tests/requirements.txt
	@python -m pytest tests/test_auth.py -v --tb=short

test-integration-mtls:
	@pip install -q -r tests/requirements.txt
	@python -m pytest tests/test_mtls.py -v --tb=short

test-integration-resilience:
	@pip install -q -r tests/requirements.txt
	@python -m pytest tests/test_resilience.py -v --tb=short

# Run load test (Go gRPC load tester)
.PHONY: load-test load-test-heavy load-test-chaos

load-test:
	@echo "=== Building load tester ==="
	@go build -o /tmp/loadtest ./scripts/loadtest/
	@echo "=== Running load test ==="
	@/tmp/loadtest --vus=25 --duration=30s

load-test-heavy:
	@echo "=== Building load tester ==="
	@go build -o /tmp/loadtest ./scripts/loadtest/
	@echo "=== Running heavy load test ==="
	@/tmp/loadtest --vus=50 --duration=30s

load-test-chaos:
	@echo "=== Running chaos load test ==="
	@scripts/load-test-chaos.sh

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
	@rm -rf istio-1.22.* 2>/dev/null || true
	@echo "Cleaned generated certificates and vault tokens"

# Open a shell in a running service
shell-%:
	@docker compose exec $* sh

# KIND cluster management
.PHONY: kind-up kind-down k8s-platform k8s-cert-manager k8s-logs k8s-port-forward

kind-up:
	@echo "=== Creating KIND cluster ==="
	@kind create cluster --config k8s/kind-config.yaml 2>&1
	@echo "=== Installing platform components ==="
	$(MAKE) k8s-platform

kind-down:
	@echo "=== Deleting KIND cluster ==="
	@kind delete cluster 2>&1

k8s-platform:
	@echo "=== Creating namespaces ==="
	@kubectl apply -f k8s/namespaces/init.yaml 2>&1
	@echo "=== Installing metrics-server ==="
	@kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml 2>&1
	@kubectl patch deployment metrics-server -n kube-system --type='json' -p='[{"op": "add", "path": "/spec/template/spec/containers/0/args/-", "value": "--kubelet-insecure-tls"}]' 2>&1
	@kubectl wait --for=condition=Available deployment/metrics-server -n kube-system --timeout=60s 2>&1
	@echo "=== Installing cert-manager ==="
	$(MAKE) k8s-cert-manager
	@echo "=== Platform ready (lighter: no Istio) ==="

k8s-cert-manager:
	@echo "=== Adding Helm repo ==="
	@helm repo add jetstack https://charts.jetstack.io 2>/dev/null || true
	@helm repo update 2>&1 | tail -1
	@echo "=== Installing cert-manager ==="
	@helm upgrade --install cert-manager jetstack/cert-manager \
		-n cert-manager --create-namespace \
		--set crds.enabled=true 2>&1 | tail -5
	@kubectl wait --for=condition=Available deployment/cert-manager -n cert-manager --timeout=120s 2>&1
	@kubectl wait --for=condition=Available deployment/cert-manager-webhook -n cert-manager --timeout=120s 2>&1
	@echo "=== Applying CA ClusterIssuer and Certificates ==="
	@kubectl apply -k k8s/platform/cert-manager/ 2>&1
	@sleep 5
	@kubectl get certificates -A 2>&1
	@echo "=== cert-manager ready ==="

k8s-istio:
	@echo "=== Labeling namespace ==="
	@kubectl label namespace grpc-services istio-injection=enabled --overwrite 2>&1
	@echo "=== Installing Istio (demo profile) ==="
	@istioctl install --set profile=demo \
		--set meshConfig.accessLogFile=/dev/stdout \
		--set meshConfig.enableTracing=true \
		--set values.gateways.istio-ingressgateway.type=NodePort \
		-y 2>&1
	@echo "=== Applying manifests ==="
	@kubectl apply -k k8s/platform/istio/ 2>&1
	@istioctl verify-install 2>&1 | tail -3
	@echo "=== Istio ready ==="

k8s-logs:
	@stern -n grpc-services . 2>/dev/null || echo "Install stern: brew install stern"

k8s-port-forward:
	@kubectl port-forward -n grpc-services svc/api-gateway 8080:80 &
	@kubectl port-forward -n monitoring svc/kube-prometheus-stack-grafana 3000:80 &
	@echo "Port forwards started (background)"


# Deploy services via Helm
.PHONY: k8s-deploy kind-load-images k8s-ingress k8s-port-forward-grpc

k8s-deploy:
	@echo "=== Loading Docker images into KIND ==="
	@for img in grpc-microservices-user-service grpc-microservices-order-service; do \
		kind load docker-image $${img}:latest; \
	done
	@echo "=== Deploying Helm chart ==="
	@helm upgrade --install grpc-services ./helm/grpc-services -n grpc-services \
		--set services.user-service.replicas=1 \
		--set services.order-service.replicas=1 2>&1
	@echo "=== Watching pods ==="
	@kubectl get pods -n grpc-services -w

kind-load-images:
	@for img in grpc-microservices-user-service grpc-microservices-order-service; do \
		kind load docker-image $${img}:latest; \
	done

# Install Nginx Ingress Controller for KIND
k8s-ingress:
	@echo "=== Installing Nginx Ingress Controller for KIND ==="
	@kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/kind/deploy.yaml 2>&1
	@kubectl wait --for=condition=Available deployment/ingress-nginx-controller -n ingress-nginx --timeout=120s 2>&1
	@echo "=== Applying gRPC Ingress ==="
	@kubectl apply -k k8s/platform/ingress/ 2>&1
	@echo "=== Nginx Ingress ready ==="
	@echo "  gRPC available at localhost:80"
	@echo "  user-service: /user.v1.UserService/*"
	@echo "  order-service: /order.v1.OrderService/*"

# Test gRPC via port-forward (reliable alternative to nginx)
k8s-port-forward-grpc:
	@echo "=== Forwarding gRPC services to localhost ==="
	@echo "  user-service  -> localhost:50051"
	@echo "  order-service -> localhost:50052"
	@kubectl port-forward svc/user-service -n grpc-services 50051:50051 &
	@kubectl port-forward svc/order-service -n grpc-services 50052:50052 &
	@sleep 2
	@echo "=== Testing user-service health ==="
	@$(HOME)/go/bin/grpcurl -insecure -cacert certs/ca/ca.pem -cert certs/api-gateway/cert.pem -key certs/api-gateway/key.pem localhost:50051 grpc.health.v1.Health/Check 2>&1
	@sleep 1
	@echo "=== Creating test user ==="
	@$(HOME)/go/bin/grpcurl -insecure -cacert certs/ca/ca.pem -cert certs/api-gateway/cert.pem -key certs/api-gateway/key.pem -d '{"name":"TestUser","email":"test@example.com"}' localhost:50051 user.v1.UserService/CreateUser 2>&1
