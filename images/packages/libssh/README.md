# libssh Package

This package builds the libssh library from source. libssh is a C library implementing the SSH protocol for client and server applications.

## Source Repository

- **Repository**: https://git.libssh.org/projects/libssh.git
- **Version**: v0.11.0

## Build Process

The package is built using werf v2 with the following stages:

1. **src-artifact**: Clones the source code from libssh.org
2. **builder**: Builds the package using CMake on the builder/alt base image
3. **final**: Creates the final package image

## Dependencies

- gcc, git, make
- cmake build system
- glibc development libraries
- zlib development libraries

## Build Configuration

The package is configured with CMake:
- Shared and static library support
- Installation to `/usr` with libraries in `lib64`
- Examples and tests disabled for smaller size
- Release build type for optimized performance

## Output

The built package includes:
- libssh shared libraries
- Header files and development artifacts

## Usage

libssh is used by applications that need SSH client or server functionality, such as file transfer tools, remote access applications, and system administration tools.
