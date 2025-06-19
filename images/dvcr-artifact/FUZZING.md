# Uploader Package Fuzzing Tests

Fuzzing tests for the uploader package's HTTP parsing and validation functions using Go's native fuzzing framework.

## Quick Reference

| Command                                                                                                 | Description                                |
| ------------------------------------------------------------------------------------------------------- | ------------------------------------------ |
| `./docker-fuzz.sh`                                                                                      | Run all fuzz tests in Docker (recommended) |
| `./docker-fuzz.sh -t 5m`                                                                                | Run all tests for 5 minutes                |
| `cd pkg/uploader && go test -fuzz=. -fuzztime=30s`                                                      | Direct local testing                       |
| `FUZZ_TIME=5m ./fuzz.sh`                                                                                | Direct local testing                       |

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
docker run --rm --platform linux/amd64 -e FUZZ_TIME=5m $(docker build --platform linux/amd64 -q -f fuzz.Dockerfile .)
```

## Local Commands

For running locally, you can run the following commands:

```bash
FUZZ_TIME=5m ./fuzz.sh
```


# Tests

### FuzzUploader

This test is used to fuzz the uploader package's HTTP parsing and validation functions using Go's native fuzzing framework.


#### Example

```bash
$ ./fuzz.sh FuzzUploader
=== RUN   TestFuzzUploader
--- PASS: TestFuzzUploader (0.00s)
    fuzz_test.go:102: Fuzzing ProcessRequests in /virtualization/images/dvcr-artifact/pkg/uploader/uploader_fuzz_test.go
    fuzz_test.go:102: Fuzzing ProcessRequest in /virtualization/images/dvcr-artifact/pkg/uploader/uploader_fuzz_test.go
    fuzz_test.go:102: Fuzzing ProcessRequests in /virtualization/images/dvcr-artifact/pkg/uploader/uploader_fuzz_test.go
    fuzz_test.go:102: Fuzzing ProcessRequest in /virtualization/images/dvcr-artifact/pkg/uploader/uploader_fuzz_test.go
