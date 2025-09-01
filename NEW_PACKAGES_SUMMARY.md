# New Packages Added to Virtualization Project

This document summarizes the three new packages that have been added to build from source in the virtualization project.

## Overview

Three new packages have been added to the project's build system:
1. **p11-kit** - PKCS#11 module management library
2. **linux-pam** - Pluggable Authentication Modules library  
3. **libssh** - SSH protocol implementation library

## Package Details

### 1. p11-kit (v0.25.5)

- **Source**: https://github.com/p11-glue/p11-kit
- **Purpose**: Provides a way to load and enumerate PKCS#11 modules
- **Build System**: Autotools (./autogen.sh + ./configure)
- **Dependencies**: Standard build tools, glibc, zlib

### 2. linux-pam (v1.6.0)

- **Source**: https://github.com/linux-pam/linux-pam
- **Purpose**: Flexible mechanism for authenticating users
- **Build System**: Autotools (./autogen.sh + ./configure)
- **Dependencies**: Standard build tools, glibc, zlib

### 3. libssh (v0.11.0)

- **Source**: https://git.libssh.org/projects/libssh.git
- **Purpose**: C library implementing the SSH protocol
- **Build System**: CMake
- **Dependencies**: Standard build tools, glibc, zlib

## Build Configuration

All packages are configured to:
- Use **werf v2** build system
- Use **builder/alt** as the base image for building
- Follow the project's standard package structure
- Use the image naming convention: `{{ .ModuleNamePrefix }}{{ .PackagePath }}/{{ .ImageName }}-src-artifact`
- Build both shared and static libraries
- Install to `/usr` with libraries in `lib64`
- Strip binaries and libraries for smaller size

## File Structure

```
images/packages/
├── p11-kit/
│   ├── werf.inc.yaml
│   └── README.md
├── linux-pam/
│   ├── werf.inc.yaml
│   └── README.md
└── libssh/
    ├── werf.inc.yaml
    └── README.md
```

## Version Configuration

Package versions have been added to `build/components/versions.yml`:

```yaml
package:
  # ... existing packages ...
  p11-kit: v0.25.5
  linux-pam: v1.6.0
  libssh: v0.11.0
  # ... existing packages ...
```

## Build Process

Each package follows the standard three-stage build process:

1. **src-artifact**: Clones source code from the respective repository
2. **builder**: Builds the package using appropriate build system on builder/alt
3. **final**: Creates the final package image with built artifacts

## Integration

These packages are now automatically included in the project's build system and will be built alongside other packages when using werf. They can be referenced by other components that need these libraries as dependencies.
