# Contributing

## Project Status
This is a Phase 1 PoC demonstrating production-pattern microservices. The focus is on correctness and security patterns, not feature completeness.

## Getting Started
1. Fork the repository
2. Follow the quick start in [README.md](README.md)
3. Run the integration tests: `make test-integration`

## Development Workflow

### Code Style
- Go code follows standard `gofmt` formatting
- Proto files follow [Buf's lint rules](https://buf.build/docs/lint-rules)
- Python tests follow PEP 8

### Making Changes
1. Create a feature branch
2. Make your changes
3. Run verification: `make verify`
4. Run integration tests: `make test-integration`
5. Run load tests (if performance-impacting): `make load-test`

## Phase 2 Ideas
See [docs/plan-phase2.md](docs/plan-phase2.md) for the full roadmap. Key areas for contribution:

- **Kubernetes migration**: Convert compose services to Helm charts
- **Service mesh**: Add Istio for advanced traffic management
- **CI/CD**: Add GitHub Actions for automated testing and deployment
- **Chaos engineering**: Integrate Litmus or Chaos Mesh
- **GitOps**: Implement ArgoCD workflows
- **Policy enforcement**: Add OPA/Kyverno admission controllers

## Reporting Issues
Open an issue with:
- Description of the problem
- Steps to reproduce
- Logs or error output
- Environment details (OS, Docker version, Go version)

## Pull Requests
- Keep PRs focused on a single concern
- Include test coverage for new functionality
- Update documentation as needed
- Reference any related issues
