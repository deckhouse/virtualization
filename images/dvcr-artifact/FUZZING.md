# Uploader Package Fuzzing Tests

Fuzzing tests for the uploader package's HTTP parsing and validation functions using Go's native fuzzing framework.

## Quick Reference

| Command                                            | Description                                |
| -------------------------------------------------- | ------------------------------------------ |
| `./docker-fuzz.sh`                                 | Run all fuzz tests in Docker (recommended) |
| `./docker-fuzz.sh -t 5m`                           | Run all tests for 5 minutes                |
| `cd pkg/uploader && go test -fuzz=. -fuzztime=30s` | Direct local testing                       |
| `./fuzz.sh`                                        | Direct local testing                       |

**🐳 Docker Required**: All testing should be done in Docker containers for isolation and reproducibility.

## Quick Start

### Docker-based Fuzzing (Recommended)

```bash
# Build and run all fuzzing tests
./docker-fuzz.sh

# Run with custom duration
./docker-fuzz.sh -t 5m

# Direct build and run (single command)
docker run --rm --platform linux/amd64 $(docker build --platform linux/amd64 -q -f fuzz.Dockerfile .)
```

## Direct Docker Commands

For immediate testing without scripts, you can build and run in a single command:

```bash
# Build and run immediately (recommended for quick testing)
docker run --rm --platform linux/amd64 $(docker build --platform linux/amd64 -q -f fuzz.Dockerfile .)

# With custom duration
docker run --rm --platform linux/amd64 -e $(docker build --platform linux/amd64 -q -f fuzz.Dockerfile .)
```

## Local Commands

For running locally, you can run the following commands:

```bash
./fuzz.sh
```

# Tests

### FuzzUploader

This test is used to fuzz the uploader package's HTTP parsing and validation functions using Go's native fuzzing framework.
Test start uploader server and DVCR mock server before running the test. The test will send a request to the uploader server with the fuzzed data.
After the request is sent, the test will check the response status code and the response body to ensure the request was successful. Then data will be sent to the DVCR mock server.

#### Example

```bash
$ ./fuzz.sh FuzzUploader
=== RUN   TestFuzzUploader
--- PASS: TestFuzzUploader (0.00s)
    fuzz_test.go:102: Fuzzing ProcessRequests in /virtualization/images/dvcr-artifact/pkg/uploader/uploader_fuzz_test.go
    fuzz_test.go:102: Fuzzing ProcessRequest in /virtualization/images/dvcr-artifact/pkg/uploader/uploader_fuzz_test.go
    fuzz_test.go:102: Fuzzing ProcessRequests in /virtualization/images/dvcr-artifact/pkg/uploader/uploader_fuzz_test.go
    fuzz_test.go:102: Fuzzing ProcessRequest in /virtualization/images/dvcr-artifact/pkg/uploader/uploader_fuzz_test.go
```
