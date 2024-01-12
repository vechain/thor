# Contributing to VechainThor

Welcome to VechainThor! We appreciate your interest in contributing. By participating in this project, you agree to
abide by our [Code of Conduct](https://github.com/vechain/thor/blob/master/CODE_OF_CONDUCT.md).

## VeChain Improvement Proposals (VIPs)

[Vechain Improvement Proposals (VIPs)](https://github.com/vechain/VIPs) documents protocol improvements to the
VechainThor blockchain. A successful VIP represents a consensus among the Vechain community and developers, and is a
standard for VechainThor implementations. The framework is designed to establish a systematic and organized approach for
introducing new features into the VechainThor protocol. We encourage community members and developers to actively
participate in shaping the future of Vechain by proposing and discussing innovative ideas.

### Before You Propose

Before submitting a new VIP, it's crucial to ensure that your idea hasn't already been proposed or implemented. Please
take the time to check the existing proposals on our GitHub repository to avoid duplication and to better understand the
current development landscape.

## How to Contribute

1. Fork the repository to your GitHub account.
2. Clone the forked repository to your local machine:
   ```bash
   git clone https://github.com/[your-username]/thor.git
   ```
   **Note:** Replace `[your-username]` with your actual GitHub username.
3. Create a new branch for your changes:
    ```bash
    git checkout -b feature/your-feature-name
    ```
4. For a smooth review process, please ensure your branch is up-to-date with the `master` branch of the `vechain/thor`
   repository, and run the tests before committing your changes:
    ```bash
    make test
    ```
    - **Note:**: Please refer to the [README](https://github.com/vechain/thor/blob/master/README.md) for information on
      how to start the node and interact with the
      API.
5. Make your changes and commit them with a clear and concise commit message.
6. Push your changes to your forked repository:
    ```bash
    git push origin feature/your-feature-name
    ```
7. Create a pull request to the `master` branch of the `vechain/thor` repository.
8. Ensure your PR description clearly explains your changes and the problem it solves.
    - Explain the major changes you are submitting for review. Often it is useful to open a second tab in your browser
      where you can look through the diff yourself to remind yourself of all the changes you have made.
9. Wait for feedback and be ready to address any requested changes.

## Code Style and Guidelines

### Code Style

- We use [gofmt](https://golang.org/cmd/gofmt/) to format our code. Please run `gofmt .` before committing your changes.

### Code Guidelines

- We follow the [Effective Go](https://golang.org/doc/effective_go) guidelines. Please make sure your code is idiomatic
  and follows the guidelines.

## Testing

### Unit Tests

```bash
make test
```

### Unit Tests with Coverage

```bash
make test-coverage
```

### E2E Tests

Our E2E tests are written in TypeScript, utilizing hardhat contract solidity development tools. Before running the E2E
tests, ensure you have the following prerequisites installed:

- [Docker](https://docs.docker.com/get-docker/)
- [Node.js](https://nodejs.org/en/download/)
- [Yarn](https://classic.yarnpkg.com/en/docs/install/)
- [Git](https://git-scm.com/downloads)


The E2E tests are located in the tests/thor-e2e-tests directory as a submodule. If you haven't initialized the submodule yet, run:

```bash
git submodule update --init --recursive
```

To run the E2E tests, build the Docker image first:

```bash
docker build -t vechain/thor-e2e .
export THOR_IMAGE=vechain/thor-e2e
```

Then, you can run the tests:

```bash
cd tests/thor-e2e-tests
yarn install
yarn test
```
