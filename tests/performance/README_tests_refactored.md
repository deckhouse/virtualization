# Performance Testing Script - Refactored Version

## Overview

`tests_refactored.sh` is a modular, enhanced version of the original performance testing script. It provides all the functionality of the original script plus advanced features for individual step execution, debugging, and development workflows.

## Key Features

- **Full Backward Compatibility** - Runs complete scenarios exactly like the original script
- **Individual Step Execution** - Run specific test steps independently
- **From-Step Execution** - Continue execution from any step
- **Modular Architecture** - Clean, maintainable code structure
- **Enhanced Logging** - Step numbers and improved visibility
- **Batch Deployment Support** - Supports large-scale deployments (up to 15,000 VMs) with intelligent batching
- **Development Friendly** - Perfect for debugging and development

## Usage

### Full Scenario Execution (Original Behavior)

```bash
# Run scenario 1 with 2 resources (default)
./tests_refactored.sh

# Run scenario 1 with 4 resources
./tests_refactored.sh -s 1 -c 4

# Run scenario 2 with 10 resources
./tests_refactored.sh -s 2 -c 10

# Clean reports and run
./tests_refactored.sh --clean-reports

# Large scale deployment with batch processing
./tests_refactored.sh -c 15000 --batch-size 1200

# Force batch deployment for smaller numbers
./tests_refactored.sh -c 500 --enable-batch
```

### Individual Step Execution (New Feature)

```bash
# List all available steps
./tests_refactored.sh --list-steps

# Run a specific step
./tests_refactored.sh --step cleanup --scenario-dir /path/to/scenario --vi-type persistentVolumeClaim
./tests_refactored.sh --step vm-deployment --scenario-dir /path/to/scenario --vi-type persistentVolumeClaim
./tests_refactored.sh --step statistics-collection --scenario-dir /path/to/scenario
./tests_refactored.sh --step vm-operations --scenario-dir /path/to/scenario
./tests_refactored.sh --step migration-tests --scenario-dir /path/to/scenario
./tests_refactored.sh --step controller-restart --scenario-dir /path/to/scenario --vi-type persistentVolumeClaim
./tests_refactored.sh --step final-operations --scenario-dir /path/to/scenario
```

### From-Step Execution (New Feature)

```bash
# Continue from a specific step
./tests_refactored.sh --from-step vm-operations --scenario-dir /path/to/scenario --vi-type persistentVolumeClaim
./tests_refactored.sh --from-step migration-tests --scenario-dir /path/to/scenario --vi-type persistentVolumeClaim
./tests_refactored.sh --from-step controller-restart --scenario-dir /path/to/scenario --vi-type persistentVolumeClaim
```

## Command Line Options

| Option | Description | Required For | Default |
|--------|-------------|--------------|---------|
| `-s, --scenario NUMBER` | Scenario number to run (1 or 2) | Full scenarios | 1 |
| `-c, --count NUMBER` | Number of resources to create | Full scenarios | 2 |
| `--batch-size NUMBER` | Maximum resources per batch | Optional | 1200 |
| `--enable-batch` | Force batch deployment mode | Optional | false |
| `--step STEP_NAME` | Run a specific step only | Individual steps | - |
| `--from-step STEP_NAME` | Run all steps starting from STEP_NAME | From-step execution | - |
| `--list-steps` | List all available steps | - | - |
| `--scenario-dir DIR` | Directory for scenario data | Individual/From-step | - |
| `--vi-type TYPE` | Virtual image type | Some steps | persistentVolumeClaim |
| `--clean-reports` | Clean all report directories before running | Optional | false |
| `--no-pre-cleanup` | Do not cleanup resources before running | Optional | false |
| `--no-post-cleanup` | Do not cleanup resources after running | Optional | false |
| `-h, --help` | Show help message | - | - |

## Batch Deployment

For large-scale deployments (>1200 resources), the script automatically uses intelligent batch deployment:

### Features
- **Automatic Batching** - Automatically detects when batch deployment is needed
- **Configurable Batch Size** - Default 1200 resources per batch (customizable)
- **Progress Tracking** - Real-time progress updates with ETA
- **Cluster Resource Checks** - Pre-deployment resource validation
- **Stability Delays** - 30-second delays between batches to prevent cluster overload

### Configuration
```bash
# Default batch settings
MAX_BATCH_SIZE=1200
TOTAL_TARGET_RESOURCES=15000
BATCH_DEPLOYMENT_ENABLED=false
```

### Examples
```bash
# Deploy 15,000 VMs in batches of 1200
./tests_refactored.sh -c 15000 --batch-size 1200

# Force batch mode for smaller deployments
./tests_refactored.sh -c 500 --enable-batch

# Custom batch size
./tests_refactored.sh -c 5000 --batch-size 800
```

## Available Steps

The script supports 13 individual steps that can be executed independently:

### 1. cleanup
- **Purpose**: Clean up existing resources
- **Required**: `--scenario-dir`, `--vi-type`
- **Use Case**: Prepare clean environment

### 2. vm-deployment
- **Purpose**: Deploy VMs with disks
- **Required**: `--scenario-dir`, `--vi-type`
- **Use Case**: Test VM deployment functionality

### 3. statistics-collection
- **Purpose**: Gather initial statistics
- **Required**: `--scenario-dir`
- **Use Case**: Collect baseline metrics

### 4. vm-operations
- **Purpose**: Stop and start all VMs
- **Required**: `--scenario-dir`
- **Use Case**: Test VM lifecycle operations

### 5. vm-undeploy-deploy
- **Purpose**: Undeploy and redeploy 10% VMs
- **Required**: `--scenario-dir`
- **Use Case**: Test VM redeployment

### 6. vm-operations-test
- **Purpose**: Test stop/start operations on 10% VMs
- **Required**: `--scenario-dir`
- **Use Case**: Test partial VM operations

### 7. migration-tests
- **Purpose**: Run migration tests (5% and 10%)
- **Required**: `--scenario-dir`
- **Use Case**: Test migration functionality

### 8. migration-parallel-2x
- **Purpose**: Migrate with parallelMigrationsPerCluster at 2x nodes
- **Required**: `--scenario-dir`
- **Use Case**: Test parallel migration scenarios

### 9. migration-parallel-4x
- **Purpose**: Migrate with parallelMigrationsPerCluster at 4x nodes
- **Required**: `--scenario-dir`
- **Use Case**: Test higher parallel migration scenarios

### 10. migration-parallel-8x
- **Purpose**: Migrate with parallelMigrationsPerCluster at 8x nodes
- **Required**: `--scenario-dir`
- **Use Case**: Test maximum parallel migration scenarios

### 11. controller-restart
- **Purpose**: Test controller restart with VM creation
- **Required**: `--scenario-dir`, `--vi-type`
- **Use Case**: Test controller resilience

### 12. drain-node
- **Purpose**: Run drain node workload
- **Required**: `--scenario-dir`
- **Use Case**: Test node draining operations

### 13. final-operations
- **Purpose**: Final statistics and optional cleanup
- **Required**: `--scenario-dir`
- **Use Case**: Complete test sequence

## Use Cases

### Development and Debugging

```bash
# Test only VM deployment
./tests_refactored.sh --step vm-deployment --scenario-dir ./test-scenario --vi-type persistentVolumeClaim

# Test migration functionality
./tests_refactored.sh --step migration-tests --scenario-dir ./test-scenario --vi-type persistentVolumeClaim

# Debug controller restart
./tests_refactored.sh --step controller-restart --scenario-dir ./test-scenario --vi-type persistentVolumeClaim
```

### Resuming Interrupted Tests

```bash
# Continue from VM operations
./tests_refactored.sh --from-step vm-operations --scenario-dir ./existing-scenario --vi-type persistentVolumeClaim

# Continue from migration tests
./tests_refactored.sh --from-step migration-tests --scenario-dir ./existing-scenario --vi-type persistentVolumeClaim
```

### Production Testing

```bash
# Run full scenario (same as original script)
./tests_refactored.sh -s 1 -c 10

# Run with custom parameters
./tests_refactored.sh -s 1 -c 20 --clean-reports

# Large scale production test
./tests_refactored.sh -c 15000 --batch-size 1200 --clean-reports
```

## Report Structure

### Full Scenario Reports
```
report/
└── scenario_1_persistentVolumeClaim_2vm_20251021_013737/
    ├── test.log                    # Main test log
    ├── vm_operations.log          # VM operations log
    ├── summary.txt                # Summary report
    ├── statistics/                 # Statistics data
    └── vpa/                       # VPA data
```

### Individual Step Reports
```
report/
└── step_vm-deployment_persistentVolumeClaim_2vm_20251021_013653/
    ├── test.log                    # Step execution log
    ├── vm_operations.log          # VM operations log
    └── statistics/                 # Step statistics
```

## Enhanced Logging

The refactored script provides enhanced logging with:

- **Step Numbers** - Each step shows its number in the sequence
- **Clear Step Boundaries** - Easy to identify step start/end
- **Consistent Format** - Uniform logging across all steps
- **Better Visibility** - Improved debugging and monitoring

Example log output:
```
[INFO] === Executing Step 1: cleanup ===
[INFO] === Running Step 1: cleanup ===
[STEP_START] Cleanup existing resources
[STEP_END] Cleanup existing resources completed in 00:00:15
[SUCCESS] Cleanup step completed
```

## Modular Architecture

The script is organized into library modules:

- **`lib/common.sh`** - Common utilities and logging functions
- **`lib/vm_operations.sh`** - VM operation functions
- **`lib/migration.sh`** - Migration testing functions
- **`lib/statistics.sh`** - Statistics collection functions
- **`lib/controller.sh`** - Controller management functions
- **`lib/reporting.sh`** - Report generation functions
- **`lib/scenarios.sh`** - Scenario orchestration functions

## Migration from Original Script

The refactored script is fully backward compatible:

1. **Drop-in Replacement** - Can replace `tests.sh` without changes
2. **Same Output** - Generates identical reports and logs
3. **Same Performance** - No performance overhead
4. **Additional Features** - Adds new capabilities without breaking existing workflows

## Examples

### Quick Development Test
```bash
# Test VM deployment only
./tests_refactored.sh --step vm-deployment --scenario-dir ./dev-test --vi-type persistentVolumeClaim
```

### Debugging Migration Issues
```bash
# Test migration setup
./tests_refactored.sh --step migration-tests --scenario-dir ./debug-scenario --vi-type persistentVolumeClaim

# Test specific parallel migration
./tests_refactored.sh --step migration-parallel-4x --scenario-dir ./debug-scenario --vi-type persistentVolumeClaim
```

### Production Workflow
```bash
# Full production test
./tests_refactored.sh -s 1 -c 50 --clean-reports

# Resume interrupted test
./tests_refactored.sh --from-step vm-operations --scenario-dir ./production-scenario --vi-type persistentVolumeClaim
```

### Controller Testing
```bash
# Test controller restart
./tests_refactored.sh --step controller-restart --scenario-dir ./controller-test --vi-type persistentVolumeClaim

# Test node draining
./tests_refactored.sh --step drain-node --scenario-dir ./drain-test --vi-type persistentVolumeClaim
```

## Troubleshooting

### Common Issues

1. **Missing scenario directory**
   ```bash
   # Error: Scenario directory is required for individual step execution
   # Solution: Provide --scenario-dir parameter
   ./tests_refactored.sh --step cleanup --scenario-dir ./my-scenario --vi-type persistentVolumeClaim
   ```

2. **Unknown step**
   ```bash
   # Error: Unknown step: invalid-step
   # Solution: Use --list-steps to see available steps
   ./tests_refactored.sh --list-steps
   ```

3. **Missing VI type**
   ```bash
   # Error: VI type required for some steps
   # Solution: Provide --vi-type parameter
   ./tests_refactored.sh --step vm-deployment --scenario-dir ./test --vi-type persistentVolumeClaim
   ```

### Getting Help

```bash
# Show help
./tests_refactored.sh --help

# List available steps
./tests_refactored.sh --list-steps

# Show step details
./tests_refactored.sh --list-steps | grep -A 5 "cleanup"
```

## Best Practices

### Development Workflow
1. **Start Small** - Test individual steps with minimal resources
2. **Use Scenario Directories** - Create dedicated directories for different test scenarios
3. **Step-by-Step Testing** - Test each step independently before running full scenarios
4. **Monitor Logs** - Check step logs for issues and performance

### Production Workflow
1. **Full Scenarios** - Use full scenario execution for production testing
2. **Resource Planning** - Plan resources based on test requirements
3. **Monitoring** - Monitor cluster health during execution
4. **Cleanup** - Always run cleanup after tests

### Debugging
1. **Individual Steps** - Use individual step execution for debugging
2. **From-Step Execution** - Resume from specific steps after fixes
3. **Log Analysis** - Use enhanced logging for better debugging
4. **Step Isolation** - Test problematic steps in isolation

## Advanced Usage

### Custom Step Sequences
```bash
# Run specific step sequence
./tests_refactored.sh --step cleanup --scenario-dir ./custom --vi-type persistentVolumeClaim
./tests_refactored.sh --step vm-deployment --scenario-dir ./custom --vi-type persistentVolumeClaim
./tests_refactored.sh --step statistics-collection --scenario-dir ./custom
```

### Parallel Testing
```bash
# Test different scenarios in parallel
./tests_refactored.sh --step vm-deployment --scenario-dir ./test1 --vi-type persistentVolumeClaim &
./tests_refactored.sh --step vm-deployment --scenario-dir ./test2 --vi-type persistentVolumeClaim &
```

### Integration with CI/CD
```bash
# Automated testing pipeline
./tests_refactored.sh --step cleanup --scenario-dir ./ci-test --vi-type persistentVolumeClaim
./tests_refactored.sh --step vm-deployment --scenario-dir ./ci-test --vi-type persistentVolumeClaim
./tests_refactored.sh --step statistics-collection --scenario-dir ./ci-test
```
