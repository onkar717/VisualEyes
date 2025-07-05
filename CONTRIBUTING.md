# Contributing to VisualEyes

We love your input! We want to make contributing to VisualEyes as easy and transparent as possible, whether it's:

- Reporting a bug
- Discussing the current state of the code
- Submitting a fix
- Proposing new features
- Becoming a maintainer

## We Develop with Github

We use GitHub to host code, to track issues and feature requests, as well as accept pull requests.

## We Use [Github Flow](https://guides.github.com/introduction/flow/index.html)

Pull requests are the best way to propose changes to the codebase. We actively welcome your pull requests:

1. Fork the repo and create your branch from `main`.
2. If you've added code that should be tested, add tests.
3. If you've changed APIs, update the documentation.
4. Ensure the test suite passes.
5. Make sure your code lints.
6. Issue that pull request!

## Any contributions you make will be under the MIT Software License

In short, when you submit code changes, your submissions are understood to be under the same [MIT License](http://choosealicense.com/licenses/mit/) that covers the project. Feel free to contact the maintainers if that's a concern.

## Report bugs using Github's [issue tracker](https://github.com/onkar717/visual-eyes/issues)

We use GitHub issues to track public bugs. Report a bug by [opening a new issue](https://github.com/onkar717/visual-eyes/issues/new); it's that easy!

## Write bug reports with detail, background, and sample code

**Great Bug Reports** tend to have:

- A quick summary and/or background
- Steps to reproduce
  - Be specific!
  - Give sample code if you can.
- What you expected would happen
- What actually happens
- Notes (possibly including why you think this might be happening, or stuff you tried that didn't work)

## Development Process

1. Clone the repository
2. Create a new branch: `git checkout -b feature-name`
3. Make your changes
4. Run tests: `make test`
5. Run linter: `make lint`
6. Commit your changes: `git commit -m 'Add some feature'`
7. Push to the branch: `git push origin feature-name`
8. Submit a pull request

### Development Environment Setup

1. Install prerequisites:
   - Go 1.21 or later
   - PostgreSQL 14 or later
   - Docker and Docker Compose (optional)

2. Set up the development environment:
   ```bash
   # Install dependencies
   make deps

   # Build the project
   make build

   # Run tests
   make test
   ```

### Code Style

- Follow Go best practices and style guide
- Use meaningful variable and function names
- Write comments for complex logic
- Keep functions focused and small
- Add tests for new functionality

## License

By contributing, you agree that your contributions will be licensed under its MIT License. 