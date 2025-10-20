# Performance Testing Script - Original Version

## Overview

`tests.sh` is the original performance testing script for Kubernetes Virtual Machines (VMs) and Virtual Disks (VDs) operations. It provides comprehensive end-to-end testing scenarios for virtualization workloads.

## Features

- **Complete Scenario Execution** - Runs full test scenarios from start to finish
- **Comprehensive Testing** - Tests VM lifecycle, migrations, controller restarts, and node draining
- **Detailed Reporting** - Generates comprehensive reports with timing and statistics
- **Multiple Scenarios** - Supports different virtual image types
- **Production Ready** - Battle-tested in production environments

## Usage

### Basic Commands

```bash
# Run scenario 1 with 2 resources (default)
./tests.sh

# Run scenario 1 with 4 resources
./tests.sh -s 1 -c 4

# Run scenario 2 with 10 resources
./tests.sh -s 2 -c 10

# Clean reports and run
./tests.sh --clean-reports
```

### Command Line Options

| Option | Description | Default |
|--------|-------------|---------|
| `-s, --scenario NUMBER` | Scenario number to run (1 or 2) | 1 |
| `-c, --count NUMBER` | Number of resources to create | 2 |
| `--clean-reports` | Clean all report directories before running | false |
| `-h, --help` | Show help message | - |

### Examples

```bash
# Default execution (scenario 1, 2 resources)
./tests.sh

# Custom resource count
./tests.sh -c 10

# Different scenario
./tests.sh -s 2 -c 5

# Clean start
./tests.sh --clean-reports -c 20
```

## Scenarios

### Scenario 1: persistentVolumeClaim (Default)
- **Virtual Image Type**: persistentVolumeClaim
- **Test Coverage**: VM lifecycle, migrations, controller operations
- **Use Case**: Production workloads with persistent storage

### Scenario 2: containerRegistry (Currently Disabled)
- **Virtual Image Type**: containerRegistry
- **Test Coverage**: Similar to Scenario 1 but with container images
- **Use Case**: Container-based workloads

## Test Sequence

The script runs a comprehensive 22-step test sequence:

1. **Cleanup** - Remove existing resources
2. **VM Deployment** - Deploy VMs with disks
3. **Statistics Collection** - Gather initial statistics
4. **VM Stop** - Stop all VMs
5. **VM Start** - Start all VMs
6. **Migration Setup** - Start 5% migration in background
7. **VM Undeploy** - Undeploy 10% VMs (keeping disks)
8. **VM Deploy** - Deploy 10% VMs
9. **Statistics Collection** - Gather statistics for 10% VMs
10. **VM Undeploy 10%** - Undeploy 10% VMs (keeping disks)
11. **VM Deploy 10%** - Deploy 10% VMs (keeping disks)
12. **VM Statistics** - Gather statistics for 10% VMs
13. **VM Operations** - Test stop/start operations on 10% VMs
14. **Migration Cleanup** - Stop migration and cleanup
15. **Migration Percentage** - Migrate 10% VMs
16. **Migration Parallel 2x** - Test with 2x parallel migrations
17. **Migration Parallel 4x** - Test with 4x parallel migrations
18. **Migration Parallel 8x** - Test with 8x parallel migrations
19. **Controller Restart** - Test controller restart with VM creation
20. **Final Statistics** - Gather final statistics
21. **Drain Node** - Test node draining
22. **Final Cleanup** - Clean up all resources

## Report Structure

Reports are generated in the `report/` directory with the following structure:

```
report/
└── scenario_1_persistentVolumeClaim_2vm_20251021_013737/
    ├── test.log                    # Main test log
    ├── vm_operations.log          # VM operations log
    ├── summary.txt                # Summary report
    ├── statistics/                 # Statistics data
    │   ├── *.csv                  # CSV statistics files
    │   └── ...
    └── vpa/                       # VPA data
        ├── vpa_*.yaml             # VPA configurations
        └── ...
```

### Report Naming Convention

```
{scenario_name}_{vi_type}_{count}vm_{timestamp}
```

Example: `scenario_1_persistentVolumeClaim_2vm_20251021_013737`

## Configuration

### Default Values

```bash
SCENARIO_NUMBER=1
MAIN_COUNT_RESOURCES=2
PERCENT_VMS=10
MIGRATION_DURATION="1m"
MIGRATION_PERCENTAGE_10=10
MIGRATION_PERCENTAGE_5=5
```

### Resource Calculations

- **Percent Resources**: 10% of total resources
- **Migration 5% Count**: 5% of total resources (minimum 1)
- **Migration 10% Count**: 10% of total resources (minimum 1)

## Dependencies

### Required Tools
- `kubectl` - Kubernetes command-line tool
- `helm` - Package manager for Kubernetes
- `tmux` - Terminal multiplexer for migration testing
- `jq` - JSON processor
- `bc` - Calculator for percentages

### Kubernetes Requirements
- Kubernetes cluster with virtualization support
- Virtualization controller running
- Proper RBAC permissions
- Storage classes configured

## Output and Logging

### Log Levels
- **INFO** - General information
- **SUCCESS** - Successful operations
- **WARNING** - Non-critical issues
- **ERROR** - Error conditions

### Log Files
- **test.log** - Main test execution log
- **vm_operations.log** - Detailed VM operations log
- **summary.txt** - Comprehensive summary report

### Console Output
- Real-time progress updates
- Step-by-step execution status
- Duration and timing information
- Error messages and warnings

## Performance Metrics

The script measures and reports:

- **VM Deployment Time** - Time to deploy VMs and disks
- **VM Stop Time** - Time to stop all VMs
- **VM Start Time** - Time to start all VMs
- **Migration Times** - Time for various migration scenarios
- **Controller Restart Time** - Time for controller restart
- **Node Drain Time** - Time for node draining operations

## Troubleshooting

### Common Issues

1. **Permission Denied**
   ```bash
   # Ensure proper Kubernetes access
   kubectl auth can-i create virtualmachines
   ```

2. **Storage Class Issues**
   ```bash
   # Check available storage classes
   kubectl get storageclass
   ```

3. **Controller Not Available**
   ```bash
   # Check controller status
   kubectl get pods -n d8-virtualization
   ```

4. **Migration Failures**
   ```bash
   # Check migration status
   kubectl get vmop -n perf
   ```

### Debug Mode

Enable debug output by uncommenting the debug line:
```bash
# set -x  # Uncomment this line for debug output
```

## Best Practices

1. **Resource Planning**
   - Start with small resource counts for testing
   - Increase gradually for production testing
   - Monitor cluster resources during execution

2. **Environment Setup**
   - Ensure cluster has sufficient resources
   - Configure proper storage classes
   - Set up monitoring and logging

3. **Test Execution**
   - Run tests during low-traffic periods
   - Monitor cluster health during execution
   - Keep logs for analysis

4. **Cleanup**
   - Always run cleanup after tests
   - Monitor for orphaned resources
   - Verify cluster state after completion

## Examples

### Development Testing
```bash
# Quick test with minimal resources
./tests.sh -c 2

# Test with more resources
./tests.sh -c 10
```

### Production Testing
```bash
# Full production test
./tests.sh -c 50 --clean-reports

# Long-running test
./tests.sh -c 100
```

### Custom Scenarios
```bash
# Test scenario 2 (if enabled)
./tests.sh -s 2 -c 20

# Clean environment test
./tests.sh --clean-reports -c 30
```

## Support

For issues and questions:
1. Check the logs in the report directory
2. Verify Kubernetes cluster status
3. Ensure all dependencies are installed
4. Check resource availability
