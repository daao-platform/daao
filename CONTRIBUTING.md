# Contributing to DAAO

Thank you for your interest in contributing to DAAO! We welcome contributions from the community.

## How to Contribute

### Reporting Issues

- Use [GitHub Issues](../../issues) to report bugs or request features
- Include steps to reproduce, expected behavior, and actual behavior
- Include your OS, Go version, and Node.js version where relevant

### Submitting Pull Requests

1. Fork the repository
2. Create a feature branch from `main` (`git checkout -b feature/my-feature`)
3. Make your changes
4. Ensure all tests pass (`go test ./...` and `cd cockpit && npm test`)
5. Commit with a descriptive message following [Conventional Commits](https://www.conventionalcommits.org/)
6. Push to your fork and open a Pull Request

### Code Style

- **Go:** Run `gofmt` and `go vet` before committing
- **TypeScript/React:** Follow the existing ESLint configuration in `cockpit/`
- **Commits:** Use conventional commit messages (e.g., `feat:`, `fix:`, `docs:`)

### What to Contribute

- Bug fixes and improvements to existing features
- Documentation improvements
- Test coverage
- Performance optimizations

### Enterprise Features

Enterprise feature implementations are maintained in a private repository and are only available in official builds. Community contributions to enterprise interfaces (stubs in `internal/enterprise/`) require prior discussion — please open an issue first.

## Contributor License Agreement

By submitting a pull request, you agree to license your contribution under the same [Business Source License 1.1](./LICENSE) that covers the project. You represent that you have the right to make the contribution and that it does not infringe on any third-party rights.

## Code of Conduct

Be respectful, constructive, and professional. We're building something together.

## Questions?

Open a [Discussion](../../discussions) or reach out via Issues.
