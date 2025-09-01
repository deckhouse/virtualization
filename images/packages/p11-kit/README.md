# p11-kit Package

This package builds the p11-kit library from source. p11-kit is a library that provides a way to load and enumerate PKCS#11 modules.

## Source Repository

- **Repository**: https://github.com/p11-glue/p11-kit
- **Version**: v0.25.5

## Build Process

The package is built using werf v2 with the following stages:

1. **src-artifact**: Clones the source code from GitHub
2. **builder**: Builds the package using autotools on the builder/alt base image
3. **final**: Creates the final package image

## Dependencies

- gcc, git, make
- autoconf, automake, libtool
- glibc development libraries
- zlib development libraries

## Build Configuration

The package is configured with:
- Shared and static library support
- Installation to `/usr` with libraries in `lib64`
- Documentation disabled for smaller size

## Output

The built package includes:
- p11-kit binary
- libp11-kit shared libraries
- Header files and development artifacts
